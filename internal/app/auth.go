package app

import (
	"context"

	"github.com/angelmsger/jenkins-cli/internal/auth"
	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newAuthCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, check identity and log out",
	}
	cmd.AddCommand(newAuthLoginCmd(s), newAuthStatusCmd(s), newAuthLogoutCmd(s))
	return cmd
}

func newAuthLoginCmd(s *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Store credentials for the active context (interactive)",
		Long: "Prompts for your Jenkins username and API token (or password), verifies\n" +
			"them against the server, and stores the secret in the OS keychain.\n" +
			"Requires an interactive terminal; in CI / agent sandboxes set\n" +
			"JENKINS_USER + JENKINS_TOKEN (or JENKINS_PASSWORD) instead.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !stdinIsTTY() {
				return cerrors.New(cerrors.CategoryAuth, "AUTH_LOGIN_NEEDS_TTY",
					"auth login requires an interactive terminal").
					WithHint("Set JENKINS_USER + JENKINS_TOKEN (or JENKINS_PASSWORD), "+
						"or run `jenkins-cli config init` in a terminal.").
					WithNextSteps("jenkins-cli auth status", "jenkins-cli config init")
			}
			cfg := s.cfg()
			if cfg.BaseURL == "" {
				return cerrors.New(cerrors.CategoryConfig, "NO_BASE_URL",
					"no server configured yet").
					WithNextSteps("jenkins-cli config init")
			}
			scheme := cfg.Auth.Scheme
			if scheme == "" {
				scheme = auth.SchemeToken
			}
			cred := auth.Credential{Scheme: scheme, Username: cfg.Auth.Username}
			if cred.Username == "" {
				u, err := promptLine("Username", "")
				if err != nil {
					return err
				}
				cred.Username = u
			}
			secretLabel := "API token"
			if scheme == auth.SchemeBasic {
				secretLabel = "Password"
			}
			secret, err := promptSecret(secretLabel)
			if err != nil {
				return err
			}
			cred.Secret = secret

			backend, err := verifyAndSave(s, cfg.BaseURL, cred)
			if err != nil {
				return err
			}
			return s.emit(map[string]any{
				"logged_in": true,
				"base_url":  cfg.BaseURL,
				"username":  cred.Username,
				"scheme":    scheme,
				"stored_in": backend,
			})
		},
	}
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
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				out["authenticated"] = false
				out["error"] = err.Error()
				return s.emit(out)
			}
			user, err := client.WhoAmI(ctx)
			if err != nil {
				out["authenticated"] = false
				out["error"] = err.Error()
				return s.emit(out)
			}
			out["authenticated"] = true
			if user.ID != "" {
				out["user_id"] = user.ID
			}
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
			if err := auth.Forget(cfg.BaseURL, scheme, s.store); err != nil {
				return cerrors.Wrap(err, cerrors.CategoryConfig, "LOGOUT_FAILED",
					"failed to remove stored credential")
			}
			return s.emit(map[string]any{"logged_out": true, "base_url": cfg.BaseURL, "scheme": scheme})
		},
	}
}

// verifyAndSave builds a client from cred, pings the server to confirm the
// credential works, then persists the secret. It returns the storage backend.
func verifyAndSave(s *appState, baseURL string, cred auth.Credential) (string, error) {
	if err := cred.Validate(); err != nil {
		return "", err
	}
	client, err := apiclient.BuildClient(apiclient.BuildParams{
		BaseURL:       baseURL,
		AuthDecorator: cred.Decorator(),
		Timeout:       s.timeout(),
		MaxRetries:    s.cfg().Defaults.MaxRetries,
	})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		return "", err
	}
	return auth.Save(baseURL, cred, s.store)
}
