package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/angelmsger/jenkins-cli/internal/timeutil"
)

// raw* mirror the Jenkins api/json shapes before normalization into our models.
type rawJob struct {
	Class               string         `json:"_class"`
	Name                string         `json:"name"`
	URL                 string         `json:"url"`
	Color               string         `json:"color"`
	Buildable           bool           `json:"buildable"`
	Description         string         `json:"description"`
	HealthReport        []HealthReport `json:"healthReport"`
	Property            []rawProperty  `json:"property"`
	LastBuild           *rawBuildRef   `json:"lastBuild"`
	LastSuccessfulBuild *rawBuildRef   `json:"lastSuccessfulBuild"`
	LastFailedBuild     *rawBuildRef   `json:"lastFailedBuild"`
	LastCompletedBuild  *rawBuildRef   `json:"lastCompletedBuild"`
	Jobs                []rawJob       `json:"jobs"`
	Builds              []rawBuild     `json:"builds"`
}

type rawProperty struct {
	ParameterDefinitions []rawParamDef `json:"parameterDefinitions"`
}

type rawParamDef struct {
	Name                  string `json:"name"`
	Type                  string `json:"type"`
	Description           string `json:"description"`
	DefaultParameterValue *struct {
		Value any `json:"value"`
	} `json:"defaultParameterValue"`
}

type rawBuildRef struct {
	Number    int    `json:"number"`
	URL       string `json:"url"`
	Result    string `json:"result"`
	Building  bool   `json:"building"`
	Timestamp int64  `json:"timestamp"`
	Duration  int64  `json:"duration"`
}

// lastBuildTree is the projection for a job's last-build pointer. It is rich
// enough to answer "did it pass, when, how long, is it still running" for each
// branch in one listing call — no per-branch follow-up.
const lastBuildTree = "lastBuild[number,url,result,building,timestamp,duration]"

// jobsTree builds a ?tree= expression for nested job listings down to depth
// levels (depth 0 = just the immediate children).
func jobsTree(depth int) string {
	leaf := "name,url,color,_class," + lastBuildTree
	expr := leaf
	for i := 0; i < depth; i++ {
		expr = leaf + ",jobs[" + expr + "]"
	}
	return "jobs[" + expr + "]"
}

// ListJobs returns the jobs under folder ("" = instance root).
func (c *apiClient) ListJobs(ctx context.Context, folder string, depth int) ([]Job, error) {
	if depth < 0 {
		depth = 0
	}
	q := url.Values{"tree": {jobsTree(depth)}}
	var raw struct {
		Jobs []rawJob `json:"jobs"`
	}
	if err := c.getJSON(ctx, jobPath(folder)+"/api/json", q, &raw); err != nil {
		return nil, err
	}
	parent := pathDisplay(folder)
	out := make([]Job, 0, len(raw.Jobs))
	for _, rj := range raw.Jobs {
		out = append(out, toJob(rj, parent))
	}
	return out, nil
}

const jobDetailTree = "name,url,color,_class,buildable,description," +
	"healthReport[score,description]," +
	"property[parameterDefinitions[name,type,description,defaultParameterValue[value]]]," +
	lastBuildTree + ",lastSuccessfulBuild[number,url,result]," +
	"lastFailedBuild[number,url,result],lastCompletedBuild[number,url,result]," +
	"jobs[name,url,color,_class," + lastBuildTree + "]"

// GetJob returns one job by human path, including parameters, health, last-build
// pointers and (for folders / multibranch) the child jobs.
func (c *apiClient) GetJob(ctx context.Context, path string) (*Job, error) {
	q := url.Values{"tree": {jobDetailTree}}
	var rj rawJob
	if err := c.getJSON(ctx, jobPath(path)+"/api/json", q, &rj); err != nil {
		return nil, err
	}
	parent := parentPath(path)
	job := toJob(rj, parent)
	return &job, nil
}

const buildHistoryTree = "number,url,result,building,timestamp,duration,displayName,builtOn," +
	"actions[causes[shortDescription,userId,userName,upstreamProject,upstreamBuild]]"

// JobBuilds returns a job's recent build history, newest first.
func (c *apiClient) JobBuilds(ctx context.Context, path string, limit int) ([]Build, error) {
	if limit <= 0 {
		limit = 20
	}
	tree := "builds[" + buildHistoryTree + "]{0," + strconv.Itoa(limit) + "}"
	q := url.Values{"tree": {tree}}
	var raw struct {
		Builds []rawBuild `json:"builds"`
	}
	if err := c.getJSON(ctx, jobPath(path)+"/api/json", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Build, 0, len(raw.Builds))
	for _, rb := range raw.Builds {
		out = append(out, toBuild(rb))
	}
	return out, nil
}

// toJob normalizes a rawJob into a Job, computing the human path from the
// parent path and the job name.
func toJob(r rawJob, parent string) Job {
	j := Job{
		Class: r.Class,
		Kind:  kindFromClass(r.Class),
		// Jenkins encodes a branch name's slash in the job name (feature/x ->
		// feature%2Fx). Show the human form, but keep the encoded name in the
		// path so it round-trips back through jobPath unchanged.
		Name:        displayName(r.Name),
		Path:        joinPath(parent, r.Name),
		URL:         r.URL,
		Color:       r.Color,
		Buildable:   r.Buildable,
		Description: r.Description,
		Health:      r.HealthReport,
	}
	if r.Color != "" {
		j.Status = statusFromColor(r.Color)
	}
	for _, p := range r.Property {
		for _, pd := range p.ParameterDefinitions {
			def := ParameterDef{Name: pd.Name, Type: pd.Type, Description: pd.Description}
			if pd.DefaultParameterValue != nil {
				def.DefaultValue = pd.DefaultParameterValue.Value
			}
			j.Parameters = append(j.Parameters, def)
		}
	}
	j.LastBuild = toBuildRef(r.LastBuild)
	j.LastSuccess = toBuildRef(r.LastSuccessfulBuild)
	j.LastFailure = toBuildRef(r.LastFailedBuild)
	j.LastComplete = toBuildRef(r.LastCompletedBuild)
	for _, child := range r.Jobs {
		j.Jobs = append(j.Jobs, toJob(child, j.Path))
	}
	return j
}

func toBuildRef(r *rawBuildRef) *BuildRef {
	if r == nil || r.Number == 0 {
		return nil
	}
	b := &BuildRef{Number: r.Number, URL: r.URL, Result: r.Result, Building: r.Building}
	switch {
	case r.Building:
		b.Status = "building"
	case r.Result != "":
		b.Status = strings.ToLower(r.Result)
	}
	if r.Timestamp > 0 {
		t := timeutil.FromMillis(r.Timestamp)
		b.StartedAt = t.Format(time.RFC3339)
		b.StartedAgo = timeutil.HumanSince(t)
	}
	if r.Duration > 0 {
		b.Duration = durationHuman(r.Duration)
	}
	return b
}

// displayName returns the human form of a Jenkins job name, decoding the
// percent-encoding Jenkins applies to branch names that contain a slash. If it
// is not valid percent-encoding, the raw name is returned unchanged.
func displayName(name string) string {
	if dec, err := url.PathUnescape(name); err == nil {
		return dec
	}
	return name
}

// joinPath joins a parent path and a name with "/", skipping an empty parent.
func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

// parentPath returns everything but the last segment of a human path.
func parentPath(path string) string {
	p := pathDisplay(path)
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}
