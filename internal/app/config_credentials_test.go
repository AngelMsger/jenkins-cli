package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/internal/config"
	"github.com/zalando/go-keyring"
)

// A stored secret is keyed by the server's host and the scheme, so every
// context on one Jenkins shares it — a team preset and the personal context
// `auth reuse` associates with it in particular. `config init` therefore only
// ever saves a secret: it has no cleanup step that could delete one another
// context, or the edited context itself, still resolves. These tests pin that.

// jenkinsIdentityStub answers the identity probe `config init` verifies a
// credential with, for any username and secret.
func jenkinsIdentityStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if !ok || r.URL.Path != "/me/api/json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"`+user+`","fullName":"Test User"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// isolateConfigInit keeps a developer's environment, `.env` and keychain out
// of a test that runs the real command tree.
func isolateConfigInit(t *testing.T) string {
	t.Helper()
	keyring.MockInit()
	for _, name := range []string{
		"JENKINS_URL", "JENKINS_USER", "JENKINS_TOKEN", "JENKINS_PASSWORD",
		"JENKINS_AUTH_SCHEME", "JENKINS_CREDENTIAL_URL", "JENKINS_CONTEXT",
		"JENKINS_FORMAT", "JENKINS_CLI_READ_ONLY",
	} {
		t.Setenv(name, "")
	}
	t.Chdir(t.TempDir())
	return t.TempDir()
}

// scriptedConfigInit runs the plain `config init` wizard for one context,
// answering its prompts (server URL, username, scheme, secret) from answers.
func scriptedConfigInit(t *testing.T, cfgDir, context string, answers ...string) {
	t.Helper()
	stdin, feed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := feed.WriteString(strings.Join(answers, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = feed.Close()
	sink, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = stdin, sink, sink
	defer func() {
		os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
		_ = stdin.Close()
		_ = sink.Close()
	}()
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--config", cfgDir, "config", "init", "--context", context})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config init --context %s: %v", context, err)
	}
}

// resolveStoredSecret loads a context the way a fresh process does and
// resolves its secret from the store alone.
func resolveStoredSecret(t *testing.T, cfgDir, context string) (string, error) {
	t.Helper()
	resolved, err := config.Load(config.LoadOptions{ConfigDir: cfgDir, Context: context})
	if err != nil {
		t.Fatal(err)
	}
	cred, err := auth.Resolve(resolved.Config, resolved.Secrets, auth.NewStore(cfgDir))
	return cred.Secret, err
}

// Re-running the wizard on a context and changing only how the server URL is
// spelled must not delete the credential it has just saved.
func TestConfigInitKeepsTheCredentialWhenOnlyTheURLSpellingChanges(t *testing.T) {
	cfgDir := isolateConfigInit(t)
	srv := jenkinsIdentityStub(t)

	scriptedConfigInit(t, cfgDir, "default", srv.URL, "alice", "token", "first-token")
	if got, err := resolveStoredSecret(t, cfgDir, "default"); err != nil || got != "first-token" {
		t.Fatalf("fresh setup did not store the credential: %q %v", got, err)
	}
	for _, spelling := range []string{srv.URL + "/", strings.Replace(srv.URL, "http://", "HTTP://", 1) + "//"} {
		scriptedConfigInit(t, cfgDir, "default", spelling, "alice", "token", "edited-token")
		if got, err := resolveStoredSecret(t, cfgDir, "default"); err != nil || got != "edited-token" {
			t.Fatalf("editing the URL as %q lost the credential just saved: %q %v", spelling, got, err)
		}
	}
	file, _, err := config.ReadFile(cfgDir)
	if err != nil || len(file.Contexts) != 1 || file.Contexts[0].BaseURL != srv.URL {
		t.Fatalf("an equivalent spelling changed the stored context: %+v %v", file, err)
	}
}

// Moving one context to another server, or replacing its credential, must
// leave the secret that a second context on the old server still resolves.
func TestConfigInitKeepsACredentialAnotherContextUses(t *testing.T) {
	cfgDir := isolateConfigInit(t)
	shared, elsewhere := jenkinsIdentityStub(t), jenkinsIdentityStub(t)

	scriptedConfigInit(t, cfgDir, "default", shared.URL, "alice", "token", "shared-token")
	// A team preset on the same server resolves the personal context's secret.
	file, _, err := config.ReadFile(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	file.Upsert(config.NamedContext{Name: "team", BaseURL: shared.URL + "/", Auth: config.AuthConfig{Scheme: "token", Username: "alice"}})
	if err := config.WriteFile(cfgDir, file); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveStoredSecret(t, cfgDir, "team"); err != nil || got != "shared-token" {
		t.Fatalf("contexts on one server do not share a credential: %q %v", got, err)
	}

	scriptedConfigInit(t, cfgDir, "default", elsewhere.URL, "alice", "token", "moved-token")
	if got, err := resolveStoredSecret(t, cfgDir, "default"); err != nil || got != "moved-token" {
		t.Fatalf("the moved context lost its new credential: %q %v", got, err)
	}
	if got, err := resolveStoredSecret(t, cfgDir, "team"); err != nil || got != "shared-token" {
		t.Fatalf("moving a context deleted the credential another context uses: %q %v", got, err)
	}
}
