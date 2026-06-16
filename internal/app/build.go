package app

import (
	"strings"

	"github.com/angelmsger/jenkins-cli/internal/apiclient"
	cerrors "github.com/angelmsger/jenkins-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newBuildCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Inspect builds: status, logs, stages, tests, changes",
		Long: "A build is one run of a job. List a job's history, then drill into a\n" +
			"build's result, console log, pipeline stages, test failures or SCM\n" +
			"changes. A build reference is a number or a permalink keyword: last,\n" +
			"lastSuccessful, lastFailed, lastCompleted, lastStable (default: last).",
	}
	cmd.AddCommand(
		newBuildListCmd(s),
		newBuildGetCmd(s),
		newBuildLogCmd(s),
		newBuildStagesCmd(s),
		newBuildTestsCmd(s),
		newBuildChangesCmd(s),
		newBuildArtifactsCmd(s),
		newBuildStopCmd(s),
	)
	return cmd
}

func newBuildListCmd(s *appState) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list <path>",
		Short: "List a job's recent builds (newest first)",
		Long: "Returns a job's build history: number, status, start time (and how long\n" +
			"ago), duration and what triggered each build. Use --limit to control how\n" +
			"many builds come back.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			builds, err := client.JobBuilds(ctx, args[0], limit)
			if err != nil {
				return err
			}
			return s.emitList(builds, pageInfo{})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "number of builds to return")
	return cmd
}

func newBuildGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <path> [ref]",
		Short: "Get one build: result, timing, trigger cause",
		Long: "Returns a build's result and status, when it started (absolute and\n" +
			"relative) and how long it ran, which node it ran on, what triggered it,\n" +
			"and any build parameters. ref defaults to the latest build.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			build, err := client.GetBuild(ctx, path, ref)
			if err != nil {
				return err
			}
			return s.emit(build)
		},
	}
	return cmd
}

func newBuildStagesCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stages <path> [ref]",
		Short: "Show a Pipeline build's stage-by-stage status",
		Long: "Returns each Pipeline stage with its status and duration, so you can see\n" +
			"exactly which stage failed. Applies to Pipeline / multibranch jobs only.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			run, err := client.Stages(ctx, path, ref)
			if err != nil {
				return notPipelineHint(err)
			}
			return s.emit(run)
		},
	}
	return cmd
}

func newBuildTestsCmd(s *appState) *cobra.Command {
	var failedOnly bool
	cmd := &cobra.Command{
		Use:   "tests <path> [ref]",
		Short: "Show a build's test results",
		Long: "Returns the build's test totals plus the individual cases. Use\n" +
			"--failed-only to keep just the failing cases (with their error details) —\n" +
			"the high-signal view when triaging a red build.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			report, err := client.Tests(ctx, path, ref)
			if err != nil {
				return noTestsHint(err)
			}
			if failedOnly {
				report.Cases = filterFailed(report.Cases)
			}
			return s.emit(report)
		},
	}
	cmd.Flags().BoolVar(&failedOnly, "failed-only", false, "keep only failing test cases")
	return cmd
}

func newBuildChangesCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changes <path> [ref]",
		Short: "Show the SCM commits in a build",
		Long: "Returns the source-control changes a build picked up: commit id, author,\n" +
			"message and affected paths. Works for both freestyle and pipeline jobs.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			cs, err := client.Changes(ctx, path, ref)
			if err != nil {
				return err
			}
			return s.emit(cs)
		},
	}
	return cmd
}

func newBuildArtifactsCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifacts <path> [ref]",
		Short: "List a build's archived artifacts",
		Long:  "Returns the build's archived artifacts with a direct download URL for each.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			arts, err := client.Artifacts(ctx, path, ref)
			if err != nil {
				return err
			}
			return s.emitList(arts, pageInfo{})
		},
	}
	return cmd
}

func newBuildStopCmd(s *appState) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "stop <path> <ref>",
		Short: "Abort a running build (write)",
		Long: "Aborts the build at <path> #<ref>. With --dry-run, print the request\n" +
			"that would be sent and exit. This is a write: it is blocked in read-only\n" +
			"mode unless --allow-writes is given.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := args[0], args[1]
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(apiclient.StopBuildPlan(client.BaseURL(), path, ref))
			}
			if err := client.StopBuild(ctx, path, ref); err != nil {
				return err
			}
			return s.emit(map[string]any{"stopped": true, "job": path, "build": ref})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the request without stopping the build")
	return cmd
}

// pathAndRef splits a [<path>, <ref>] argument slice; a missing ref means the
// latest build.
func pathAndRef(args []string) (path, ref string) {
	path = args[0]
	if len(args) > 1 {
		ref = args[1]
	}
	return path, ref
}

// filterFailed keeps only failing test cases (FAILED or REGRESSION).
func filterFailed(cases []apiclient.TestCase) []apiclient.TestCase {
	out := cases[:0]
	for _, c := range cases {
		switch strings.ToUpper(c.Status) {
		case "FAILED", "REGRESSION", "ERROR":
			out = append(out, c)
		}
	}
	return out
}

// notPipelineHint augments a 404 from the stages endpoint with Pipeline-specific
// guidance.
func notPipelineHint(err error) error {
	ce := cerrors.AsCLIError(err)
	if ce != nil && ce.Code == "HTTP_NOT_FOUND" {
		return cerrors.New(cerrors.CategoryNotFound, "NOT_PIPELINE",
			"no stage data for that build").
			WithHint("`build stages` works only for Pipeline / multibranch jobs. "+
				"For a freestyle job, use `build log` and `build tests` instead.").
			WithNextSteps("jenkins-cli build get <path>", "jenkins-cli build log <path>")
	}
	return err
}

// noTestsHint augments a 404 from the test report endpoint.
func noTestsHint(err error) error {
	ce := cerrors.AsCLIError(err)
	if ce != nil && ce.Code == "HTTP_NOT_FOUND" {
		return cerrors.New(cerrors.CategoryNotFound, "NO_TEST_REPORT",
			"that build has no test report").
			WithHint("The build did not publish test results (no JUnit/xUnit step, or it "+
				"failed before tests ran). Check the console log for the failure.").
			WithNextSteps("jenkins-cli build log <path>", "jenkins-cli build stages <path>")
	}
	return err
}
