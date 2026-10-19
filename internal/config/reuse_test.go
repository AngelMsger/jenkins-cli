// An external test package: the auth schemes reuse depends on are registered
// by internal/auth, which imports this package.
package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/angelmsger/jenkins-cli/internal/auth" // registers the auth schemes
	"github.com/angelmsger/jenkins-cli/internal/config"
)

// clearEnv keeps a developer's JENKINS_* variables out of these loads.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"JENKINS_URL", "JENKINS_CREDENTIAL_URL", "JENKINS_AUTH_SCHEME", "JENKINS_USER",
		"JENKINS_FORMAT", "JENKINS_CLI_READ_ONLY", "JENKINS_TOKEN", "JENKINS_PASSWORD", "JENKINS_CONTEXT"} {
		t.Setenv(key, "")
	}
}

func TestLoadOffersReuseBeforeSignIn(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	file := config.File{CurrentContext: "team", Contexts: []config.NamedContext{
		{Name: "personal", BaseURL: "https://jenkins.example.test", Auth: config.AuthConfig{Scheme: config.SchemeBasic, Username: "alice"}},
		{Name: "team", BaseURL: "https://jenkins.example.test", Auth: config.AuthConfig{Scheme: config.SchemeToken}},
	}}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	load := func(opt config.LoadOptions) *config.Resolved {
		t.Helper()
		opt.ConfigDir, opt.DotenvPath = dir, filepath.Join(t.TempDir(), "absent.env")
		r, err := config.Load(opt)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load(config.LoadOptions{})
	if r.Config.ReuseStep != "jenkins-cli --use-context 'team' auth reuse" {
		t.Fatalf("reuse step %q", r.Config.ReuseStep)
	}
	g, err := config.Guide(r.Config, r.Sources)
	if err != nil || g.NextSteps[0] != r.Config.ReuseStep || !strings.Contains(g.Instructions[0], "auth reuse") {
		t.Fatalf("guide %+v %v", g, err)
	}
	if r = load(config.LoadOptions{Context: "personal"}); r.Config.ReuseStep != "" {
		t.Fatalf("a context with an identity was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{AuthScheme: config.SchemeToken}}); r.Config.ReuseStep != "" {
		t.Fatalf("a supplied scheme without same-scheme sources was offered reuse: %q", r.Config.ReuseStep)
	}
	if r = load(config.LoadOptions{Flags: config.FlagValues{BaseURL: "https://other.example.test"}}); r.Config.ReuseStep != "" {
		t.Fatalf("an override was offered reuse: %q", r.Config.ReuseStep)
	}
	t.Setenv("JENKINS_USER", "bob")
	if r = load(config.LoadOptions{}); r.Config.ReuseStep != "" {
		t.Fatalf("a runtime username was offered reuse: %q", r.Config.ReuseStep)
	}
}

// The scheme a preset supplies becomes the context's own, so the setup plan
// still leads with reuse of a login under the other scheme.
func TestSetupPlanLeadsWithReuse(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	file := config.File{Contexts: []config.NamedContext{
		{Name: "personal", BaseURL: "https://jenkins.example.test", Auth: config.AuthConfig{Scheme: config.SchemeBasic, Username: "alice"}},
	}}
	if err := config.WriteFile(dir, file); err != nil {
		t.Fatal(err)
	}
	resolved, err := config.Load(config.LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(t.TempDir(), "absent.env"), Context: "team", Setup: true,
		Flags: config.FlagValues{BaseURL: "https://jenkins.example.test", AuthScheme: config.SchemeToken}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := config.PlanServiceContext(file, "team", resolved, false, false)
	if err != nil || len(plan.NextSteps) == 0 || plan.NextSteps[0] != "jenkins-cli --use-context 'team' auth reuse" {
		t.Fatalf("plan %+v %v", plan.NextSteps, err)
	}
}
