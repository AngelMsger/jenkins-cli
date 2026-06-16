// Package constants holds project-wide constants and build-time metadata.
package constants

import "time"

// Build-time metadata, injected via -ldflags. See Makefile.
var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

const (
	// AppName is the binary / command name.
	AppName = "jenkins-cli"

	// EnvPrefix is the environment variable prefix for all settings.
	EnvPrefix = "JENKINS_"

	// ConfigParentDirName groups every angelmsger CLI's per-user config under
	// one shared $HOME-relative directory (~/.angelmsger).
	ConfigParentDirName = ".angelmsger"

	// ConfigDirName is the per-CLI config directory under ConfigParentDirName,
	// i.e. ~/.angelmsger/jenkins.
	ConfigDirName = "jenkins"

	// ConfigFileName is the YAML config file within ConfigDirName.
	ConfigFileName = "config.yaml"

	// CredentialsFileName is the fallback secret store when no keychain is available.
	CredentialsFileName = "credentials"

	// KeychainService is the service name used for OS keychain entries.
	KeychainService = "jenkins-cli"
)

// Defaults for runtime behaviour.
const (
	DefaultFormat     = "json"
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 3

	// DefaultBuildHistoryLimit is the number of builds returned by `job builds`
	// when --limit is not given. Kept modest so a stray listing does not flood
	// an agent's context window.
	DefaultBuildHistoryLimit = 20

	// MaxConsoleBytes caps how much console text `build log` returns in one shot
	// (without --follow). Large logs are common; this keeps a single fetch from
	// flooding the agent's context. Override with an explicit --start offset.
	MaxConsoleBytes = 200_000

	// DefaultLocalBaseURL is the default URL of a local Jenkins install.
	DefaultLocalBaseURL = "http://localhost:8080"
)

// UserAgent identifies the CLI to the Jenkins server.
func UserAgent() string {
	return AppName + "/" + Version
}
