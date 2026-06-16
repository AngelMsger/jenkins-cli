// Package auth models Jenkins credentials, resolves them from configuration
// or secure storage, and applies them to outgoing HTTP requests.
package auth

import (
	"net/url"
	"strings"

	cerrors "github.com/angelmsger/jenkins-cli/internal/errors"
)

// Scheme identifies an authentication scheme. Jenkins authenticates API
// requests with HTTP Basic auth in both schemes; they differ only in what the
// secret is (an API token vs a password).
const (
	// SchemeToken is the recommended Jenkins credential: a username plus an API
	// token, encoded as HTTP Basic.
	SchemeToken = "token"
	// SchemeBasic is a username plus a password, encoded as HTTP Basic.
	SchemeBasic = "basic"
)

// Credential is a fully resolved credential ready to authenticate requests.
type Credential struct {
	Scheme   string
	Username string // the Jenkins login the secret belongs to
	Secret   string // API token (token) or password (basic)
}

// Validate reports whether the credential is internally consistent. Both
// schemes require a username and a secret — Jenkins Basic auth is
// username:secret on the wire.
func (c Credential) Validate() error {
	switch c.Scheme {
	case SchemeToken:
		if c.Username == "" || c.Secret == "" {
			return cerrors.New(cerrors.CategoryConfig, "AUTH_NO_TOKEN",
				"token auth requires both a username and an API token")
		}
	case SchemeBasic:
		if c.Username == "" || c.Secret == "" {
			return cerrors.New(cerrors.CategoryConfig, "AUTH_NO_BASIC",
				"basic auth requires both a username and a password")
		}
	default:
		return cerrors.Newf(cerrors.CategoryConfig, "AUTH_BAD_SCHEME",
			"unknown auth scheme %q (want token or basic)", c.Scheme)
	}
	return nil
}

// Redacted returns a copy safe for logging: the secret is masked.
func (c Credential) Redacted() Credential {
	c.Secret = maskSecret(c.Secret)
	return c
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

// AccountKey derives the keychain account identifier for a base URL and scheme.
// It is stable across runs so credentials can be located later.
func AccountKey(baseURL, scheme string) string {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return host + ":" + scheme
}
