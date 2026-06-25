package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/angelmsger/jenkins-cli/pkg/transport"
)

// newTestClient wires a Client to an httptest server with the given handler.
func newTestClient(t *testing.T, h http.HandlerFunc) (Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	tc := transport.New(transport.Options{Timeout: 5 * time.Second})
	return New(Config{BaseURL: srv.URL, Transport: tc}), srv
}

func TestListJobs(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/job/fx/api/json" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(`{"jobs":[
			{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowJob","name":"app","url":"http://x/job/app/","color":"blue","lastBuild":{"number":5,"url":"http://x/job/app/5/","result":"SUCCESS","timestamp":1700000000000,"duration":45000}},
			{"_class":"org.jenkinsci.plugins.workflow.job.WorkflowJob","name":"feature%2Flogin","url":"http://x/job/feature%252Flogin/","color":"red_anime","lastBuild":{"number":2,"result":"FAILURE","building":true}},
			{"_class":"com.cloudbees.hudson.plugins.folder.Folder","name":"team","url":"http://x/job/team/","color":""}
		]}`))
	})
	jobs, err := client.ListJobs(context.Background(), "fx", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want 3 jobs, got %d", len(jobs))
	}
	// Enriched last_build: result + timing in one listing call.
	lb := jobs[0].LastBuild
	if lb == nil || lb.Number != 5 || lb.Status != "success" || lb.StartedAt == "" || lb.Duration != "45s" {
		t.Errorf("job0 lastBuild = %+v", lb)
	}
	// Branch name with a slash: decoded for display, but the path keeps the
	// encoded form so it round-trips through jobPath.
	if jobs[1].Name != "feature/login" {
		t.Errorf("job1 name = %q, want feature/login", jobs[1].Name)
	}
	if jobs[1].Path != "fx/feature%2Flogin" {
		t.Errorf("job1 path = %q, want fx/feature%%2Flogin", jobs[1].Path)
	}
	if jobs[1].LastBuild == nil || jobs[1].LastBuild.Status != "building" || !jobs[1].LastBuild.Building {
		t.Errorf("job1 lastBuild = %+v", jobs[1].LastBuild)
	}
	if jobs[2].Kind != "folder" {
		t.Errorf("job2 kind = %q, want folder", jobs[2].Kind)
	}
}

func TestGetJobPathEncoding(t *testing.T) {
	var gotPath string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"_class":"x","name":"app","color":"red","buildable":true,
			"property":[{"parameterDefinitions":[{"name":"BRANCH","type":"StringParameterDefinition","defaultParameterValue":{"value":"main"}}]}],
			"lastFailedBuild":{"number":3,"url":"u","result":"FAILURE"}}`))
	})
	job, err := client.GetJob(context.Background(), "team/app")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/job/team/job/app/api/json" {
		t.Errorf("path = %q, want /job/team/job/app/api/json", gotPath)
	}
	if job.Status != "failure" {
		t.Errorf("status = %q, want failure", job.Status)
	}
	if len(job.Parameters) != 1 || job.Parameters[0].Name != "BRANCH" || job.Parameters[0].DefaultValue != "main" {
		t.Errorf("parameters = %+v", job.Parameters)
	}
	if job.LastFailure == nil || job.LastFailure.Number != 3 {
		t.Errorf("lastFailure = %+v", job.LastFailure)
	}
}

func TestGetBuildRefResolution(t *testing.T) {
	var gotPath string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"number":7,"result":"FAILURE","building":false,"duration":91000,"timestamp":1700000000000,"builtOn":"agent-1",
			"actions":[{"causes":[{"shortDescription":"Started by user Alice","userId":"alice","userName":"Alice"}]},{"parameters":[{"name":"BRANCH","value":"main"}]}]}`))
	})
	b, err := client.GetBuild(context.Background(), "app", "lastFailed")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/job/app/lastFailedBuild/api/json" {
		t.Errorf("path = %q", gotPath)
	}
	if b.Status != "failure" || b.Duration != "1m31s" {
		t.Errorf("status/duration = %q / %q", b.Status, b.Duration)
	}
	if b.StartedAt == "" || len(b.Causes) != 1 || b.Causes[0].UserID != "alice" {
		t.Errorf("build = %+v", b)
	}
	if len(b.Parameters) != 1 || b.Parameters[0].Name != "BRANCH" {
		t.Errorf("params = %+v", b.Parameters)
	}
}

func TestConsoleProgressive(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/logText/progressiveText") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("X-More-Data", "true")
		w.Header().Set("X-Text-Size", "1024")
		w.Write([]byte("line one\nline two\n"))
	})
	chunk, err := client.Console(context.Background(), "app", "5", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !chunk.More || chunk.NextStart != 1024 {
		t.Errorf("more/next = %v / %d", chunk.More, chunk.NextStart)
	}
	if !strings.Contains(chunk.Text, "line two") {
		t.Errorf("text = %q", chunk.Text)
	}
}

func TestStages(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"#7","status":"FAILED","durationMillis":120000,"stages":[
			{"name":"Build","status":"SUCCESS","durationMillis":30000},
			{"name":"Test","status":"FAILED","durationMillis":90000}]}`))
	})
	run, err := client.Stages(context.Background(), "app", "")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "FAILED" || len(run.Stages) != 2 {
		t.Fatalf("run = %+v", run)
	}
	if run.Stages[1].Name != "Test" || run.Stages[1].Status != "FAILED" {
		t.Errorf("stage = %+v", run.Stages[1])
	}
}

