package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestFileFormat pins the bytes of config.yaml. The engine is shared; the
// shape is jenkins-cli's, and existing files must keep reading and writing
// the same way.
func TestFileFormat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	in := File{
		CurrentContext: "prod",
		Contexts: []NamedContext{
			{Name: "default", BaseURL: "http://localhost:8080", Auth: AuthConfig{Scheme: SchemeToken, Username: "alice"}},
			{Name: "prod", BaseURL: "https://ci.example.com/jenkins", Auth: AuthConfig{Scheme: SchemeBasic, Username: "bob",
				CredentialURL: "https://ci.example.com/jenkins/me/security"}},
		},
		Defaults: Defaults{Format: "table", Timeout: 90 * time.Second, MaxRetries: 5, ReadOnly: true},
	}
	if err := WriteFile(dir, in); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(ConfigFilePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	const want = `current_context: prod
contexts:
    - name: default
      server: http://localhost:8080
      auth:
        scheme: token
        username: alice
    - name: prod
      server: https://ci.example.com/jenkins
      auth:
        credential_url: https://ci.example.com/jenkins/me/security
        scheme: basic
        username: bob
defaults:
    format: table
    timeout: 1m30s
    max_retries: 5
    read_only: true
`
	if string(raw) != want {
		t.Errorf("config.yaml changed:\n%s", raw)
	}
	got, ok, err := ReadFile(dir)
	if err != nil || !ok || !reflect.DeepEqual(got, in) {
		t.Errorf("read back %+v, %v, %v", got, ok, err)
	}
}

// TestSchema pins jenkins-cli's environment variables, defaults and scheme
// inference.
func TestSchema(t *testing.T) {
	cleanSetupEnv(t)
	dir := t.TempDir()
	load := func() *Resolved {
		t.Helper()
		r, err := Load(LoadOptions{ConfigDir: dir, DotenvPath: filepath.Join(dir, "absent.env")})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := load()
	want := Config{Auth: AuthConfig{Scheme: SchemeToken}, Defaults: Defaults{Format: "json", Timeout: 30 * time.Second, MaxRetries: 3}}
	if !reflect.DeepEqual(r.Config, want) || r.Sources[FieldAuthScheme] != "default" {
		t.Fatalf("defaults = %+v (%v)", r.Config, r.Sources)
	}

	t.Setenv("JENKINS_URL", "jenkins.example.com:8080")
	t.Setenv("JENKINS_USER", "alice")
	t.Setenv("JENKINS_CREDENTIAL_URL", "https://help.example.com/token")
	t.Setenv("JENKINS_FORMAT", "table")
	t.Setenv("JENKINS_CLI_READ_ONLY", "yes")
	t.Setenv("JENKINS_PASSWORD", "pw")
	r = load()
	if r.Config.BaseURL != "jenkins.example.com:8080" || r.Config.Auth.Username != "alice" || r.Config.Auth.Scheme != SchemeBasic ||
		r.Config.Auth.CredentialURL != "https://help.example.com/token" || r.Config.Defaults.Format != "table" ||
		!r.Config.Defaults.ReadOnly || r.Secrets != (Secrets{Password: "pw"}) {
		t.Fatalf("environment = %+v %+v", r.Config, r.Secrets)
	}
	t.Setenv("JENKINS_TOKEN", "tok")
	if r = load(); r.Config.Auth.Scheme != SchemeToken {
		t.Errorf("a token wins over a password: %q", r.Config.Auth.Scheme)
	}
	t.Setenv("JENKINS_AUTH_SCHEME", SchemeBasic)
	if r = load(); r.Config.Auth.Scheme != SchemeBasic {
		t.Errorf("an explicit scheme wins: %q", r.Config.Auth.Scheme)
	}
	if base, err := NormalizeServiceURL(r.Config.BaseURL); err != nil || base != "http://jenkins.example.com:8080" {
		t.Errorf("a bare host gets http://: %q %v", base, err)
	}
}
