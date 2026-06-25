package app

import (
	"strconv"

	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newQueueCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Inspect and manage the build queue",
		Long: "The queue holds builds waiting to start. List it to see what is pending\n" +
			"or blocked and why, inspect one item, or cancel a queued build before it\n" +
			"runs.",
	}
	cmd.AddCommand(
		newQueueListCmd(s),
		newQueueGetCmd(s),
		newQueueCancelCmd(s),
	)
	return cmd
}

func newQueueListCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List builds waiting in the queue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			items, err := client.ListQueue(ctx)
			if err != nil {
				return err
			}
			return s.emitList(items, pageInfo{})
		},
	}
	return cmd
}

func newQueueGetCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get one queue item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseQueueID(args[0])
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			item, err := client.GetQueueItem(ctx, id)
			if err != nil {
				return err
			}
			return s.emit(item)
		},
	}
	return cmd
}

func newQueueCancelCmd(s *appState) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a queued build before it starts (write)",
		Long: "Removes a still-queued build by its queue id. With --dry-run, print the\n" +
			"request that would be sent and exit. This is a write: it is blocked in\n" +
			"read-only mode unless --allow-writes is given.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseQueueID(args[0])
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient()
			if err != nil {
				return err
			}
			if dryRun {
				return s.emit(apiclient.CancelQueuePlan(client.BaseURL(), id))
			}
			if err := client.CancelQueueItem(ctx, id); err != nil {
				return err
			}
			return s.emit(map[string]any{"cancelled": true, "queue_id": id})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the request without cancelling")
	return cmd
}

// parseQueueID parses a queue id argument into an int.
func parseQueueID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return 0, cerrors.Newf(cerrors.CategoryUsage, "BAD_QUEUE_ID",
			"queue id %q must be an integer", arg).
			WithNextSteps("jenkins-cli queue list")
	}
	return id, nil
}
