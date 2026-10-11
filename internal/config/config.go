// Package config resolves CLI configuration from layered sources: CLI flags,
// environment variables, a .env file, a YAML config file and built-in
// defaults, in that precedence order (highest first).
//
// The engine — layering, provenance, context selection, the atomic file
// write and team presets — comes from agent-cli-kit/config. This package
// registers jenkins-cli's fields, environment variables, file location and
// command wording with it (schema.go), keeps the YAML shape of the file
// (file.go), and keeps the names the rest of the repo uses.
//
// Secrets (passwords, tokens) are never stored in the YAML config file. They
// are surfaced through Resolved.Secrets when supplied via flags/env/.env, or
// loaded from the OS keychain by the auth package.
package config

import (
	"time"

	kit "github.com/angelmsger/agent-cli-kit/config"
)

// Auth scheme values.
const (
	// SchemeToken is the recommended Jenkins credential: a username plus an API
	// token, combined into an HTTP Basic Authorization header.
	SchemeToken = "token"
	// SchemeBasic is username+password HTTP Basic auth (same wire format as
	// SchemeToken; kept for users who authenticate with a password).
	SchemeBasic = "basic"
)

// DefaultContextName is the name given to an unnamed context.
const DefaultContextName = kit.DefaultContextName

// Context selection sources, reported on Resolved.ContextSource. The first two
// are explicit (the caller named a context for this invocation); the rest are
// implicit (the CLI fell back to a stored or sole context).
const (
	ContextSourceFlag    = kit.ContextSourceFlag    // --use-context
	ContextSourceEnv     = kit.ContextSourceEnv     // JENKINS_CONTEXT
	ContextSourceCurrent = kit.ContextSourceCurrent // the file's current_context
	ContextSourceSingle  = kit.ContextSourceSingle  // the sole defined context
	ContextSourceDefault = kit.ContextSourceDefault // a context literally named "default"
	ContextSourceNone    = kit.ContextSourceNone    // nothing selected (no/empty config)
)

// NamedContext is one named Jenkins server profile inside the config file.
// Runtime defaults are shared across contexts and live in File.Defaults.
type NamedContext struct {
	Name    string `config:"name"`
	BaseURL string `config:"server"`
	Auth    AuthConfig
}

// Config holds the resolved, non-secret configuration.
type Config struct {
	// CredentialBaseURL preserves an equivalent stored URL's native lookup key.
	// It is runtime-only and never changes the request destination.
	CredentialBaseURL string     `yaml:"-" json:"-"`
	BaseURL           string     `yaml:"server" config:"server"`
	Auth              AuthConfig `yaml:"auth"`
	Defaults          Defaults   `yaml:"defaults"`
}

// AuthConfig holds non-secret auth settings. Username is the Jenkins login the
// API token (or password) belongs to.
type AuthConfig struct {
	CredentialURL string `yaml:"credential_url,omitempty" config:"auth.credential_url"`
	Scheme        string `yaml:"scheme" config:"auth.scheme"`
	Username      string `yaml:"username,omitempty" config:"auth.username"`
}

// Defaults holds tunable runtime defaults.
type Defaults struct {
	Format     string        `yaml:"format" config:"defaults.format"`
	Timeout    time.Duration `yaml:"timeout" config:"defaults.timeout"`
	MaxRetries int           `yaml:"max_retries" config:"defaults.max_retries"`
	// ReadOnly blocks every mutating client method. Settable from the config
	// file, from JENKINS_CLI_READ_ONLY, or temporarily overridden via
	// --allow-writes.
	ReadOnly bool `yaml:"read_only,omitempty" config:"defaults.read_only"`
}

// Secrets holds credentials observed in non-file layers. Empty fields mean the
// secret was not supplied via flags/env/.env and must come from the keychain.
type Secrets struct {
	Password string `config:"secret.password"`
	Token    string `config:"secret.token"`
}

// Resolved is the outcome of Load: the merged Config plus provenance and any
// transient secrets.
type Resolved struct {
	Config  Config
	Secrets Secrets
	// Sources maps a field key to the layer name that supplied its final
	// value: "flag", "env", "dotenv", "file", "default".
	Sources map[string]string
	// ActiveContext is the name of the context whose fields were applied.
	// Empty when no config file (or no contexts) exists — pure-env usage.
	ActiveContext string
	// ContextSource records which precedence rule chose ActiveContext (one of
	// the ContextSource* constants), so callers can tell an explicit choice
	// (flag/env) from an implicit fallback (current_context, sole, default).
	ContextSource string
	// ContextNames lists every context defined in the file, in file order.
	ContextNames []string
}

// ContextSelectedExplicitly reports whether the active context was chosen for
// this invocation (via --use-context or JENKINS_CONTEXT) rather than fallen
// back to from the file's current_context, the sole context, or a "default"
// context. It is the signal used to decide whether to nudge an agent that may
// not realise which of several contexts it is hitting.
func (r *Resolved) ContextSelectedExplicitly() bool {
	return r.ContextSource == ContextSourceFlag || r.ContextSource == ContextSourceEnv
}

// Field keys used for layer maps and provenance tracking.
const (
	fieldCredentialURL = kit.KeyCredentialURL
	fieldServer        = kit.KeyServer
	fieldAuthScheme    = kit.KeyAuthScheme
	fieldAuthUsername  = "auth.username"
	fieldFormat        = "defaults.format"
	fieldTimeout       = "defaults.timeout"
	fieldMaxRetries    = "defaults.max_retries"
	fieldReadOnly      = "defaults.read_only"
	// Secret field keys (never persisted to the YAML file).
	fieldPassword = "secret.password"
	fieldToken    = "secret.token"
)

// Field key accessors for callers outside this package (e.g. config show).
const (
	FieldServer        = fieldServer
	FieldCredentialURL = fieldCredentialURL
	FieldAuthScheme    = fieldAuthScheme
	FieldAuthUser      = fieldAuthUsername
	FieldFormat        = fieldFormat
	FieldTimeout       = fieldTimeout
	FieldMaxRetries    = fieldMaxRetries
	FieldReadOnly      = fieldReadOnly
)
