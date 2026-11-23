package app

import (
	"context"
	"errors"
	"reflect"
	"slices"

	kitauth "github.com/angelmsger/agent-cli-kit/auth"
	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/internal/config"
	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// loginServices keeps verification and persistence independently testable.
type loginServices struct {
	Verify         func(config.Config, auth.Credential) error
	Save           func(string, auth.Credential) (string, error)
	Write          func(string, config.File) error
	ConfirmAliases func([]string) (bool, error)
}

func (s *appState) loginServices() loginServices {
	return loginServices{Verify: func(cfg config.Config, cred auth.Credential) error { return verifyCredential(s, cfg, cred) },
		Save: func(base string, cred auth.Credential) (string, error) { return auth.Save(base, cred, s.store) }, Write: config.WriteFile}
}

type loginPlan struct {
	before, after config.File
	name          string
	retryStep     string
}

// planLogin checks the complete service identity before any verification or secret write.
func planLogin(s *appState, cfg config.Config, cred auth.Credential) (loginPlan, error) {
	base, err := config.NormalizeServiceURL(cfg.BaseURL)
	if err != nil {
		return loginPlan{}, err
	}
	file, _, err := config.ReadFile(s.cfgDir)
	if err != nil {
		return loginPlan{}, cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read login target")
	}
	name := s.resolved.ActiveContext
	if name == "" {
		name = config.DefaultContextName
	}
	nc, found := file.Context(name)
	if found {
		name = nc.Name
	}
	if found && nc.BaseURL != "" {
		previous, e := config.NormalizeServiceURL(nc.BaseURL)
		if e != nil || previous != base {
			return loginPlan{}, cerrors.New(cerrors.CategoryConfig, "CONTEXT_BASE_URL_MISMATCH", "login target differs from the configured context; no credential was stored").WithDetails(map[string]string{"context": name, "server": base}).WithHint("Select or create a context for the intended service before logging in.").WithNextSteps(constants.AppName+" config set-context <name> --base-url <url>", constants.AppName+" --use-context <name> auth login")
		}
	}
	if !found {
		nc = config.NamedContext{Name: name}
	}
	if nc.BaseURL == "" {
		// Preserve legacy keychain lookup for bare-host input whose normalized key differs.
		nc.BaseURL = base
		if auth.AccountKey(base, cred.Scheme) != auth.AccountKey(cfg.BaseURL, cred.Scheme) {
			nc.BaseURL = cfg.BaseURL
		}
	}
	// Equivalent URL spellings can still have different legacy keychain keys.
	// Record the spelling used by resolution so login also works after env overrides disappear.
	if auth.AccountKey(nc.BaseURL, cred.Scheme) != auth.AccountKey(cfg.BaseURL, cred.Scheme) {
		nc.BaseURL = cfg.BaseURL
	}
	nc.Auth.Scheme = cred.Scheme
	nc.Auth.Username = cred.Username
	if !found {
		nc.Auth.CredentialURL = cfg.Auth.CredentialURL
	}
	return newLoginPlan(file, nc, constants.AppName+" auth login"), nil
}

func newLoginPlan(file config.File, target config.NamedContext, retryStep string) loginPlan {
	next := file
	next.Contexts = slices.Clone(file.Contexts)
	next.Upsert(target)
	if next.CurrentContext == "" {
		next.CurrentContext = target.Name
	}
	return loginPlan{before: file, after: next, name: target.Name, retryStep: retryStep}
}

func (p *loginPlan) checkConfig(dir string) error {
	current, _, err := config.ReadFile(dir)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to recheck login configuration")
	}
	if !reflect.DeepEqual(p.before, current) {
		return cerrors.New(cerrors.CategoryConflict, "LOGIN_CONFIG_CHANGED", "login configuration changed during sign-in").
			WithNextSteps(constants.AppName+" config get-contexts", p.retryStep)
	}
	return nil
}

// complete uses the target snapshot taken before prompting. Verification,
// alias confirmation, and host keychain access can each take time.
func (p *loginPlan) complete(s *appState, cfg config.Config, cred auth.Credential, services loginServices) (string, error) {
	if err := cred.Validate(); err != nil {
		return "", err
	}
	if err := p.checkConfig(s.cfgDir); err != nil {
		return "", err
	}
	if err := services.Verify(cfg, cred); err != nil {
		return "", err
	}
	target, _ := p.after.Context(p.name)
	target.Auth.Username = cred.Username
	p.after.Upsert(target)
	if err := p.sharedCredentials(cfg, cred, services.ConfirmAliases); err != nil {
		return "", err
	}
	if err := p.checkConfig(s.cfgDir); err != nil {
		return "", err
	}
	backend, err := services.Save(cfg.BaseURL, cred)
	if err != nil {
		return "", credentialSaveFailure(err, backend, p.retryStep)
	}
	err = p.checkConfig(s.cfgDir)
	if err == nil && !reflect.DeepEqual(p.before, p.after) {
		err = services.Write(s.cfgDir, p.after)
	}
	if err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryConfig, "LOGIN_CONFIG_WRITE_FAILED", "credential stored, but login configuration could not be confirmed").
			WithDetails(map[string]any{"context": p.name, "server": cfg.BaseURL, "scheme": cred.Scheme, "credential_stored": true, "stored_in": backend}).
			WithHint("Check the context and config-directory access, then repeat the same sign-in with the verified credential.").
			WithNextSteps(constants.AppName+" config get-contexts", p.retryStep)
	}
	return backend, nil
}

