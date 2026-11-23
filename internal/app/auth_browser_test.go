package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/agent-cli-kit/terminal"
	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/internal/config"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

func TestBrowserLoginFromPasswordContext(t *testing.T) {
	for _, tc := range []struct{ name, response, code string }{
		{"verified", `{"id":"sso-user"}`, ""},
		{"anonymous", `{"id":"anonymous"}`, "AUTH_IDENTITY_UNAVAILABLE"},
		{"other user", `{"id":"other-user"}`, "AUTH_IDENTITY_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := isolateConfigInit(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, secret, ok := r.BasicAuth()
				if !ok || user != "sso-user" || secret != "issued-token" || r.URL.Path != "/ci/me/api/json" {
					t.Errorf("unexpected identity probe at %s", r.URL.Path)
					http.Error(w, "denied", 401)
					return
				}
				_, _ = io.WriteString(w, tc.response)
			}))
			defer server.Close()
			base := server.URL + "/ci"
			team := config.NamedContext{Name: "team", BaseURL: base, Auth: config.AuthConfig{
				Username: "password-user", Scheme: "basic", CredentialURL: "https://help.example.test/guide",
			}}
			personal := team
			personal.Name = "personal"
			before := config.File{CurrentContext: personal.Name, Contexts: []config.NamedContext{team, personal}}
			if err := config.WriteFile(dir, before); err != nil {
				t.Fatal(err)
			}
			s := &appState{gflags: globalFlags{configPath: dir, useContext: team.Name}}
			if err := s.load(); err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Save(base, auth.Credential{Scheme: "basic", Username: team.Auth.Username, Secret: "old-password"}, s.store); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			prompt := terminal.Prompter{In: strings.NewReader("sso-user\nissued-token\n"), Out: &output}
			opened := ""
			err := interactiveLogin(s, true, prompt, func(url string) error {
				opened = url
				return errors.New("desktop unavailable")
			})
			if opened != base+"/me/security" || !strings.Contains(output.String(), "manually") ||
				!strings.Contains(output.String(), "Jenkins user ID [password-user]") || strings.Contains(output.String(), "issued-token") {
				t.Fatalf("unexpected browser handoff: %s, %s", opened, output.String())
			}
			after, _, readErr := config.ReadFile(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.code != "" {
				if err == nil || cerrors.AsCLIError(err).Code != tc.code || !reflect.DeepEqual(before, after) {
					t.Fatalf("unexpected failed login: %v, config %+v", err, after)
				}
				if _, err := s.store.Load(auth.AccountKey(base, "token")); err == nil {
					t.Fatal("failed verification stored token")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			target, _ := after.Context("team")
			if target.Auth.Scheme != "token" || target.Auth.Username != "sso-user" || target.Auth.CredentialURL != team.Auth.CredentialURL || after.CurrentContext != before.CurrentContext {
				t.Fatalf("unexpected migrated context: %+v", after)
			}
			for name, want := range map[string]string{"team": "issued-token", "personal": "old-password"} {
				if got, err := resolveStoredSecret(t, dir, name); err != nil || got != want {
					t.Fatalf("unexpected stored credential for %s, err=%v", name, err)
				}
			}
		})
	}
}

func TestLoginPreservesAnotherIdentityAndConcurrentEdits(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		s, cfg, cred := loginStateForTest(t)
		other := config.NamedContext{Name: "other", BaseURL: cfg.BaseURL, Auth: config.AuthConfig{Scheme: cred.Scheme, Username: "other-user"}}
		existing := config.File{CurrentContext: "other", Contexts: []config.NamedContext{other}}
		saveOther := func() {
			if err := config.WriteFile(s.cfgDir, existing); err != nil {
				t.Fatal(err)
			}
		}
		if !concurrent {
			saveOther()
		}
		_, err := completeLogin(s, cfg, cred, loginServices{Verify: func(config.Config, auth.Credential) error {
			if concurrent {
				saveOther()
			}
			return nil
		}})
		want := "LOGIN_CREDENTIAL_SHARED"
		if concurrent {
			want = "LOGIN_CONFIG_CHANGED"
		}
		if err == nil || cerrors.AsCLIError(err).Code != want {
			t.Fatalf("concurrent=%t, want %s, got %v", concurrent, want, err)
		}
	}
}

