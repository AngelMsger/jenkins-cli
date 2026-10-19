package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/agent-cli-kit/reuse"
	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/internal/config"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// The decision itself is agent-cli-kit/reuse's and tested there. These tests
// cover jenkins-cli's wiring: how contexts are described, how the destination
// is rewritten, and the native store, verification, account and command. Both
// Jenkins schemes send a username, so a destination without one has no login
// of its own by configuration.

const reuseBase = "https://service.example.test/deploy"

// fakeService answers lookups from keychain entries keyed like the real store
// and authenticates "username|secret" pairs as Jenkins accounts.
type fakeService struct {
	secrets  map[string]string
	accounts map[string]reuse.Account
}

func (f *fakeService) lookup(key string) credstore.Lookup {
	if v, ok := f.secrets[key]; ok {
		return credstore.Lookup{Secret: v, Keychain: credstore.Hit}
	}
	return credstore.Lookup{Keychain: credstore.Miss, File: credstore.Miss}
}

func (f *fakeService) resolve(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
	l := f.lookup(auth.AccountKey(cfg.BaseURL, cfg.Auth.Scheme))
	if !l.Found() {
		return auth.Credential{}, l, cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_NOT_VISIBLE_OR_MISSING", "missing")
	}
	cred := auth.Credential{Scheme: cfg.Auth.Scheme, Username: cfg.Auth.Username, Secret: l.Secret}
	return cred, l, cred.Validate()
}

func (f *fakeService) verify(_ context.Context, _ config.Config, cred auth.Credential) (reuse.Account, error) {
	if account, ok := f.accounts[cred.Username+"|"+cred.Secret]; ok {
		return account, nil
	}
	return reuse.Account{}, cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
}

func reuseBasicContext(name, user string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "basic", Username: user}}
}

func reuseTokenContext(name, user string) config.NamedContext {
	return config.NamedContext{Name: name, BaseURL: reuseBase, Auth: config.AuthConfig{Scheme: "token", Username: user}}
}

// reuseFixture writes contexts (the last is the destination) and a service on
// which existing-user's password is stored and verifies.
func reuseFixture(t *testing.T, contexts ...config.NamedContext) (*appState, *fakeService, reuseServices) {
	t.Helper()
	dir := t.TempDir()
	if len(contexts) == 0 {
		contexts = []config.NamedContext{reuseBasicContext("personal", "existing-user"), reuseBasicContext("team", "")}
	}
	file := config.File{CurrentContext: contexts[0].Name, Contexts: contexts}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	target := contexts[len(contexts)-1]
	s := &appState{cfgDir: dir, resolved: &config.Resolved{Config: config.StoredContext(target, file.Defaults), ActiveContext: target.Name}}
	fake := &fakeService{
		secrets:  map[string]string{auth.AccountKey(reuseBase, "basic"): "test-secret"},
		accounts: map[string]reuse.Account{"existing-user|test-secret": {ID: "existing-user", Name: "Existing User"}},
	}
	services := reuseServices{Read: config.ReadFile, Write: config.WriteFile, Resolve: fake.resolve, Lookup: fake.lookup, Verify: fake.verify}
	return s, fake, services
}

func targetOf(t *testing.T, s *appState) config.NamedContext {
	t.Helper()
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := file.Context(s.resolved.ActiveContext)
	return c
}

