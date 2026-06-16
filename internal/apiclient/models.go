package apiclient

import "strings"

// Job is a Jenkins job, folder or multibranch project. The same shape backs the
// discovery listing (a compact map) and `job get` (the full detail); fields the
// server did not return stay zero/empty.
type Job struct {
	Class        string         `json:"class"` // Jenkins _class, e.g. WorkflowJob, Folder
	Kind         string         `json:"kind"`  // friendly type: folder, multibranch, pipeline, freestyle, job
	Name         string         `json:"name"`
	Path         string         `json:"path,omitempty"` // human path relative to the instance root (folder/job)
	URL          string         `json:"url,omitempty"`
	Color        string         `json:"color,omitempty"`  // raw Jenkins color (blue, red, yellow, *_anime)
	Status       string         `json:"status,omitempty"` // derived from color: success, failure, unstable, building, disabled, not_built
	Buildable    bool           `json:"buildable,omitempty"`
	Description  string         `json:"description,omitempty"`
	Health       []HealthReport `json:"health,omitempty"`
	Parameters   []ParameterDef `json:"parameters,omitempty"`
	LastBuild    *BuildRef      `json:"last_build,omitempty"`
	LastSuccess  *BuildRef      `json:"last_successful_build,omitempty"`
	LastFailure  *BuildRef      `json:"last_failed_build,omitempty"`
	LastComplete *BuildRef      `json:"last_completed_build,omitempty"`
	// Jobs holds nested children for folders and multibranch projects (the
	// branch / PR jobs). Populated by `job list --depth` and `job get`.
	Jobs []Job `json:"jobs,omitempty"`
}

// HealthReport is one Jenkins job health line (the weather icon score).
type HealthReport struct {
	Score       int    `json:"score"`
	Description string `json:"description,omitempty"`
}

// ParameterDef is one build parameter a job declares.
type ParameterDef struct {
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	DefaultValue any    `json:"default,omitempty"`
	Description  string `json:"description,omitempty"`
}

// BuildRef is a pointer to a build (number + url) with a compact snapshot of
// its outcome and timing — enough to read a job's (or a multibranch branch's)
// last-build situation without a follow-up call.
type BuildRef struct {
	Number     int    `json:"number"`
	URL        string `json:"url,omitempty"`
	Result     string `json:"result,omitempty"`
	Status     string `json:"status,omitempty"` // lowercase result, or "building"
	Building   bool   `json:"building,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`  // RFC3339
	StartedAgo string `json:"started_ago,omitempty"` // relative, e.g. "3h ago"
	Duration   string `json:"duration,omitempty"`    // human, e.g. 2m58s
}

// Build is one build (run) of a job.
type Build struct {
	Number        int     `json:"number"`
	DisplayName   string  `json:"display_name,omitempty"`
	URL           string  `json:"url,omitempty"`
	Result        string  `json:"result,omitempty"` // SUCCESS, FAILURE, UNSTABLE, ABORTED, NOT_BUILT
	Status        string  `json:"status,omitempty"` // lowercase result, or "building"
	Building      bool    `json:"building"`
	StartedAt     string  `json:"started_at,omitempty"` // RFC3339, from the epoch-ms timestamp
	StartedAgo    string  `json:"started_ago,omitempty"`
	TimestampMs   int64   `json:"timestamp_ms,omitempty"`
	DurationMs    int64   `json:"duration_ms,omitempty"`
	Duration      string  `json:"duration,omitempty"` // human, e.g. 1m30s
	BuiltOn       string  `json:"built_on,omitempty"` // agent / node name
	Causes        []Cause `json:"causes,omitempty"`   // who / what triggered it
	Parameters    []KV    `json:"parameters,omitempty"`
	ChangesetKind string  `json:"changeset_kind,omitempty"`
	ChangeCount   int     `json:"change_count,omitempty"`
}

// Cause explains why a build started (a user, a timer, an upstream build, SCM).
type Cause struct {
	ShortDescription string `json:"description,omitempty"`
	UserID           string `json:"user_id,omitempty"`
	UserName         string `json:"user_name,omitempty"`
	UpstreamProject  string `json:"upstream_project,omitempty"`
	UpstreamBuild    int    `json:"upstream_build,omitempty"`
}

// KV is a name/value pair (build parameters).
type KV struct {
	Name  string `json:"name"`
	Value any    `json:"value,omitempty"`
}

// ConsoleChunk is one slice of a build's console output. More reports whether
// the build is still producing output; NextStart is the byte offset to pass as
// --start on the next fetch (progressive console).
type ConsoleChunk struct {
	Text      string `json:"text"`
	More      bool   `json:"more"`
	NextStart int64  `json:"next_start"`
}

