package app

import (
	"strings"

	"github.com/angelmsger/jenkins-cli/internal/config"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newConfigSetContextCmd(s *appState) *cobra.Command {
	var overwrite, activate, dryRun bool
	cmd := &cobra.Command{Use: "set-context <name>", Short: "Configure service presets without credentials or network access", Args: cobra.ExactArgs(1),
		Long:    "Write service fields from flags, environment or .env over the named target context. Personal environment fields and secrets are ignored. Existing conflicting fields require --overwrite; unrelated contexts and user identities are preserved.",
		Example: "  jenkins-cli config set-context team --base-url https://service.example.com --dry-run\n  jenkins-cli config set-context team --base-url https://service.example.com --activate",
		RunE: func(cmd *cobra.Command, args []string) error {
			file, _, err := config.ReadFile(s.cfgDir)
			if err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_READ", "failed to read config")
			}
			plan, err := config.PlanServiceContext(file, args[0], s.resolved, overwrite, activate)
			if err != nil {
				return err
			}
			if plan.Changed && !dryRun {
				if err := config.WriteFile(s.cfgDir, plan.File); err != nil {
					return cerrors.Wrap(err, cerrors.CategoryConfig, "CONFIG_WRITE", "failed to write service presets")
				}
			}
			return s.emit(struct {
				config.ContextPlan
				DryRun     bool   `json:"dry_run"`
				ConfigFile string `json:"config_file"`
			}{plan, dryRun, config.ConfigFilePath(s.cfgDir)})
		},
	}
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "update conflicting service fields; preserve personal identity and credentials")
	cmd.Flags().BoolVar(&activate, "activate", false, "make this the current context")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview configuration changes without writing files or credentials")
	return cmd
}

func newAuthGuideCmd(s *appState) *cobra.Command {
	return &cobra.Command{Use: "guide", Short: "Show offline credential acquisition guidance for this service", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, err := config.Guide(s.cfg(), s.resolved.Sources)
			if err != nil {
				return err
			}
			s.scopeAuthSteps(g.NextSteps)
			return s.emit(g)
		},
	}
}

// Recovery can start a personal login. Carry its configuration directory and
// explicit service override even when reuse already supplied a context.
func (s *appState) scopeAuthSteps(steps []string) {
	configFlag := ""
	if s.gflags.configPath != "" {
		configFlag = " --config " + cerrors.ShellArg(s.gflags.configPath)
	}
	baseFlag := ""
	if s.gflags.baseURL != "" {
		base, err := config.NormalizeServiceURL(s.gflags.baseURL)
		if err != nil {
			// Never print userinfo or send the user to a different default server.
			base = "<jenkins-url>"
		}
		baseFlag = " --base-url " + cerrors.ShellArg(base)
	}
	for i, step := range steps {
		command, ok := strings.CutPrefix(step, "jenkins-cli ")
		if !ok {
			continue
		}
		flags := configFlag + baseFlag
		switch {
		case strings.HasPrefix(command, "auth "):
			if s.resolved != nil && s.resolved.ActiveContext != "" {
				command = "--use-context " + cerrors.ShellArg(s.resolved.ActiveContext) + " " + command
			}
		case strings.HasPrefix(command, "--use-context ") && strings.Contains(command, " auth "):
			// The step already names its destination, which may be a new context.
		case command == "config get-contexts", strings.HasPrefix(command, "config set-context "),
			strings.HasPrefix(command, "--base-url <jenkins-url> auth "):
			// Discovery is offline; a setup template supplies its own server.
			flags = configFlag
		case command == "doctor":
			if s.gflags.useContext != "" {
				command = "--use-context " + cerrors.ShellArg(s.gflags.useContext) + " " + command
			}
		case command == "config init", strings.HasPrefix(command, "config init --context "):
		default:
			continue
		}
		steps[i] = "jenkins-cli" + flags + " " + command
	}
}

// setupPrefill applies presets to the actual wizard destination, not the active context.
func (s *appState) setupPrefill(name string, _ *config.NamedContext) (*config.NamedContext, error) {
	resolved, err := config.Load(config.LoadOptions{ConfigDir: s.cfgDir, Context: name, Setup: true, Flags: config.FlagValues{
		BaseURL: s.gflags.baseURL, AuthScheme: s.gflags.authScheme, CredentialURL: s.gflags.credentialURL,
	}})
	if err != nil {
		return nil, err
	}
	cfg := resolved.Config
	return &config.NamedContext{Name: name, BaseURL: cfg.BaseURL, Auth: cfg.Auth}, nil
}