func TestBrowserLoginOnNewServer(t *testing.T) {
	dir := isolateConfigInit(t)
	server := jenkinsIdentityStub(t)
	s := &appState{gflags: globalFlags{configPath: dir, baseURL: server.URL}}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	prompt := terminal.Prompter{In: strings.NewReader("first-user\nfirst-token\n"), Out: io.Discard}
	if err := interactiveLogin(s, true, prompt, func(page string) error {
		if page != server.URL+"/me/security" {
			t.Fatalf("opened %q", page)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveStoredSecret(t, dir, "default"); err != nil || got != "first-token" {
		t.Fatalf("could not reload token: %v", err)
	}

	// A flag pointing a configured context at a different deployment must
	// fail before opening a browser or reading any input.
	s.gflags.baseURL = server.URL + "/other"
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	err := interactiveLogin(s, true, terminal.Prompter{}, func(string) error {
		t.Fatal("opened a mismatched service")
		return nil
	})
	if err == nil || cerrors.AsCLIError(err).Code != "CONTEXT_BASE_URL_MISMATCH" {
		t.Fatalf("wrong target error: %v", err)
	}
}

func TestBrowserLoginMigratesSharedTokenAliasesWithConsent(t *testing.T) {
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			dir := isolateConfigInit(t)
			server := jenkinsIdentityStub(t)
			selected := config.NamedContext{Name: "default", BaseURL: server.URL, Auth: config.AuthConfig{Scheme: "token", Username: "previous"}}
			alias := selected
			alias.Name = "team"
			other := selected
			other.Name, other.Auth.Scheme = "password", "basic"
			before := config.File{CurrentContext: "default", Contexts: []config.NamedContext{selected, alias, other}}
			if err := config.WriteFile(dir, before); err != nil {
				t.Fatal(err)
			}
			s := &appState{gflags: globalFlags{configPath: dir}}
			if err := s.load(); err != nil {
				t.Fatal(err)
			}
			if _, err := auth.Save(server.URL, auth.Credential{Scheme: "token", Username: "previous", Secret: "old-token"}, s.store); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			prompt := terminal.Prompter{In: strings.NewReader("mapped\nnew-token\n" + answer + "\n"), Out: &output}
			err := interactiveLogin(s, true, prompt, func(string) error { return nil })
			if !strings.Contains(output.String(), "contexts sharing this token (team)") {
				t.Fatalf("unexpected confirmation: %s", output.String())
			}
			after, _, readErr := config.ReadFile(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantSecret := "old-token"
			if answer == "n" {
				if err == nil || cerrors.AsCLIError(err).Code != "LOGIN_CREDENTIAL_SHARED" || !reflect.DeepEqual(before, after) {
					t.Fatalf("declined migration changed config: %v %+v", err, after)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				wantSecret = "new-token"
				if after.Contexts[0].Auth.Username != "mapped" || after.Contexts[1].Auth.Username != "mapped" || after.Contexts[2] != other || after.CurrentContext != "default" {
					t.Fatalf("unexpected alias migration: %+v", after)
				}
			}
			if got, err := resolveStoredSecret(t, dir, "team"); err != nil || got != wantSecret {
				t.Fatalf("unexpected shared credential, err=%v", err)
			}
		})
	}
}

func TestBrowserLoginKeepsContextDeletedDuringSignIn(t *testing.T) {
	dir := isolateConfigInit(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("verification contacted a service removed during sign-in")
	}))
	defer server.Close()
	selected := config.NamedContext{Name: "default", BaseURL: server.URL, Auth: config.AuthConfig{Scheme: "basic", Username: "old"}}
	other := config.NamedContext{Name: "other", BaseURL: server.URL + "/other"}
	before := config.File{CurrentContext: selected.Name, Contexts: []config.NamedContext{selected, other}}
	if err := config.WriteFile(dir, before); err != nil {
		t.Fatal(err)
	}
	s := &appState{gflags: globalFlags{configPath: dir, useContext: "default"}}
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	after := config.File{CurrentContext: other.Name, Contexts: []config.NamedContext{other}}
	prompt := terminal.Prompter{In: strings.NewReader("mapped\nnew-token\n"), Out: io.Discard}
	err := interactiveLogin(s, true, prompt, func(string) error {
		if err := config.WriteFile(dir, after); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Code != "LOGIN_CONFIG_CHANGED" || !strings.Contains(strings.Join(ce.NextSteps, " "), "auth login --browser") {
		t.Fatalf("lost the changed-config outcome or the original sign-in mode: %v", err)
	}
	got, _, readErr := config.ReadFile(dir)
	if readErr != nil || !reflect.DeepEqual(got, after) {
		t.Fatalf("stale login recreated deleted context: %+v %v", got, readErr)
	}
	if _, err := s.store.Load(auth.AccountKey(server.URL, "token")); err == nil {
		t.Fatal("stale login stored token")
	}
}

func TestLoginPreservesEditsDuringCredentialSave(t *testing.T) {
	s, cfg, cred := loginStateForTest(t)
	plan, err := planLogin(s, cfg, cred)
	if err != nil {
		t.Fatal(err)
	}
	plan.retryStep = "jenkins-cli auth login --browser"
	changed := config.File{CurrentContext: "another", Contexts: []config.NamedContext{{Name: "another", BaseURL: "https://another.test"}}}
	_, err = plan.complete(s, cfg, cred, loginServices{
		Verify: func(config.Config, auth.Credential) error { return nil },
		Save: func(string, auth.Credential) (string, error) {
			return "keychain", config.WriteFile(s.cfgDir, changed)
		},
		Write: func(string, config.File) error {
			t.Fatal("overwrote concurrent change after keychain save")
			return nil
		},
	})
	ce := cerrors.AsCLIError(err)
	if err == nil || ce.Code != "LOGIN_CONFIG_WRITE_FAILED" ||
		ce.Details.(map[string]any)["credential_stored"] != true || !strings.Contains(strings.Join(ce.NextSteps, " "), "auth login --browser") {
		t.Fatalf("lost partial write outcome or recovery: %+v", ce)
	}
}

func TestConfigInitProtectsRetainedCredentialSlots(t *testing.T) {
	dir := isolateConfigInit(t)
	server := jenkinsIdentityStub(t)
	scriptedConfigInit(t, dir, "personal", server.URL, "alice", "token", "alice-token")
	scriptedConfigInit(t, dir, "team", server.URL, "alice", "token", "alice-token")
	before, _, err := config.ReadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = scriptedConfigInitResult(t, dir, "team", server.URL, "bob", "token", "bob-token")
	if err == nil || cerrors.AsCLIError(err).Code != "LOGIN_CREDENTIAL_SHARED" {
		t.Fatalf("wizard failed to protect retained personal context: %v", err)
	}
	after, _, err := config.ReadFile(dir)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("wizard changed conflicting context: %+v %v", after, err)
	}
	if got, err := resolveStoredSecret(t, dir, "personal"); err != nil || got != "alice-token" {
		t.Fatal("wizard overwrote the retained user's token")
	}
	// Replace explicitly discards the previous contexts. No retained context
	// resolves Alice's token, so Bob can legitimately use that slot.
	scriptedConfigInit(t, dir, "", "replace", server.URL, "bob", "token", "bob-token")
	if got, err := resolveStoredSecret(t, dir, "default"); err != nil || got != "bob-token" {
		t.Fatal("explicit replace did not configure the new identity")
	}
}