func TestAuthReusePreviewApplyAndIdempotence(t *testing.T) {
	s, _, services := reuseFixture(t)
	path := config.ConfigFilePath(s.cfgDir)
	bytesBefore, _ := os.ReadFile(path)
	before, _, _ := config.ReadFile(s.cfgDir)
	preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !preview.Changed || preview.State != "available" || !preview.Verified || preview.Scheme != "basic" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if afterPreview, _ := os.ReadFile(path); string(afterPreview) != string(bytesBefore) {
		t.Fatal("preview changed config")
	}
	applied, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || applied.State != "reused" || applied.SourceContext != "personal" || applied.Username != "existing-user" ||
		applied.Account != "existing-user" || applied.AccountName != "Existing User" {
		t.Fatalf("apply: %+v %v", applied, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if got := targetOf(t, s); got.Auth.Username != "existing-user" || after.CurrentContext != before.CurrentContext || !reflect.DeepEqual(after.Contexts[0], before.Contexts[0]) {
		t.Fatalf("did not preserve the source or the current context: %+v", after)
	}
	services.Write = func(string, config.File) error { t.Fatal("repeated write"); return nil }
	services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
		t.Fatal("populated identity must remain unchanged")
		return auth.Credential{}, credstore.Lookup{}, nil
	}
	repeated, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || repeated.Changed || repeated.State != "unchanged" {
		t.Fatalf("repeat: %+v %v", repeated, err)
	}
	raw, _ := json.Marshal(applied)
	if strings.Contains(string(raw), "test-secret") {
		t.Fatal("secret reached output")
	}
	if len(applied.NextSteps) != 1 || !strings.HasSuffix(applied.NextSteps[0], "auth status") {
		t.Fatalf("next steps: %v", applied.NextSteps)
	}
}

// A token team context without a username adopts the personal password login
// on the same Jenkins and reports the account it belongs to.
func TestAuthReuseTokenContextAdoptsBasicLogin(t *testing.T) {
	s, _, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), reuseTokenContext("team", ""))
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "basic" || r.Username != "existing-user" || r.Account != "existing-user" {
		t.Fatalf("got %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" {
		t.Fatalf("destination %+v", got)
	}
}

// A password stored for the service that no context points at any more is
// paired with the username a context records, once that context's own token
// is rejected.
func TestAuthReuseStoredSecretPairsWithRecordedUsername(t *testing.T) {
	s, fake, services := reuseFixture(t, reuseTokenContext("personal", "existing-user"), reuseTokenContext("team", ""))
	fake.secrets[auth.AccountKey(reuseBase, "token")] = "revoked-token"
	before, _, _ := config.ReadFile(s.cfgDir)
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "" || r.Scheme != "basic" || r.Username != "existing-user" ||
		r.Account != "existing-user" || !strings.Contains(r.Reason, "stored") {
		t.Fatalf("got %+v %v", r, err)
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "existing-user" || !reflect.DeepEqual(after.Contexts[0], before.Contexts[0]) {
		t.Fatalf("destination %+v, source %+v", got, after.Contexts[0])
	}
}

// A password and an API token of one Jenkins account are one login: no choice
// is asked for, and the destination's own scheme is preferred.
func TestAuthReuseOneAccountIsOneLogin(t *testing.T) {
	s, fake, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), reuseTokenContext("team", ""))
	fake.secrets[auth.AccountKey(reuseBase, "token")] = "api-token"
	fake.accounts["existing-user|api-token"] = reuse.Account{ID: "existing-user", Name: "Existing User"}
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || r.State != "available" || r.Scheme != "token" || r.Username != "existing-user" || r.Account != "existing-user" {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestAuthReuseMatchesCompleteServiceBeforeCredentialAccess(t *testing.T) {
	for _, kind := range []string{"host", "path", "protocol", "port"} {
		t.Run(kind, func(t *testing.T) {
			personal := reuseBasicContext("personal", "existing-user")
			switch kind {
			case "host":
				personal.BaseURL = "https://other.example.test/deploy"
			case "path":
				personal.BaseURL = "https://service.example.test/other"
			case "protocol":
				personal.BaseURL = "http://service.example.test/deploy"
			case "port":
				personal.BaseURL = "https://service.example.test:8443/deploy"
			}
			for _, scheme := range []string{"basic", "token"} {
				team := reuseBasicContext("team", "")
				team.Auth.Scheme = scheme
				s, _, services := reuseFixture(t, personal, team)
				resolve := services.Resolve
				services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
					if cfg.Auth.Username == "existing-user" {
						t.Fatal("read another service's credential")
					}
					return resolve(cfg)
				}
				result, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
				if err != nil || result.Changed || result.State != "unavailable" {
					t.Fatalf("%s mismatch: %+v %v", scheme, result, err)
				}
				_, err = reuseAuthentication(s, reuseOptions{From: "personal", DryRun: true}, services)
				if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
					t.Fatalf("%s explicit mismatch: %v", scheme, err)
				}
			}
		})
	}
}

