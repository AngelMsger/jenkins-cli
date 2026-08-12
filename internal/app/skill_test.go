package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentIDs(t *testing.T) {
	t.Parallel()
	want := []string{
		"claude-code", "codex", "cursor", "agents", "gemini", "github-copilot",
		"opencode", "continue", "windsurf", "grok", "pi", "kilo", "roo",
	}
	ids := agentIDs()
	if len(ids) != len(want) {
		t.Fatalf("agentIDs() = %v (%d), want %d entries", ids, len(ids), len(want))
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing agent id %q", id)
		}
	}
}

func TestAgentDests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id              string
		wantHomeSuffix  string
		wantProjectPath string
	}{
		{"claude-code", filepath.Join(".claude", "skills", "jenkins"), filepath.Join(".claude", "skills", "jenkins")},
		{"codex", filepath.Join(".codex", "skills", "jenkins"), filepath.Join(".agents", "skills", "jenkins")},
		{"cursor", filepath.Join(".cursor", "skills", "jenkins"), filepath.Join(".cursor", "skills", "jenkins")},
		{"agents", filepath.Join(".agents", "skills", "jenkins"), filepath.Join(".agents", "skills", "jenkins")},
		{"gemini", filepath.Join(".gemini", "skills", "jenkins"), filepath.Join(".gemini", "skills", "jenkins")},
		{"github-copilot", filepath.Join(".copilot", "skills", "jenkins"), filepath.Join(".agents", "skills", "jenkins")},
		{"opencode", filepath.Join(".config", "opencode", "skills", "jenkins"), filepath.Join(".opencode", "skills", "jenkins")},
		{"continue", filepath.Join(".continue", "skills", "jenkins"), filepath.Join(".continue", "skills", "jenkins")},
		{"windsurf", filepath.Join(".codeium", "windsurf", "skills", "jenkins"), filepath.Join(".windsurf", "skills", "jenkins")},
		{"grok", filepath.Join(".grok", "skills", "jenkins"), filepath.Join(".grok", "skills", "jenkins")},
		{"pi", filepath.Join(".pi", "agent", "skills", "jenkins"), filepath.Join(".pi", "skills", "jenkins")},
		{"kilo", filepath.Join(".kilocode", "skills", "jenkins"), filepath.Join(".kilocode", "skills", "jenkins")},
		{"roo", filepath.Join(".roo", "skills", "jenkins"), filepath.Join(".roo", "skills", "jenkins")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec, ok := agentByID(tc.id)
			if !ok {
				t.Fatalf("agentSpec %q missing", tc.id)
			}
			projectPath, err := agentDest(spec, true)
			if err != nil {
				t.Fatal(err)
			}
			if projectPath != tc.wantProjectPath {
				t.Fatalf("project dest = %q, want %q", projectPath, tc.wantProjectPath)
			}
			homePath, err := agentDest(spec, false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(homePath, tc.wantHomeSuffix) {
				t.Fatalf("home dest %q does not end with %q", homePath, tc.wantHomeSuffix)
			}
		})
	}
}
