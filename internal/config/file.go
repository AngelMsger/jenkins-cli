package config

import (
	"time"

	kit "github.com/angelmsger/agent-cli-kit/config"
)

// authShape / defaultsShape / contextShape are the on-disk YAML building
// blocks. Timeout is a human-readable duration string ("30s").
type authShape struct {
	CredentialURL string `yaml:"credential_url,omitempty"`
	Scheme        string `yaml:"scheme,omitempty"`
	Username      string `yaml:"username,omitempty"`
}

type defaultsShape struct {
	Format     string `yaml:"format,omitempty"`
	Timeout    string `yaml:"timeout,omitempty"`
	MaxRetries int    `yaml:"max_retries,omitempty"`
	ReadOnly   bool   `yaml:"read_only,omitempty"`
}

type contextShape struct {
	Name   string    `yaml:"name"`
	Server string    `yaml:"server,omitempty"`
	Auth   authShape `yaml:"auth,omitempty"`
}

// fileShape is the on-disk YAML representation of the config file.
type fileShape struct {
	CurrentContext string         `yaml:"current_context,omitempty"`
	Contexts       []contextShape `yaml:"contexts,omitempty"`
	Defaults       defaultsShape  `yaml:"defaults,omitempty"`
}

// File is the parsed config file: a set of named contexts plus the shared
// runtime defaults and the name of the current context. Context looks a name
// up case-insensitively and returns the stored spelling; Upsert and Remove
// match names the same way.
type File = kit.File[NamedContext, Defaults]

// DefaultConfigDir returns the per-user config directory
// (~/.angelmsger/jenkins).
func DefaultConfigDir() (string, error) { return kit.DefaultDir() }

// ResolveConfigDir picks the config directory to use when --config was not
// supplied.
func ResolveConfigDir() (string, error) { return kit.ResolveDir() }

// ConfigFilePath returns the YAML config file path inside dir.
func ConfigFilePath(dir string) string { return kit.FilePath(dir) }

// ReadFile reads and parses the config file in dir. The bool return is false
// when the file does not exist.
func ReadFile(dir string) (File, bool, error) {
	var fs fileShape
	if ok, err := kit.ReadFile(dir, &fs); !ok || err != nil {
		return File{}, false, err
	}
	// Missing defaults stay zero; the default layer fills them during Load.
	timeout, _ := time.ParseDuration(fs.Defaults.Timeout)
	f := File{
		CurrentContext: fs.CurrentContext,
		Defaults: Defaults{
			Format:     fs.Defaults.Format,
			Timeout:    timeout,
			MaxRetries: fs.Defaults.MaxRetries,
			ReadOnly:   fs.Defaults.ReadOnly,
		},
	}
	for _, cs := range fs.Contexts {
		f.Contexts = append(f.Contexts, NamedContext{
			Name:    cs.Name,
			BaseURL: cs.Server,
			Auth:    AuthConfig{Scheme: cs.Auth.Scheme, Username: cs.Auth.Username, CredentialURL: cs.Auth.CredentialURL},
		})
	}
	return f, true, nil
}

// WriteFile persists a File to dir/config.yaml, creating dir with 0700
// permissions and replacing the file atomically. Secrets are never written
// here.
func WriteFile(dir string, f File) error {
	fs := fileShape{
		CurrentContext: f.CurrentContext,
		Defaults: defaultsShape{
			Format:     f.Defaults.Format,
			MaxRetries: f.Defaults.MaxRetries,
			ReadOnly:   f.Defaults.ReadOnly,
		},
	}
	if f.Defaults.Timeout > 0 {
		fs.Defaults.Timeout = f.Defaults.Timeout.String()
	}
	for _, c := range f.Contexts {
		fs.Contexts = append(fs.Contexts, contextShape{
			Name:   c.Name,
			Server: c.BaseURL,
			Auth:   authShape{Scheme: c.Auth.Scheme, Username: c.Auth.Username, CredentialURL: c.Auth.CredentialURL},
		})
	}
	return kit.WriteFile(dir, &fs)
}
