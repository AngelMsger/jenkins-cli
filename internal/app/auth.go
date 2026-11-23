package app

import (
	"fmt"
	"strings"

	"github.com/angelmsger/agent-cli-kit/terminal"
	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/internal/config"
	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newAuthCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, check identity and log out",
	}
	cmd.AddCommand(newAuthReuseCmd(s), newAuthGuideCmd(s), newAuthLoginCmd(s), newAuthStatusCmd(s), newAuthLogoutCmd(s))
	return cmd
}

func newAuthLoginCmd(s *appState) *cobra.Command {
	var browser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Verify and store credentials for the active context",
		Long: "Enter a Jenkins user ID and API token (or password) in a terminal.\n" +
			"With --browser, open Jenkins in your browser, complete single sign-on,\n" +
			"then enter the Jenkins user ID and a personal API token in the terminal.\n" +
			"This also migrates a password context after Jenkins verifies the token.",
		Example: "  jenkins-cli auth login --browser\n  jenkins-cli --use-context team auth login --browser",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if browser && cmd.Flags().Changed("auth-scheme") && s.cfg().Auth.Scheme != auth.SchemeToken {
				return cerrors.New(cerrors.CategoryUsage, "BROWSER_REQUIRES_TOKEN", "--browser uses the Jenkins token scheme").
					WithNextSteps("jenkins-cli auth login --browser")
			}
			if !stdinIsTTY() {
				return cerrors.New(cerrors.CategoryAuth, "AUTH_LOGIN_NEEDS_TTY",
					"auth login requires an interactive terminal").
					WithHint("Run auth login in your terminal. For CI, supply JENKINS_USER and JENKINS_TOKEN through the environment.").
					WithNextSteps("jenkins-cli auth guide", "jenkins-cli auth login --browser")
			}
			return interactiveLogin(s, browser, prompter(), openBrowser)
		},
	}
	cmd.Flags().BoolVar(&browser, "browser", false, "open Jenkins for SSO and verify a personal API token")
	return cmd
}

func interactiveLogin(s *appState, browser bool, prompt terminal.Prompter, open func(string) error) error {
	cfg := s.cfg()
	if cfg.BaseURL == "" {
		return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL", "no server configured yet").
			WithNextSteps("jenkins-cli config init", "jenkins-cli --base-url <jenkins-url> auth login --browser")
	}
	if browser || cfg.Auth.Scheme == "" {
		cfg.Auth.Scheme = auth.SchemeToken
	}
	cred := auth.Credential{Scheme: cfg.Auth.Scheme, Username: cfg.Auth.Username}
	plan, err := planLogin(s, cfg, cred)
	if err != nil {
		return err
	}
	guide, err := config.Guide(cfg, s.resolved.Sources)
	if err != nil {
		return err
	}
	for _, line := range guide.Lines() {
		fmt.Fprintln(prompt.Out, line)
	}
	if browser {
		// Only open the normalized Jenkins service. Team-supplied guide links
		// remain display-only even when they name another origin.
		plan.retryStep += " --browser"
		page := guide.Server + "/me/security"
		fmt.Fprintln(prompt.Out, "Complete sign-in in your browser and create a personal API token at", page)
		if err := open(page); err != nil {
			fmt.Fprintln(prompt.Out, "Open the Jenkins URL above manually to continue.")
		}
	}
	if browser || cred.Username == "" {
		cred.Username, err = prompt.Line("Jenkins user ID", cred.Username)
		if err != nil {
			return err
		}
	}
	label := "API token"
	if cred.Scheme == auth.SchemeBasic {
		label = "Password"
	}
	cred.Secret, err = prompt.Secret(label)
	if err != nil {
		return err
	}
	services := s.loginServices()
	if browser {
		services.Verify = s.verifyBrowserToken
		services.ConfirmAliases = func(names []string) (bool, error) {
			return prompt.Confirm(fmt.Sprintf("Also change the Jenkins user ID to %q in contexts sharing this token (%s)?", cred.Username, strings.Join(names, ", ")))
		}
	}
	backend, err := plan.complete(s, cfg, cred, services)
	if err != nil {
		return err
	}
	result := map[string]any{
		"logged_in": true,
		"base_url":  cfg.BaseURL,
		"username":  cred.Username,
		"scheme":    cred.Scheme,
		"stored_in": backend,
	}
	if browser {
		steps := []string{"jenkins-cli auth status"}
		fromEnv := func(field string) bool {
			source := s.resolved.Sources[field]
			return source == "env" || source == "dotenv"
		}
		if s.resolved.Secrets.Password != "" || s.resolved.Secrets.Token != "" ||
			fromEnv(config.FieldAuthUser) || fromEnv(config.FieldAuthScheme) {
			steps = append([]string{"Check shell and .env credential overrides before testing the stored login."}, steps...)
		}
		s.scopeAuthSteps(steps)
		result["next_steps"] = steps
	}
	return s.emit(result)
}

func newAuthStatusCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active identity and verify connectivity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			out := map[string]any{
				"base_url": cfg.BaseURL,
				"scheme":   cfg.Auth.Scheme,
				"username": cfg.Auth.Username,
				"context":  s.resolved.ActiveContext,
			}
			client, err := s.newClient()
			var user *apiclient.User
			if err == nil {
				ctx, cancel := cmdContext(s)
				defer cancel()
				user, err = client.WhoAmI(ctx)
			}
			if err != nil {
				ce := cerrors.AsCLIError(err)
				withReuseRecovery(ce, s, cmd)
				s.scopeAuthSteps(ce.NextSteps)
				out["authenticated"] = false
				out["error"] = err.Error()
				out["error_code"] = ce.Code
				out["hint"] = ce.Hint
				out["next_steps"] = ce.NextSteps
				return s.emit(out)
			}
			out["authenticated"] = true
			out["user_id"] = user.ID
			if user.FullName != "" {
				out["full_name"] = user.FullName
			}
			return s.emit(out)
		},
	}
}

func newAuthLogoutCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored credential for the active context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			if cfg.BaseURL == "" {
				return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
					"no server configured").WithNextSteps("jenkins-cli config init")
			}
			scheme := cfg.Auth.Scheme
			if scheme == "" {
				scheme = auth.SchemeToken
			}
			if err := auth.ForgetForConfig(cfg, scheme, s.store); err != nil {
				return credentialLogoutFailure(err)
			}
			return s.emit(map[string]any{"logged_out": true, "base_url": cfg.BaseURL, "scheme": scheme})
		},
	}
}
