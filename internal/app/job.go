package app

import (
	"strings"

	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newJobCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Discover jobs and trigger builds",
		Long: "Jobs are the projects, folders and multibranch pipelines on the\n" +
			"instance. `job list` is the discovery map; `job get` shows one job's\n" +
			"detail (parameters, health, branch jobs, last-build pointers); `job\n" +
			"build` triggers a build. Job paths are folder/job[/branch] — list to\n" +
			"find the exact name before querying.",
	}
	cmd.AddCommand(
		newJobListCmd(s),
		newJobGetCmd(s),
		newJobBuildCmd(s),
	)
	return cmd
}

func newJobListCmd(s *appState) *cobra.Command {
	var (
		folder string
		depth  int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List jobs (the discovery map)",
		Long: "Lists jobs with their type, status and last-build snapshot. Pass --folder\n" +
			"to list inside a folder — or a multibranch project, which lists its\n" +
			"branch / PR jobs — and --depth to recurse nested containers. Each job's\n" +
			"last_build carries its result, start time and duration, so one call shows\n" +
			"every branch's build situation. Status is derived from Jenkins' color\n" +
			"field (success / failure / unstable / building / disabled / not_built).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			jobs, err := client.ListJobs(ctx, folder, depth)
			if err != nil {
				return err
			}
			return s.emitList(jobs, pageInfo{})
		},
	}
	f := cmd.Flags()
	f.StringVar(&folder, "folder", "", "list jobs inside this folder, or branches of this multibranch project")
	f.IntVar(&depth, "depth", 0, "recurse nested folders this many levels (0 = immediate children)")
	return cmd
}

func newJobGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Get one job, including parameters and last-build pointers",
		Long: "Returns a single job by path (folder/job[/branch]): its description,\n" +
			"build parameters, health, last / lastSuccessful / lastFailed build\n" +
			"pointers, and — for folders and multibranch projects — the child jobs\n" +
			"(branches and PRs).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			job, err := client.GetJob(ctx, args[0])
			if err != nil {
				return err
			}
			return s.emit(job)
		},
	}
	return cmd
}

func newJobBuildCmd(s *appState) *cobra.Command {
	var (
		params []string
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "build <path>",
		Short: "Trigger a build of a job (write)",
		Long: "Starts a build of the job at <path>. Pass --param KEY=VALUE (repeatable)\n" +
			"for a parameterized job. With --dry-run, print the request that would be\n" +
			"sent and exit without triggering. This is a write: it is blocked in\n" +
			"read-only mode unless --allow-writes is given.",
		Example: "  jenkins-cli job build my-folder/my-app\n" +
			"  jenkins-cli job build my-app --param BRANCH=main --param CLEAN=true\n" +
			"  jenkins-cli job build my-app --param BRANCH=main --dry-run",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			kv, err := parseParams(params)
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(apiclient.TriggerBuildPlan(client.BaseURL(), args[0], kv))
			}
			ref, err := client.TriggerBuild(ctx, args[0], kv)
			if err != nil {
				return err
			}
			return s.emit(map[string]any{"triggered": true, "job": args[0], "queue": ref})
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&params, "param", nil, "build parameter KEY=VALUE (repeatable)")
	f.BoolVar(&dryRun, "dry-run", false, "print the request without triggering the build")
	return cmd
}

// parseParams turns repeated KEY=VALUE strings into a map.
func parseParams(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_PARAM",
				"parameter %q must be KEY=VALUE", p).
				WithHint("Pass each parameter as --param NAME=value.")
		}
		out[k] = v
	}
	return out, nil
}