// PipelineRun is a Pipeline build's stage breakdown (Jenkins wfapi/describe):
// where the run is in its stages and which stage failed.
type PipelineRun struct {
	Name       string  `json:"name,omitempty"`
	Status     string  `json:"status,omitempty"` // SUCCESS, FAILED, IN_PROGRESS, ...
	StartedAt  string  `json:"started_at,omitempty"`
	DurationMs int64   `json:"duration_ms,omitempty"`
	Duration   string  `json:"duration,omitempty"`
	Stages     []Stage `json:"stages"`
}

// Stage is one Pipeline stage.
type Stage struct {
	Name       string `json:"name"`
	Status     string `json:"status,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Duration   string `json:"duration,omitempty"`
}

// TestReport is a build's test results (Jenkins testReport).
type TestReport struct {
	Total   int        `json:"total"`
	Passed  int        `json:"passed"`
	Failed  int        `json:"failed"`
	Skipped int        `json:"skipped"`
	Cases   []TestCase `json:"cases,omitempty"`
}

// TestCase is one test case result.
type TestCase struct {
	Suite        string  `json:"suite,omitempty"`
	ClassName    string  `json:"class_name,omitempty"`
	Name         string  `json:"name"`
	Status       string  `json:"status"` // PASSED, FAILED, SKIPPED, FIXED, REGRESSION
	DurationSec  float64 `json:"duration_sec,omitempty"`
	ErrorDetails string  `json:"error_details,omitempty"`
	ErrorStack   string  `json:"error_stack,omitempty"`
}

// ChangeSet is a build's SCM changes (commits since the previous build).
type ChangeSet struct {
	Kind    string   `json:"kind,omitempty"`
	Commits []Commit `json:"commits"`
}

// Commit is one SCM commit in a changeset.
type Commit struct {
	ID            string   `json:"id,omitempty"`
	Author        string   `json:"author,omitempty"`
	AuthoredAt    string   `json:"authored_at,omitempty"`
	Message       string   `json:"message,omitempty"`
	AffectedPaths []string `json:"affected_paths,omitempty"`
}

// Artifact is one archived build artifact.
type Artifact struct {
	FileName     string `json:"file_name"`
	RelativePath string `json:"relative_path"`
	URL          string `json:"url,omitempty"`
}

// QueueItem is one entry in the build queue (a build waiting to start).
type QueueItem struct {
	ID           int    `json:"id"`
	Why          string `json:"why,omitempty"`
	Task         string `json:"task,omitempty"` // the job name the item will build
	URL          string `json:"url,omitempty"`
	Blocked      bool   `json:"blocked"`
	Buildable    bool   `json:"buildable"`
	Stuck        bool   `json:"stuck"`
	Pending      bool   `json:"pending"`
	InQueueSince string `json:"in_queue_since,omitempty"`
	Params       string `json:"params,omitempty"`
}

// QueueRef points at a queued build created by triggering a job.
type QueueRef struct {
	QueueURL string `json:"queue_url,omitempty"`
	QueueID  int    `json:"queue_id,omitempty"`
}

// WritePlan describes the HTTP request a write command would send. It backs
// --dry-run: a credentials-free preview that sends nothing.
type WritePlan struct {
	DryRun bool   `json:"dry_run"`
	Method string `json:"method"`
	URL    string `json:"url"`
	Body   string `json:"body,omitempty"`
}

// statusFromColor maps a Jenkins job "color" to a stable status word. Jenkins
// encodes the last build's outcome in the color, with an "_anime" suffix while
// a build is running.
func statusFromColor(color string) string {
	building := strings.HasSuffix(color, "_anime")
	base := strings.TrimSuffix(color, "_anime")
	switch base {
	case "blue", "green":
		if building {
			return "building"
		}
		return "success"
	case "red":
		if building {
			return "building"
		}
		return "failure"
	case "yellow":
		if building {
			return "building"
		}
		return "unstable"
	case "aborted":
		return "aborted"
	case "disabled":
		return "disabled"
	case "notbuilt", "grey", "":
		if building {
			return "building"
		}
		return "not_built"
	default:
		if building {
			return "building"
		}
		return base
	}
}

// kindFromClass maps a Jenkins _class to a friendly job kind.
func kindFromClass(class string) string {
	switch {
	case class == "":
		return ""
	case strings.Contains(class, "OrganizationFolder"):
		return "org_folder"
	case strings.Contains(class, "MultiBranch"):
		return "multibranch"
	case strings.Contains(class, "Folder"):
		return "folder"
	case strings.Contains(class, "WorkflowJob"):
		return "pipeline"
	case strings.Contains(class, "FreeStyleProject"):
		return "freestyle"
	default:
		return "job"
	}
}
