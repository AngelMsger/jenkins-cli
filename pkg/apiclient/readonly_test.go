package apiclient

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// assertCoversMethods fails unless names is exactly the method set of iface,
// so a table test cannot silently skip a method added to the interface.
func assertCoversMethods(t *testing.T, iface reflect.Type, names []string) {
	t.Helper()
	var want []string
	for i := range iface.NumMethod() {
		want = append(want, iface.Method(i).Name)
	}
	got := slices.Sorted(slices.Values(names))
	if !slices.Equal(got, want) {
		t.Fatalf("table covers %v, %s declares %v", got, iface.Name(), want)
	}
}

// TestReadOnlyBlocksEveryWrite drives every Writer method on the wrapper and
// asserts each one returns a READONLY_BLOCKED permission error that names the
// operation, before any HTTP request is sent.
func TestReadOnlyBlocksEveryWrite(t *testing.T) {
	t.Parallel()
	inner, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read-only client must not send a request (%s %s)", r.Method, r.URL.Path)
	})
	c := NewReadOnly(inner)
	ctx := context.Background()

	cases := []struct {
		name string
		op   string // the operation the error message must name
		fn   func() error
	}{
		{"TriggerBuild", "trigger a build", func() error {
			_, err := c.TriggerBuild(ctx, "app", map[string]string{"BRANCH": "main"})
			return err
		}},
		{"StopBuild", "stop a build", func() error { return c.StopBuild(ctx, "app", "5") }},
		{"CancelQueueItem", "cancel a queue item", func() error { return c.CancelQueueItem(ctx, 1) }},
	}
	var names []string
	for _, tc := range cases {
		names = append(names, tc.name)
	}
	assertCoversMethods(t, reflect.TypeFor[Writer](), names)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ce := cerrors.AsCLIError(tc.fn())
			if ce == nil {
				t.Fatalf("%s: expected a CLIError, got nil", tc.name)
			}
			if ce.Category != cerrors.CategoryPermission || ce.Code != "READONLY_BLOCKED" {
				t.Errorf("%s: got %s/%s, want permission/READONLY_BLOCKED", tc.name, ce.Category, ce.Code)
			}
			if !strings.Contains(ce.Message, tc.op) {
				t.Errorf("%s: message %q does not name the operation %q", tc.name, ce.Message, tc.op)
			}
			if !strings.Contains(strings.Join(ce.NextSteps, " "), "--allow-writes") {
				t.Errorf("%s: next_steps missing --allow-writes hint: %v", tc.name, ce.NextSteps)
			}
		})
	}
}

// StopBuildNumber resolves a permalink with a read before it stops. On a
// read-only client it is blocked before that read, like StopBuild.
func TestReadOnlyBlocksStopBuildNumber(t *testing.T) {
	inner, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read-only client must not send a request (%s %s)", r.Method, r.URL.Path)
	})
	_, err := StopBuildNumber(context.Background(), NewReadOnly(inner), "app", "last")
	if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "READONLY_BLOCKED" || !strings.Contains(ce.Message, "stop a build") {
		t.Fatalf("got %+v; want READONLY_BLOCKED for stop a build", ce)
	}
}

// readCall is one call a recordingReader received.
type readCall struct {
	method string
	args   []any
}

// errCannedRead is returned alongside every canned read result, so delegation
// tests also see errors pass through unchanged.
var errCannedRead = errors.New("canned read error")

// recordingReader answers every Reader method with a fixed result and records
// the call. It embeds Client only to satisfy NewReadOnly's parameter type: a
// write that reached it would panic on the nil embedded interface.
type recordingReader struct {
	Client
	calls []readCall
}

func (r *recordingReader) record(method string, args ...any) {
	r.calls = append(r.calls, readCall{method: method, args: args})
}

func (r *recordingReader) BaseURL() string {
	r.record("BaseURL")
	return "BaseURL"
}

func (r *recordingReader) Ping(ctx context.Context) error {
	r.record("Ping", ctx)
	return errCannedRead
}

func (r *recordingReader) WhoAmI(ctx context.Context) (*User, error) {
	r.record("WhoAmI", ctx)
	return &User{}, errCannedRead
}

func (r *recordingReader) ListJobs(ctx context.Context, folder string, depth int) ([]Job, error) {
	r.record("ListJobs", ctx, folder, depth)
	return []Job{{}}, errCannedRead
}