// The store has one secret per host and scheme. A user ID change on a
// shared token needs the same change to aliases of the previous identity.
func (p *loginPlan) sharedCredentials(cfg config.Config, cred auth.Credential, confirm func([]string) (bool, error)) error {
	key := auth.AccountKey(cfg.BaseURL, cred.Scheme)
	service := config.ReuseService(cfg)
	previous, found := p.before.Context(p.name)
	aliases := []string{}
	for _, other := range config.ReuseContexts(p.after) {
		if other.Name == p.name || auth.AccountKey(other.KeyBase, other.Scheme) != key {
			continue
		}
		if other.Service == service && (other.Username == "" || other.Username == cred.Username) {
			continue
		}
		if confirm == nil || other.Service != service || !found || credentialKey(previous) != key || previous.Auth.Username != other.Username {
			return cerrors.New(cerrors.CategoryConflict, "LOGIN_CREDENTIAL_SHARED", "another context uses this credential slot for a different service or user ID").
				WithDetails(map[string]string{"context": p.name, "other_context": other.Name, "scheme": cred.Scheme}).
				WithHint("Use per-invocation environment credentials for different identities on the same Jenkins host.").
				WithNextSteps("jenkins-cli config get-contexts")
		}
		aliases = append(aliases, other.Name)
	}
	if len(aliases) == 0 {
		return nil
	}
	ok, err := confirm(aliases)
	if err != nil {
		return err
	}
	if !ok {
		return cerrors.New(cerrors.CategoryConflict, "LOGIN_CREDENTIAL_SHARED", "shared user ID migration declined before saving the token").
			WithNextSteps("jenkins-cli config get-contexts")
	}
	for _, name := range aliases {
		c, _ := p.after.Context(name)
		c.Auth.Username = cred.Username
		p.after.Upsert(c)
	}
	return nil
}

// Browser login asks for Jenkins' canonical ID, so a proxy authenticating
// every API request as a different account cannot validate the submitted token.
func (s *appState) verifyBrowserToken(cfg config.Config, cred auth.Credential) error {
	user, err := verifyCredentialWithin(context.Background(), s, cfg, cred)
	if err != nil {
		return err
	}
	if user.ID != cred.Username {
		return cerrors.New(cerrors.CategoryAuth, "AUTH_IDENTITY_MISMATCH", "Jenkins returned a different user ID from the one entered").
			WithHint("Check the signed-in Jenkins user ID and the account that owns the API token.").
			WithNextSteps("jenkins-cli auth login --browser")
	}
	return nil
}

func verifyCredential(s *appState, cfg config.Config, cred auth.Credential) error {
	_, err := verifyCredentialWithin(context.Background(), s, cfg, cred)
	return err
}

// verifyCredentialWithin verifies cred under parent, which may carry an earlier
// deadline than the request timeout, and returns the authenticated user.
func verifyCredentialWithin(parent context.Context, s *appState, cfg config.Config, cred auth.Credential) (*apiclient.User, error) {
	if err := cred.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, s.timeout())
	defer cancel()
	client, err := apiclient.BuildClient(apiclient.BuildParams{BaseURL: cfg.BaseURL, AuthDecorator: cred.Decorator(), Timeout: s.timeout(), MaxRetries: cfg.Defaults.MaxRetries, Verbose: s.verboseLog()})
	if err != nil {
		return nil, err
	}
	return client.WhoAmI(ctx)
}

// credentialSaveFailure preserves a successful backend write when cleanup of
// another store failed. Login must not persist a new identity after this error.
func credentialSaveFailure(cause error, backend, retryStep string) error {
	message := "credentials verified but storage could not be completed"
	hint := "The verified credential was not stored. Restore host access to the OS keychain and config directory, then retry auth login with the same credential."
	if backend != "" {
		message = "credential stored, but credential-store cleanup failed"
		hint = "The verified credential was stored, but another credential copy could not be removed. Restore host access to the OS keychain and config directory, then retry auth login with the same credential."
	}
	return cerrors.Wrap(cause, cerrors.CategoryConfig, "CREDENTIAL_SAVE_FAILED", message).
		WithDetails(map[string]any{"credential_stored": backend != "", "stored_in": backend}).
		WithHint(hint).
		WithRecovery(kitauth.HostRecovery()).
		WithNextSteps("Restore credential-store access on the host and repeat sign-in with the same verified credential.", retryStep)
}

func credentialLogoutFailure(cause error) error {
	// Service-scope guards run before store access and retain their own recovery.
	var existing *cerrors.CLIError
	if errors.As(cause, &existing) {
		return cause
	}
	return cerrors.Wrap(cause, cerrors.CategoryConfig, "LOGOUT_FAILED", "stored credential removal could not be completed").
		WithHint("Restore host access to the OS keychain and config directory, then retry auth logout. Do not acquire a replacement credential.").
		WithRecovery(kitauth.HostRecovery()).
		WithNextSteps("Retry auth logout on the host after restoring credential-store access.")
}
