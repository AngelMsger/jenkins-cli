// Package auth declares jenkins-cli's auth schemes and resolves credentials
// from configuration or secure storage. The credential model, header encoding
// and recovery errors come from agent-cli-kit/auth; this package registers
// Jenkins's schemes and wording with it.
package auth

import (
	kit "github.com/angelmsger/agent-cli-kit/auth"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
	_ "github.com/angelmsger/jenkins-cli/pkg/errors" // installs Jenkins's error guidance
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

func init() {
	kit.Register(&kit.Product{
		Schemes: []kit.Scheme{
			{Name: SchemeToken, Kind: kit.Basic, MissingCode: "AUTH_NO_TOKEN",
				Missing: "token auth requires both a username and an API token"},
			{Name: SchemeBasic, Kind: kit.Basic,
				Missing: "basic auth requires both a username and a password"},
		},
		DefaultScheme:    SchemeToken,
		Name:             "Jenkins",
		Doctor:           constants.AppName + " doctor",
		MissingStep:      "Only if the host retry also reports missing credentials, run `" + constants.AppName + " config init` in the user's terminal or set JENKINS_* environment variables.",
		InaccessibleStep: "Do not run `config init` unless the same check also fails in the host environment.",
		ListSchemes:      true,
	})
}

// Credential is a fully resolved credential ready to authenticate requests.
// Its Validate, Header, Decorator and Redacted methods follow the schemes
// registered above.
type Credential = kit.Credential

// AccountKey derives the keychain account identifier for a base URL and scheme.
func AccountKey(baseURL, scheme string) string { return kit.AccountKey(baseURL, scheme) }