func (r *recordingReader) GetJob(ctx context.Context, path string) (*Job, error) {
	r.record("GetJob", ctx, path)
	return &Job{}, errCannedRead
}

func (r *recordingReader) JobBuilds(ctx context.Context, path string, limit int) ([]Build, error) {
	r.record("JobBuilds", ctx, path, limit)
	return []Build{{}}, errCannedRead
}

func (r *recordingReader) GetBuild(ctx context.Context, path string, ref string) (*Build, error) {
	r.record("GetBuild", ctx, path, ref)
	return &Build{}, errCannedRead
}

func (r *recordingReader) Console(ctx context.Context, path string, ref string, start int64) (*ConsoleChunk, error) {
	r.record("Console", ctx, path, ref, start)
	return &ConsoleChunk{}, errCannedRead
}

func (r *recordingReader) Stages(ctx context.Context, path string, ref string) (*PipelineRun, error) {
	r.record("Stages", ctx, path, ref)
	return &PipelineRun{}, errCannedRead
}

func (r *recordingReader) Tests(ctx context.Context, path string, ref string) (*TestReport, error) {
	r.record("Tests", ctx, path, ref)
	return &TestReport{}, errCannedRead
}

func (r *recordingReader) Changes(ctx context.Context, path string, ref string) (*ChangeSet, error) {
	r.record("Changes", ctx, path, ref)
	return &ChangeSet{}, errCannedRead
}

func (r *recordingReader) Artifacts(ctx context.Context, path string, ref string) ([]Artifact, error) {
	r.record("Artifacts", ctx, path, ref)
	return []Artifact{{}}, errCannedRead
}

func (r *recordingReader) ListQueue(ctx context.Context) ([]QueueItem, error) {
	r.record("ListQueue", ctx)
	return []QueueItem{{}}, errCannedRead
}

func (r *recordingReader) GetQueueItem(ctx context.Context, id int) (*QueueItem, error) {
	r.record("GetQueueItem", ctx, id)
	return &QueueItem{}, errCannedRead
}

type delegationKey struct{}

// TestReadOnlyDelegatesEveryRead calls every Reader method through the
// wrapper and asserts the inner client received exactly that one call with the
// same arguments, and that its results and error came back unchanged.
func TestReadOnlyDelegatesEveryRead(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), delegationKey{}, "read-only delegation")
	// argsFor builds fresh arguments on every call, so the expected values are
	// independent of the ones handed to the wrapper: a wrapper that mutated an
	// argument in place before forwarding it would otherwise change the
	// expectation along with it.
	argsFor := func() map[string][]any {
		return map[string][]any{
			"BaseURL":      nil,
			"Ping":         {ctx},
			"WhoAmI":       {ctx},
			"ListJobs":     {ctx, "folder", 7},
			"GetJob":       {ctx, "path"},
			"JobBuilds":    {ctx, "path", 7},
			"GetBuild":     {ctx, "path", "ref"},
			"Console":      {ctx, "path", "ref", int64(7)},
			"Stages":       {ctx, "path", "ref"},
			"Tests":        {ctx, "path", "ref"},
			"Changes":      {ctx, "path", "ref"},
			"Artifacts":    {ctx, "path", "ref"},
			"ListQueue":    {ctx},
			"GetQueueItem": {ctx, 7},
		}
	}
	assertCoversMethods(t, reflect.TypeFor[Reader](), slices.Collect(maps.Keys(argsFor())))

	call := func(c Client, method string, args []any) []any {
		in := make([]reflect.Value, len(args))
		for i, a := range args {
			in[i] = reflect.ValueOf(a)
		}
		var out []any
		for _, v := range reflect.ValueOf(c).MethodByName(method).Call(in) {
			out = append(out, v.Interface())
		}
		return out
	}

	for method := range argsFor() {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			inner := &recordingReader{}
			got := call(NewReadOnly(inner), method, argsFor()[method])
			expected := argsFor()[method]
			if want := call(&recordingReader{}, method, expected); !reflect.DeepEqual(got, want) {
				t.Errorf("results = %#v, want the inner client's %#v", got, want)
			}
			want := []readCall{{method: method, args: expected}}
			if !reflect.DeepEqual(inner.calls, want) {
				t.Errorf("inner client received %#v, want %#v", inner.calls, want)
			}
		})
	}
}
