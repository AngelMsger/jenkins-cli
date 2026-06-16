package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/jenkins-cli/internal/errors"
)

// readOnlyClient wraps a Client and blocks every mutating method before a
// request leaves the process. Reads pass straight through; the three write
// methods (TriggerBuild, StopBuild, CancelQueueItem) return a structured
// READONLY_BLOCKED error.
//
// This is how the session read-only posture (defaults.read_only /
// JENKINS_CLI_READ_ONLY / --allow-writes) is enforced. As the control-plane
// surface grows (create/configure jobs, credentials, nodes), each new write
// method gets one override here and nothing else changes.
type readOnlyClient struct {
	Client
}

// NewReadOnly returns a read-only view of c.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Client: c}
}

func (r *readOnlyClient) TriggerBuild(context.Context, string, map[string]string) (*QueueRef, error) {
	return nil, blocked("trigger a build")
}

func (r *readOnlyClient) StopBuild(context.Context, string, string) error {
	return blocked("stop a build")
}

func (r *readOnlyClient) CancelQueueItem(context.Context, int) error {
	return blocked("cancel a queue item")
}

// blocked builds the structured error returned for a write attempt in read-only
// mode. Note --dry-run previews still work: they never call these methods.
func blocked(op string) error {
	return cerrors.Newf(cerrors.CategoryPermission, "READONLY_BLOCKED",
		"refusing to %s: read-only mode is enabled", op).
		WithHint("This session is read-only (defaults.read_only / JENKINS_CLI_READ_ONLY). "+
			"Preview the request with --dry-run, or re-run with --allow-writes to override.").
		WithNextSteps(
			"Add --dry-run to preview the request without sending it",
			"Add --allow-writes to override read-only mode for this command",
			"unset JENKINS_CLI_READ_ONLY / set defaults.read_only=false to disable it")
}