// A scheme the invocation supplies keeps reuse within that scheme.
func TestAuthReuseSuppliedSchemeStaysWithinIt(t *testing.T) {
	s, _, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), reuseTokenContext("team", ""))
	s.resolved.Sources = map[string]string{config.FieldAuthScheme: "flag"}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("pinned: %+v %v", r, err)
	}
	for _, opts := range []reuseOptions{{From: "personal"}, {FromScheme: "basic"}} {
		_, err = reuseAuthentication(s, opts, services)
		if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_MISMATCH" {
			t.Fatalf("pinned explicit %+v: %v", opts, err)
		}
	}
}

// Both Jenkins schemes send the username, so under either one a recorded
// username is the destination's identity.
func TestAuthReusePreservesDestinationIdentity(t *testing.T) {
	for _, team := range []config.NamedContext{reuseBasicContext("team", "other-user"), reuseTokenContext("team", "other-user")} {
		s, _, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), team)
		services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
			t.Fatal("must not replace an existing identity")
			return auth.Credential{}, credstore.Lookup{}, nil
		}
		services.Lookup = func(string) credstore.Lookup {
			t.Fatal("must not read the store for an existing identity")
			return credstore.Lookup{}
		}
		r, err := reuseAuthentication(s, reuseOptions{}, services)
		if err != nil || r.State != "unchanged" || r.Changed {
			t.Fatalf("%s identity: %+v %v", team.Auth.Scheme, r, err)
		}
	}
}

func TestAuthReuseAmbiguityAndExplicitSelection(t *testing.T) {
	second := reuseBasicContext("second", "second-user")
	second.BaseURL = "https://SERVICE.example.test:443/deploy"
	s, fake, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), second, reuseBasicContext("team", ""))
	fake.secrets[auth.AccountKey(second.BaseURL, "basic")] = "second-secret"
	fake.accounts["second-user|second-secret"] = reuse.Account{ID: "second-user"}
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Code != "AUTH_REUSE_AMBIGUOUS" {
		t.Fatalf("ambiguity: %v", err)
	}
	details, _ := ce.Details.(map[string]any)
	candidates, _ := details["candidates"].([]reuse.Candidate)
	if len(candidates) != 2 || candidates[0].Account == candidates[1].Account || candidates[0].Account == "" {
		t.Fatalf("candidates %+v", ce.Details)
	}
	r, err := reuseAuthentication(s, reuseOptions{From: "SECOND"}, services)
	if err != nil || r.SourceContext != "second" || r.State != "reused" || r.Account != "second-user" {
		t.Fatalf("explicit choice: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.BaseURL != second.BaseURL || got.Auth.Username != "second-user" {
		t.Fatalf("destination does not resolve the chosen key: %+v", got)
	}
}

