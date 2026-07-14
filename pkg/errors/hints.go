package errors

// defaultGuidance returns the default hint and next-step commands for a
// category. Callers may override these via WithHint / WithNextSteps when more
// specific guidance is available.
func defaultGuidance(cat Category) (hint string, steps []string) {
	switch cat {
	case CategoryUsage:
		return "The command was invoked incorrectly. Check flags and arguments.",
			[]string{"jenkins-cli <command> --help"}
	case CategoryConfig:
		return "No usable configuration was found or it is invalid.",
			[]string{"jenkins-cli config init", "jenkins-cli config show"}
	case CategoryAuth:
		return "The server rejected the credentials. The password/token may be wrong.",
			[]string{"jenkins-cli auth status", "jenkins-cli config init"}
	case CategoryPermission:
		return "The credentials are valid but lack permission for this Jenkins resource.",
			[]string{"jenkins-cli job list", "Verify the account has the required Jenkins job or system permission in the web UI."}
	case CategoryNotFound:
		return "The requested Jenkins job, build or queue item does not exist.",
			[]string{"jenkins-cli job list", "jenkins-cli build list <job-path>", "jenkins-cli queue list"}
	case CategoryConflict:
		return "The resource changed since it was last read (version conflict).",
			[]string{"Re-fetch the resource to get its current state, then retry."}
	case CategoryRateLimit:
		return "The server is rate limiting requests. Retry after a short wait.",
			[]string{"Wait and retry; narrow the time range or reduce --limit."}
	case CategoryNetwork:
		return "The server could not be reached (DNS, TLS or timeout).",
			[]string{"jenkins-cli doctor", "Check --base-url / JENKINS_URL and network connectivity."}
	case CategoryServer:
		return "The Jenkins server returned an internal error.",
			[]string{"Retry later.", "jenkins-cli doctor"}
	case CategoryParse:
		return "A response could not be parsed or rendered.",
			[]string{"Retry with --format json and --verbose to inspect raw content."}
	default:
		return "An unexpected internal error occurred.",
			[]string{"Retry with --verbose for details."}
	}
}
