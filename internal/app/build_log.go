package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/angelmsger/jenkins-cli/internal/apiclient"
	"github.com/angelmsger/jenkins-cli/internal/output"
	"github.com/spf13/cobra"
)

// followPollInterval is how long `build log --follow` waits between polls of the
// progressive console while a build is still running.
const followPollInterval = 2 * time.Second

func newBuildLogCmd(s *appState) *cobra.Command {
	var (
		follow bool
		start  int64
	)
	cmd := &cobra.Command{
		Use:   "log <path> [ref]",
		Short: "Print a build's console output",
		Long: "Prints the build's console log as plain text to stdout. Use --start to\n" +
			"resume from a byte offset, or --follow to stream new output until the\n" +
			"build finishes (Ctrl-C to stop). ref defaults to the latest build.",
		Example: "  jenkins-cli build log my-app lastFailed\n" +
			"  jenkins-cli build log my-app --follow",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, ref := pathAndRef(args)
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if follow {
				return s.followLog(client, path, ref, start)
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			chunk, err := client.Console(ctx, path, ref, start)
			if err != nil {
				return err
			}
			fmt.Fprint(os.Stdout, chunk.Text)
			if chunk.More {
				// Build still running: tell the agent how to resume or follow.
				output.EmitNotice(os.Stderr, map[string]any{"_notice": map[string]any{
					"more":       true,
					"next_start": chunk.NextStart,
					"hint":       "build still running; re-run with --start <next_start> or --follow",
				}})
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&follow, "follow", false, "stream new output until the build finishes (Ctrl-C to stop)")
	f.Int64Var(&start, "start", 0, "start byte offset into the console log")
	return cmd
}

// followLog streams the progressive console until the build completes or the
// user interrupts. Each poll is bounded by the transport's request timeout; the
// overall loop runs until the build finishes or SIGINT/SIGTERM.
func (s *appState) followLog(client apiclient.Client, path, ref string, start int64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		chunk, err := client.Console(ctx, path, ref, start)
		if err != nil {
			if ctx.Err() != nil {
				return nil // interrupted; a partial stream is a clean stop
			}
			return err
		}
		fmt.Fprint(os.Stdout, chunk.Text)
		start = chunk.NextStart
		if !chunk.More {
			return nil // build finished
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(followPollInterval):
		}
	}
}
