package errors

import "testing"

// The model is tested in agent-cli-kit; this checks that jenkins-cli's own
// guidance is what an error built through this package carries.
func TestErrorsCarryJenkinsGuidance(t *testing.T) {
	e := New(CategoryConfig, "NO_BASE_URL", "no Jenkins server URL configured")
	if len(e.NextSteps) == 0 || e.NextSteps[0] != "jenkins-cli config init" {
		t.Fatalf("NextSteps = %v, want jenkins-cli's config guidance", e.NextSteps)
	}
	if ExitCode(e) != ExitConfig {
		t.Fatalf("ExitCode = %d, want %d", ExitCode(e), ExitConfig)
	}
}
