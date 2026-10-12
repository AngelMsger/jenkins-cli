package app

import (
	"testing"

	"github.com/angelmsger/jenkins-cli/internal/config"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func initCmdForTest() *cobra.Command {
	cmd := &cobra.Command{Use: "init"}
	var ctx string
	cmd.Flags().StringVar(&ctx, "context", config.DefaultContextName, "")
	return cmd
}

// The non-interactive branches of resolveInitTarget are pure decisions and must
// hold without any prompting; the edit/add/replace prompts are exercised by the
// e2e suite (scripts/e2e.sh) which drives stdin.

func TestResolveInitTarget_NoExistingConfig(t *testing.T) {
	s := &appState{}
	target, prefill, replaceAll, err := s.resolveInitTarget(initCmdForTest(), config.File{}, config.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if target != config.DefaultContextName || prefill != nil || replaceAll {
		t.Fatalf("fresh setup: got (%q, %v, %v)", target, prefill, replaceAll)
	}
}

func TestResolveInitTarget_ExplicitContextSkipsPrompt(t *testing.T) {
	file := config.File{
		CurrentContext: "default",
		Contexts: []config.NamedContext{
			{Name: "default", BaseURL: "http://a"},
		},
	}

	t.Run("new name", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", "prod") // marks the flag Changed
		target, prefill, replaceAll, err := s.resolveInitTarget(cmd, file, "prod")
		if err != nil {
			t.Fatal(err)
		}
		if target != "prod" || prefill != nil || replaceAll {
			t.Fatalf("explicit new: got (%q, %v, %v)", target, prefill, replaceAll)
		}
	})

	t.Run("new name is saved normalized", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", " Prod ")
		target, prefill, _, err := s.resolveInitTarget(cmd, file, " Prod ")
		if err != nil || target != "prod" || prefill != nil {
			t.Fatalf("explicit new: got (%q, %v, %v)", target, prefill, err)
		}
	})

	t.Run("a padded existing name edits that context", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", " Default ")
		target, prefill, _, err := s.resolveInitTarget(cmd, file, " Default ")
		if err != nil || target != "default" || prefill == nil || prefill.BaseURL != "http://a" {
			t.Fatalf("padded existing: got (%q, %+v, %v)", target, prefill, err)
		}
	})

	t.Run("blank name", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", "  ")
		if _, _, _, err := s.resolveInitTarget(cmd, file, "  "); err == nil || cerrors.AsCLIError(err).Code != "CTX_NAME_EMPTY" {
			t.Fatalf("blank name: %v", err)
		}
	})

	t.Run("existing name prefills, case-insensitive", func(t *testing.T) {
		s := &appState{}
		cmd := initCmdForTest()
		_ = cmd.Flags().Set("context", "DEFAULT")
		target, prefill, replaceAll, err := s.resolveInitTarget(cmd, file, "DEFAULT")
		if err != nil {
			t.Fatal(err)
		}
		if target != "default" || replaceAll {
			t.Fatalf("explicit existing: got (%q, replaceAll=%v)", target, replaceAll)
		}
		if prefill == nil || prefill.BaseURL != "http://a" {
			t.Fatalf("expected prefill from the stored context, got %+v", prefill)
		}
	})
}
