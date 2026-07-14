package auth

import (
	"errors"
	"testing"

	"github.com/angelmsger/jenkins-cli/internal/config"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/zalando/go-keyring"
)

type failingKeyring struct{ err error }

func (f failingKeyring) Get(string, string) (string, error) { return "", f.err }
func (f failingKeyring) Set(string, string, string) error   { return f.err }
func (f failingKeyring) Delete(string, string) error        { return f.err }

func TestStoreDistinguishesInaccessibleFromMissing(t *testing.T) {
	t.Parallel()
	accessErr := errors.New("keychain interaction is not allowed")
	s := newStoreWithKeyring(t.TempDir(), failingKeyring{err: accessErr})
	_, err := s.Load("acct")
	var storeErr *StoreAccessError
	if !errors.As(err, &storeErr) || storeErr.Backend != BackendKeychain {
		t.Fatalf("Load() error = %v, want keychain StoreAccessError", err)
	}
	if !errors.Is(err, accessErr) {
		t.Fatalf("Load() should preserve keychain cause: %v", err)
	}
}

func TestStoreUsesFileWhenKeychainIsInaccessible(t *testing.T) {
	t.Parallel()
	s := newStoreWithKeyring(t.TempDir(), failingKeyring{err: errors.New("locked")})
	if err := s.fileSave("acct", "fallback-secret"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("acct")
	if err != nil || got != "fallback-secret" {
		t.Fatalf("Load() = %q, %v; want file fallback", got, err)
	}
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
