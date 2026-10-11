package config

import (
	"path/filepath"
	"strconv"

	kit "github.com/angelmsger/agent-cli-kit/config"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
	_ "github.com/angelmsger/jenkins-cli/pkg/errors" // installs Jenkins's error guidance
)

// EnvNoContextHint opts out of the runtime multi-context notice.
const EnvNoContextHint = "JENKINS_CLI_NO_CONTEXT_HINT"

func init() {
	kit.Register(&kit.Schema{
		// Each row names a field's environment variables and built-in
		// default. Service fields are the team presets a setup load keeps;
		// the user name and secrets are personal.
		Fields: []kit.Field{
			{Key: fieldServer, Env: []string{"JENKINS_URL"}, Service: true},
			{Key: fieldCredentialURL, Env: []string{"JENKINS_CREDENTIAL_URL"}, Service: true},
			{Key: fieldAuthScheme, Env: []string{"JENKINS_AUTH_SCHEME"}, Default: SchemeToken, Service: true},
			{Key: fieldAuthUsername, Env: []string{"JENKINS_USER"}},
			{Key: fieldFormat, Env: []string{"JENKINS_FORMAT"}, Default: constants.DefaultFormat, Service: true},
			{Key: fieldTimeout, Default: constants.DefaultTimeout.String()},
			{Key: fieldMaxRetries, Default: strconv.Itoa(constants.DefaultMaxRetries)},
			{Key: fieldReadOnly, Env: []string{"JENKINS_CLI_READ_ONLY"}, Service: true},
			// A secret in the environment selects its scheme; a token wins
			// over a password.
			{Key: fieldToken, Env: []string{"JENKINS_TOKEN"}, Implies: SchemeToken},
			{Key: fieldPassword, Env: []string{"JENKINS_PASSWORD"}, Implies: SchemeBasic},
		},
		Paths: kit.Paths{
			Dir:  filepath.Join(constants.ConfigParentDirName, constants.ConfigDirName),
			File: constants.ConfigFileName,
		},
		Words: kit.Words{
			App:          constants.AppName,
			ContextFlag:  "--use-context",
			ContextEnv:   "JENKINS_CONTEXT",
			NoHintEnv:    EnvNoContextHint,
			ListContexts: "config get-contexts",
			UseContext:   "config use-context",
			SetContext:   "config set-context",
			AuthGuide:    "auth guide",
			AuthLogin:    "auth login",
		},
		Schemes: []string{SchemeToken, SchemeBasic},
		// A bare host means a local http:// install, as `config init` assumes.
		URL: kit.URLRules{AssumeScheme: "http"},
	})
}
