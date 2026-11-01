package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// NewReadOnly wraps c so that every mutating method returns a READONLY_BLOCKED
// error before a request leaves the process. Reads (the Reader half of Client)
// pass straight through.
//
// This is how the session read-only posture (defaults.read_only /
// JENKINS_CLI_READ_ONLY / --allow-writes) is enforced.
func NewReadOnly(c Client) Client {
	return &readOnlyClient{Reader: c}
}

// readOnlyClient is the read-only enforcement layer. It keeps the inner client
// only as a Reader, so reads are delegated unchanged and no write can reach it.
// Every Writer method is overridden below to return READONLY_BLOCKED; the
// assertion that follows stops compilation when a new Writer method (create or
// configure jobs, credentials, nodes, ...) lacks its override.
type readOnlyClient struct {
	Reader
}

var _ Client = (*readOnlyClient)(nil)

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
