package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/angelmsger/jenkins-cli/internal/config"
)

func TestAuthRecoveryKeepsExplicitServiceAndConfig(t *testing.T) {
	s := appState{
		gflags: globalFlags{
			configPath: "/team's config", baseURL: "https://JENKINS.EXAMPLE:443/deploy/", useContext: "team's ci",
		},
		resolved: &config.Resolved{ActiveContext: "team's ci"},
	}
	steps := []string{
		"jenkins-cli auth login --browser",
		"jenkins-cli --use-context 'source' auth reuse --auth-scheme token",
		"jenkins-cli config get-contexts",
		"jenkins-cli config set-context <name> --base-url <url>",
		"jenkins-cli doctor",
		"jenkins-cli build get 'app' '1'",
		"Check the Jenkins user ID.",
		"jenkins-cli config init --context 'source'",
	}
	configOnly := `jenkins-cli --config '/team'"'"'s config' `
	service := configOnly + "--base-url 'https://jenkins.example/deploy' "
	want := []string{
		service + `--use-context 'team'"'"'s ci' auth login --browser`,
		service + "--use-context 'source' auth reuse --auth-scheme token",
		configOnly + "config get-contexts",
		configOnly + "config set-context <name> --base-url <url>",
		service + `--use-context 'team'"'"'s ci' doctor`,
		steps[5], steps[6],
		service + "config init --context 'source'",
	}
	s.scopeAuthSteps(steps)
	if !slices.Equal(steps, want) {
		t.Fatalf("steps %q, want %q", steps, want)
	}
}

func TestAuthRecoveryForFirstLoginAndInvalidService(t *testing.T) {
	for _, raw := range []string{
		"https://alice:private-value@ci.example",
		"https://ci.example/?state=private-value",
		"invalid\nprivate-value",
	} {
		s := appState{gflags: globalFlags{baseURL: raw}}
		steps := []string{"jenkins-cli auth login --browser"}
		s.scopeAuthSteps(steps)
		if steps[0] != "jenkins-cli --base-url '<jenkins-url>' auth login --browser" || strings.Contains(steps[0], "private-value") {
			t.Fatalf("invalid service entered recovery: %q", steps)
		}
	}
	s := appState{gflags: globalFlags{baseURL: "jenkins.example/ci"}}
	steps := []string{"jenkins-cli auth login --browser"}
	s.scopeAuthSteps(steps)
	if steps[0] != "jenkins-cli --base-url 'http://jenkins.example/ci' auth login --browser" {
		t.Fatalf("first login lost explicit URL: %q", steps)
	}
}