// One stored password that verifies for two recorded usernames needs a choice;
// --from-scheme with --from-context pairs it with that context's username.
func TestAuthReuseStoredSecretAmbiguityAndFromScheme(t *testing.T) {
	s, fake, services := reuseFixture(t, reuseTokenContext("alice", "alice"), reuseTokenContext("bob", "bob"), reuseTokenContext("team", ""))
	fake.secrets[auth.AccountKey(reuseBase, "basic")] = "shared-secret"
	fake.accounts = map[string]reuse.Account{"alice|shared-secret": {ID: "alice"}, "bob|shared-secret": {ID: "bob"}}
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Code != "AUTH_REUSE_AMBIGUOUS" || !strings.Contains(strings.Join(ce.NextSteps, "\n"), "--from-scheme basic") {
		t.Fatalf("ambiguity: %v", err)
	}
	r, err := reuseAuthentication(s, reuseOptions{FromScheme: "basic", From: "alice"}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "" || r.Scheme != "basic" || r.Username != "alice" || r.Account != "alice" {
		t.Fatalf("explicit stored choice: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Scheme != "basic" || got.Auth.Username != "alice" {
		t.Fatalf("destination %+v", got)
	}
}

func TestAuthReuseDuplicateAliasesDoNotRequireChoice(t *testing.T) {
	s, _, services := reuseFixture(t, reuseBasicContext("personal", "existing-user"), reuseBasicContext("alias", "existing-user"), reuseBasicContext("team", ""))
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !r.Changed {
		t.Fatalf("same native credential: %+v %v", r, err)
	}
}

func TestAuthReuseRetainsOperationalFailures(t *testing.T) {
	for _, kind := range []string{"store", "network", "permission", "identity"} {
		t.Run(kind, func(t *testing.T) {
			s, _, services := reuseFixture(t)
			before, _, _ := config.ReadFile(s.cfgDir)
			var failure error
			switch kind {
			case "store":
				failure = cerrors.New(cerrors.CategoryConfig, "CREDENTIAL_STORE_INACCESSIBLE", "host keychain unavailable")
				services.Resolve = func(config.Config) (auth.Credential, credstore.Lookup, error) {
					return auth.Credential{}, credstore.Lookup{Keychain: credstore.Failed}, failure
				}
			case "network":
				failure = cerrors.New(cerrors.CategoryNetwork, "NETWORK_ERROR", "network unavailable")
			case "permission":
				failure = cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "access denied")
			case "identity":
				failure = cerrors.New(cerrors.CategoryAuth, "AUTH_IDENTITY_UNAVAILABLE", "authenticated identity could not be established")
			}
			if kind != "store" {
				services.Verify = func(context.Context, config.Config, auth.Credential) (reuse.Account, error) {
					return reuse.Account{}, failure
				}
			}
			_, err := reuseAuthentication(s, reuseOptions{}, services)
			if !errors.Is(err, failure) {
				t.Fatalf("lost diagnostic: %v", err)
			}
			if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
				t.Fatal("failure mutated config")
			}
		})
	}
}

func TestAuthReuseExpiredIdentityIsUnavailable(t *testing.T) {
	s, fake, services := reuseFixture(t)
	fake.accounts = map[string]reuse.Account{}
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || r.State != "unavailable" || r.Changed {
		t.Fatalf("expired credential: %+v %v", r, err)
	}
}

func TestAuthReuseSelfConfigurationConcurrentChangeAndRotation(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.Defaults.ReadOnly = true
	r, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err != nil || !r.Changed {
		t.Fatal("read-only preview blocked", err)
	}
	r, err = reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" {
		t.Fatalf("native self-configuration should remain available: %+v %v", r, err)
	}

	s, _, services = reuseFixture(t)
	verify := services.Verify
	services.Verify = func(ctx context.Context, cfg config.Config, cred auth.Credential) (reuse.Account, error) {
		f, _, _ := config.ReadFile(s.cfgDir)
		f.CurrentContext = "changed-by-user"
		if err := config.WriteFile(s.cfgDir, f); err != nil {
			return reuse.Account{}, err
		}
		return verify(ctx, cfg, cred)
	}
	_, err = reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CONFIG_CHANGED" {
		t.Fatalf("concurrent change: %v", err)
	}

	s, fake, services := reuseFixture(t)
	before, _, _ := config.ReadFile(s.cfgDir)
	resolve := services.Resolve
	calls := 0
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		calls++
		if calls > 1 {
			fake.secrets[auth.AccountKey(reuseBase, "basic")] = "rotated-secret"
		}
		return resolve(cfg)
	}
	_, err = reuseAuthentication(s, reuseOptions{}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_CREDENTIAL_CHANGED" {
		t.Fatalf("credential rotation: %v", err)
	}
	if after, _, _ := config.ReadFile(s.cfgDir); !reflect.DeepEqual(before, after) {
		t.Fatal("rotation changed destination")
	}
}

