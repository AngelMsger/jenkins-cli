package jenkinscli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSkillErrorHandlingExample(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	data, err := SkillFS.ReadFile(SkillRoot + "/references/errors-and-exit-codes.md")
	if err != nil {
		t.Fatal(err)
	}
	lf := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, tc := range []struct {
		name, newline string
	}{
		{"LF", "\n"},
		{"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testSkillErrorHandlingExample(t, bash, strings.ReplaceAll(lf, "\n", tc.newline))
		})
	}
}

func testSkillErrorHandlingExample(t *testing.T, bash, markdown string) {
	t.Helper()
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	blocks := strings.Split(markdown, "```bash\n")
	if len(blocks) != 2 {
		t.Fatalf("expected one executable bash example, got %d", len(blocks)-1)
	}
	script, _, ok := strings.Cut(blocks[1], "```")
	if !ok {
		t.Fatal("unterminated bash example")
	}
	stub := "jenkins-cli() { printf '{\"number\":128}\\n'; return \"$JENKINS_EXAMPLE_EXIT\"; }\n"
	for _, code := range []int{0, 2, 3, 4, 6, 7, 8, 9} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			cmd := exec.Command(bash, "-eu", "-c", stub+script)
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), fmt.Sprintf("JENKINS_EXAMPLE_EXIT=%d", code))
			var out, diagnostics bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &diagnostics
			err := cmd.Run()
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != code {
				t.Fatalf("exit = %v, want %d; stderr: %s", err, code, &diagnostics)
			}
			if code == 0 {
				if strings.TrimSpace(out.String()) != `{"number":128}` || diagnostics.Len() != 0 {
					t.Fatalf("success output = %q, stderr = %q", &out, &diagnostics)
				}
			} else if out.Len() != 0 || diagnostics.Len() == 0 {
				t.Fatalf("failure stdout = %q, stderr = %q", &out, &diagnostics)
			}
		})
	}
}
