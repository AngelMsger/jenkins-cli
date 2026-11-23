package app

import (
	"context"
	"net/http"
	"time"

	"github.com/angelmsger/agent-cli-kit/doctor"
	"github.com/angelmsger/jenkins-cli/pkg/constants"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

// updateCheckTimeout caps the release-update lookup so an offline or slow
// network never stalls `doctor` for the full request timeout.
const updateCheckTimeout = 5 * time.Second

type doctorCheck struct {
	Check         string `json:"check"`
	OK            bool   `json:"ok"`
	Status        string `json:"status"`
	Detail        string `json:"detail"`
	RecoveryScope string `json:"recovery_scope,omitempty"`
}

func newDoctorCmd(s *appState) *cobra.Command {
	var skipUpdate bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration, credentials and connectivity",
		Long: "Runs a quick health check: is a server configured, are credentials\n" +
			"present, and can the server be reached and authenticated. Each check\n" +
			"reports ok/failed so an agent can self-diagnose environment problems.\n" +
			"It also reports whether a newer jenkins-cli release is available.",
		Example: "  jenkins-cli doctor\n" +
			"  jenkins-cli doctor --no-update-check",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := s.cfg()
			checks := []doctorCheck{}
			add := func(name string, ok bool, status, detail, recoveryScope string) {
				checks = append(checks, doctorCheck{
					Check: name, OK: ok, Status: status, Detail: detail, RecoveryScope: recoveryScope,
				})
			}

			add("config_dir", true, "ok", s.cfgDir, "")
			serverOK := cfg.BaseURL != ""
			add("server_configured", serverOK, statusForOK(serverOK, "missing"), cfg.BaseURL, "")

			credOK := false
			var checkErr error
			client, err := s.newClient()
			if err != nil {
				checkErr = err
				add("credentials", false, doctor.Status(err), doctor.Detail(err), doctor.RecoveryScope(err))
			} else {
				credOK = true
				add("credentials", true, "ok", "scheme "+cfg.Auth.Scheme, "")
			}

			if credOK {
				ctx, cancel := cmdContext(s)
				defer cancel()
				checkErr = client.Ping(ctx)
				if checkErr != nil {
					add("connectivity", false, doctor.Status(checkErr), doctor.Detail(checkErr), doctor.RecoveryScope(checkErr))
				} else {
					add("connectivity", true, "ok", "reached Jenkins", "")
					user, err := client.WhoAmI(ctx)
					checkErr = err
					if err != nil {
						add("identity", false, doctor.Status(err), doctor.Detail(err), doctor.RecoveryScope(err))
					} else {
						name := user.FullName
						if name == "" {
							name = user.ID
						}
						add("identity", true, "ok", name, "")
					}
				}
			}
			checks = append(checks, companionSkillDoctorCheck())

			healthy := serverOK && credOK && checkErr == nil
			report := map[string]any{
				"healthy": healthy,
				"version": versionString(),
				"checks":  checks,
			}
			if checkErr != nil {
				ce := cerrors.AsCLIError(checkErr)
				withReuseRecovery(ce, s, cmd)
				s.scopeAuthSteps(ce.NextSteps)
				report["next_steps"] = ce.NextSteps
			}
			// Release-update check: informational only, it never affects health
			// and never fails the command — being out of date is not a fault.
			if !skipUpdate {
				ctx, cancel := updateContext(s)
				defer cancel()
				st := releaseChecker.Check(ctx, &http.Client{Timeout: updateCheckTimeout}, constants.Version)
				report["update"] = st
			}
			return s.emit(report)
		},
	}
	cmd.Flags().BoolVar(&skipUpdate, "no-update-check", false,
		"skip the check for a newer jenkins-cli release")
	return cmd
}

func companionSkillDoctorCheck() doctorCheck {
	load := currentSkillLoadState()
	installs, err := inspectSkillInstalls(false)
	if err != nil {
		return doctorCheck{Check: "companion-skill", Status: "invalid", Detail: err.Error()}
	}
	projectInstalls, err := inspectSkillInstalls(true)
	if err != nil {
		return doctorCheck{Check: "companion-skill", Status: "invalid", Detail: err.Error()}
	}
	installs = append(installs, projectInstalls...)
	currentInstalled := false
	installed := false
	for _, item := range installs {
		installed = installed || item.Status == "installed"
		currentInstalled = currentInstalled || item.Alignment == "current"
	}
	switch {
	case load.Status == "current":
		return doctorCheck{Check: "companion-skill", OK: true, Status: "current",
			Detail: "loaded Skill " + load.Version + " matches " + embeddedSkillVersion()}
	case load.Loaded && currentInstalled:
		return doctorCheck{Check: "companion-skill", Status: "reload_required",
			Detail: "current Skill is installed; reload the agent context to load it"}
	case load.Loaded:
		return doctorCheck{Check: "companion-skill", Status: load.Status,
			Detail: "run `" + constants.AppName + " skill install`, then reload the agent context"}
	case currentInstalled:
		return doctorCheck{Check: "companion-skill", OK: true, Status: "installed_not_loaded",
			Detail: "current Skill is installed; reload the agent context to load it"}
	case installed:
		return doctorCheck{Check: "companion-skill", Status: "outdated",
			Detail: "run `" + constants.AppName + " skill install`, then reload the agent context"}
	default:
		return doctorCheck{Check: "companion-skill", Status: "not_installed",
			Detail: "run `" + constants.AppName + " skill install`, then reload the agent context"}
	}
}

func statusForOK(ok bool, failure string) string {
	if ok {
		return "ok"
	}
	return failure
}

// updateContext bounds the release-update lookup by updateCheckTimeout, or the
// configured request timeout when that is shorter.
func updateContext(s *appState) (context.Context, context.CancelFunc) {
	d := updateCheckTimeout
	if t := s.timeout(); t > 0 && t < d {
		d = t
	}
	return context.WithTimeout(context.Background(), d)
}
