package auth

import (
	"errors"
	"testing"

	"github.com/angelmsger/agent-cli-kit/credstore"
	"github.com/angelmsger/jenkins-cli/internal/config"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

// failingKeyring is a keychain backend whose every call fails with err.
type failingKeyring struct{ err error }

func (f failingKeyring) Get(string, string) (string, error) { return "", f.err }
func (f failingKeyring) Set(string, string, string) error   { return f.err }
func (f failingKeyring) Delete(string, string) error        { return f.err }

// newStoreWithKeyring returns jenkins-cli's Store over a substitute keychain
// backend. The store's own behavior is tested in agent-cli-kit/credstore.
func newStoreWithKeyring(dir string, backend credstore.Backend) *Store {
	return credstore.NewStoreWithBackend(dir, constants.KeychainService, backend)
}

func TestResolveCredentialRecovery(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		BaseURL: "https://jenkins.example.com",
		Auth:    config.AuthConfig{Scheme: SchemeToken, Username: "alice"},
	}
	tests := []struct {
		name string
		err  error
		code string
	}{
		{"missing", keyring.ErrNotFound, "CREDENTIAL_NOT_VISIBLE_OR_MISSING"},
		{"inaccessible", errors.New("sandbox denied keychain"), "CREDENTIAL_STORE_INACCESSIBLE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStoreWithKeyring(t.TempDir(), failingKeyring{err: tc.err})
			_, err := Resolve(cfg, config.Secrets{}, s)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != tc.code {
				t.Fatalf("Resolve() error = %+v, want code %s", ce, tc.code)
			}
			if ce.Recovery == nil || ce.Recovery.Scope != "host" || ce.Retryable {
				t.Fatalf("Resolve() recovery = %+v retryable=%v, want host and false", ce.Recovery, ce.Retryable)
			}
		})
	}
}