func TestAuthReuseRejectsTargetOverridesAndUnknownSource(t *testing.T) {
	s, _, services := reuseFixture(t)
	s.resolved.Config.BaseURL = "https://service.example.test/other"
	_, err := reuseAuthentication(s, reuseOptions{DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_TARGET_MISMATCH" {
		t.Fatal(err)
	}
	s.resolved.Config.BaseURL = reuseBase
	_, err = reuseAuthentication(s, reuseOptions{From: "missing", DryRun: true}, services)
	if err == nil || cerrors.AsCLIError(err).Code != "AUTH_REUSE_SOURCE_NOT_FOUND" {
		t.Fatal(err)
	}
}

// Contexts are described with persisted fields and built-in defaults only; an
// environment secret never becomes a source.
func TestAuthReuseUsesPersistedDefaultsWithoutEnvironmentIdentity(t *testing.T) {
	personal := reuseTokenContext("personal", "existing-user")
	personal.Auth.Scheme = ""
	s, fake, services := reuseFixture(t, personal, reuseTokenContext("team", ""))
	fake.secrets[auth.AccountKey(reuseBase, "token")] = "api-token"
	fake.accounts["existing-user|api-token"] = reuse.Account{ID: "existing-user"}
	s.resolved.Secrets = config.Secrets{Token: "environment-secret"}
	r, err := reuseAuthentication(s, reuseOptions{}, services)
	if err != nil || r.State != "reused" || r.SourceContext != "personal" || r.Scheme != "token" {
		t.Fatalf("stored defaults: %+v %v", r, err)
	}
	if got := targetOf(t, s); got.Auth.Username != "existing-user" {
		t.Fatal("identity did not come from stored source")
	}
}

// The command verifies through the native client and store: a password and an
// API token of one account are one login, and the account is Jenkins's id.
func TestAuthReuseCommandVerifiesNativeStoredCredential(t *testing.T) {
	keyring.MockInit()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		user, secret, ok := r.BasicAuth()
		if r.Method != http.MethodGet || !ok || user != "existing-user" || (secret != "retained-secret" && secret != "api-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"existing-user","fullName":"Existing User"}`))
	}))
	defer server.Close()
	base := server.URL + "/deploy"
	personal, team := reuseBasicContext("personal", "existing-user"), reuseTokenContext("team", "")
	personal.BaseURL, team.BaseURL = base, base
	s, _, _ := reuseFixture(t, personal, team)
	s.store = auth.NewStore(s.cfgDir)
	for _, cred := range []auth.Credential{
		{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"},
		{Scheme: "token", Username: "existing-user", Secret: "api-token"},
	} {
		if _, err := auth.Save(base, cred, s.store); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := reuseAuthentication(s, reuseOptions{DryRun: true}, s.reuseServices())
	if err != nil || preview.State != "available" || preview.Account != "existing-user" || preview.AccountName != "Existing User" || preview.Scheme != "token" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	command := newAuthReuseCmd(s)
	if err := command.RunE(command, nil); err != nil {
		t.Fatal(err)
	}
	if requests == 0 {
		t.Fatal("did not verify with the native API client")
	}
	after, _, _ := config.ReadFile(s.cfgDir)
	target, _ := after.Context("team")
	actual, err := auth.Resolve(config.StoredContext(target, after.Defaults), config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || actual.Username != "existing-user" || actual.Secret != "api-token" {
		t.Fatalf("native reuse unavailable after reload: %v", err)
	}
}

func TestAuthReusePreservesLegacyStoreKeyUnderEquivalentOverride(t *testing.T) {
	keyring.MockInit()
	legacy := "https://SERVICE.example.test:443/deploy/"
	personal := reuseBasicContext("personal", "existing-user")
	personal.BaseURL = legacy
	s, _, services := reuseFixture(t, personal, reuseBasicContext("team", ""))
	store := auth.NewStore(s.cfgDir)
	credential := auth.Credential{Scheme: "basic", Username: "existing-user", Secret: "retained-secret"}
	if _, err := auth.Save(legacy, credential, store); err != nil {
		t.Fatal(err)
	}
	services.Resolve = func(cfg config.Config) (auth.Credential, credstore.Lookup, error) {
		return auth.ResolveLookup(cfg, config.Secrets{}, store)
	}
	services.Lookup = store.Lookup
	services.Verify = func(context.Context, config.Config, auth.Credential) (reuse.Account, error) {
		return reuse.Account{ID: "existing-user"}, nil
	}
	if _, err := reuseAuthentication(s, reuseOptions{}, services); err != nil {
		t.Fatal(err)
	}
	fresh, err := config.Load(config.LoadOptions{ConfigDir: s.cfgDir, Context: "team", DotenvPath: filepath.Join(t.TempDir(), "absent"), Flags: config.FlagValues{BaseURL: reuseBase}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := auth.Resolve(fresh.Config, config.Secrets{}, auth.NewStore(s.cfgDir))
	if err != nil || got.Secret != credential.Secret || got.Username != credential.Username {
		t.Fatalf("equivalent override lost native credential: %v", err)
	}
	if fresh.Config.BaseURL != reuseBase {
		t.Fatal("changed request destination")
	}
	if err := auth.ForgetForConfig(fresh.Config, fresh.Config.Auth.Scheme, store); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(auth.AccountKey(legacy, credential.Scheme)); err == nil {
		t.Fatal("logout left the original credential active")
	}
	fresh.Config.BaseURL = "https://service.example.test/other"
	_, err = auth.Resolve(fresh.Config, config.Secrets{}, store)
	if err == nil || cerrors.AsCLIError(err).Code != "CREDENTIAL_SERVICE_MISMATCH" {
		t.Fatalf("foreign deployment reused stored key: %v", err)
	}
}

// A rejected credential leads with reuse when a stored login could replace it.
func TestRejectedCredentialRecoveryLeadsWithReuse(t *testing.T) {
	step := "jenkins-cli --use-context 'team' auth reuse"
	s := &appState{resolved: &config.Resolved{Config: config.Config{BaseURL: reuseBase,
		Auth: config.AuthConfig{Scheme: "token"}, ReuseStep: step}}}
	ce := cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401)
	withReuseRecovery(ce, s, &cobra.Command{Use: "list"})
	if len(ce.NextSteps) < 2 || ce.NextSteps[0] != step {
		t.Fatalf("next steps %v", ce.NextSteps)
	}
	for _, unchanged := range []struct {
		err *cerrors.CLIError
		cmd string
	}{
		{cerrors.New(cerrors.CategoryPermission, "FORBIDDEN", "denied").WithHTTPStatus(403), "list"},
		{cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401), "reuse"},
		{cerrors.New(cerrors.CategoryAuth, "HTTP_UNAUTHORIZED", "rejected").WithHTTPStatus(401), "login"},
	} {
		steps := append([]string(nil), unchanged.err.NextSteps...)
		withReuseRecovery(unchanged.err, s, &cobra.Command{Use: unchanged.cmd})
		if !reflect.DeepEqual(steps, unchanged.err.NextSteps) {
			t.Fatalf("%s %s gained a reuse step: %v", unchanged.cmd, unchanged.err.Code, unchanged.err.NextSteps)
		}
	}
}
