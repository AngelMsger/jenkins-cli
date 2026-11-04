package apiclient_test

import (
	"context"

	"github.com/angelmsger/jenkins-cli/pkg/apiclient"
)

// The method sets below are the library contract external callers and
// implementations compile against. Each pair of assignments fails to compile
// when a method's signature changes or a method is added or removed, which
// breaks them. Extend the contract with new functions or types instead; when
// a method must change, it is a breaking change for every library user.

type readerContract interface {
	BaseURL() string
	Ping(ctx context.Context) error
	WhoAmI(ctx context.Context) (*apiclient.User, error)
	ListJobs(ctx context.Context, folder string, depth int) ([]apiclient.Job, error)
	GetJob(ctx context.Context, path string) (*apiclient.Job, error)
	JobBuilds(ctx context.Context, path string, limit int) ([]apiclient.Build, error)
	GetBuild(ctx context.Context, path, ref string) (*apiclient.Build, error)
	Console(ctx context.Context, path, ref string, start int64) (*apiclient.ConsoleChunk, error)
	Stages(ctx context.Context, path, ref string) (*apiclient.PipelineRun, error)
	Tests(ctx context.Context, path, ref string) (*apiclient.TestReport, error)
	Changes(ctx context.Context, path, ref string) (*apiclient.ChangeSet, error)
	Artifacts(ctx context.Context, path, ref string) ([]apiclient.Artifact, error)
	ListQueue(ctx context.Context) ([]apiclient.QueueItem, error)
	GetQueueItem(ctx context.Context, id int) (*apiclient.QueueItem, error)
}

type writerContract interface {
	TriggerBuild(ctx context.Context, path string, params map[string]string) (*apiclient.QueueRef, error)
	StopBuild(ctx context.Context, path, ref string) error
	CancelQueueItem(ctx context.Context, id int) error
}

var (
	_ readerContract   = apiclient.Reader(nil)
	_ apiclient.Reader = readerContract(nil)
	_ writerContract   = apiclient.Writer(nil)
	_ apiclient.Writer = writerContract(nil)
)

// A caller written against the released StopBuild still compiles.
var _ = func(c apiclient.Client) error {
	return c.StopBuild(context.Background(), "app", "last")
}