func TestTests(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"failCount":1,"skipCount":0,"passCount":2,"totalCount":3,"suites":[
			{"name":"S","cases":[
				{"className":"C","name":"ok","status":"PASSED","duration":0.1},
				{"className":"C","name":"bad","status":"FAILED","duration":0.2,"errorDetails":"boom"}]}]}`))
	})
	rep, err := client.Tests(context.Background(), "app", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 3 || rep.Failed != 1 || len(rep.Cases) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Cases[1].ErrorDetails != "boom" {
		t.Errorf("case = %+v", rep.Cases[1])
	}
}

func TestChangesPluralAndFallback(t *testing.T) {
	// Plural changeSets present.
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"changeSets":[{"kind":"git","items":[
			{"commitId":"abc","timestamp":1700000000000,"msg":"fix","author":{"fullName":"Bob"},"affectedPaths":["a.go"]}]}]}`))
	})
	cs, err := client.Changes(context.Background(), "app", "")
	if err != nil {
		t.Fatal(err)
	}
	if cs.Kind != "git" || len(cs.Commits) != 1 || cs.Commits[0].Author != "Bob" {
		t.Fatalf("changes = %+v", cs)
	}
	if cs.Commits[0].Message != "fix" || len(cs.Commits[0].AffectedPaths) != 1 {
		t.Errorf("commit = %+v", cs.Commits[0])
	}
}

func TestQueue(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"id":42,"why":"Waiting for next executor","blocked":true,"inQueueSince":1700000000000,"task":{"name":"app","url":"u"}}]}`))
	})
	items, err := client.ListQueue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 42 || items[0].Task != "app" || !items[0].Blocked {
		t.Fatalf("queue = %+v", items)
	}
}

func TestNotFoundError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	_, err := client.GetJob(context.Background(), "nope")
	ce := cerrors.AsCLIError(err)
	if ce == nil || ce.Code != "HTTP_NOT_FOUND" || ce.Category != cerrors.CategoryNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestWritePlans(t *testing.T) {
	base := "http://jenkins.example.com"
	p := TriggerBuildPlan(base, "team/app", map[string]string{"BRANCH": "main"})
	if p.Method != "POST" || p.URL != base+"/job/team/job/app/buildWithParameters" {
		t.Errorf("trigger plan = %+v", p)
	}
	if !strings.Contains(p.Body, "BRANCH=main") {
		t.Errorf("trigger body = %q", p.Body)
	}
	if np := TriggerBuildPlan(base, "app", nil); np.URL != base+"/job/app/build" || np.Body != "" {
		t.Errorf("no-param trigger plan = %+v", np)
	}
	if sp := StopBuildPlan(base, "app", "5"); sp.URL != base+"/job/app/5/stop" {
		t.Errorf("stop plan = %+v", sp)
	}
	if cp := CancelQueuePlan(base, 9); cp.URL != base+"/queue/cancelItem?id=9" {
		t.Errorf("cancel plan = %+v", cp)
	}
}

func TestReadOnlyBlocksWrites(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("read-only client must not send a request (%s %s)", r.Method, r.URL.Path)
	})
	ro := NewReadOnly(client)
	if _, err := ro.TriggerBuild(context.Background(), "app", nil); !isReadonlyBlocked(err) {
		t.Errorf("TriggerBuild err = %v", err)
	}
	if err := ro.StopBuild(context.Background(), "app", "5"); !isReadonlyBlocked(err) {
		t.Errorf("StopBuild err = %v", err)
	}
	if err := ro.CancelQueueItem(context.Background(), 1); !isReadonlyBlocked(err) {
		t.Errorf("CancelQueueItem err = %v", err)
	}
}

func isReadonlyBlocked(err error) bool {
	ce := cerrors.AsCLIError(err)
	return ce != nil && ce.Code == "READONLY_BLOCKED"
}
