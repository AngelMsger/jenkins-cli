package apiclient

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/angelmsger/jenkins-cli/internal/timeutil"
)

type rawBuild struct {
	Class       string        `json:"_class"`
	Number      int           `json:"number"`
	DisplayName string        `json:"displayName"`
	URL         string        `json:"url"`
	Result      string        `json:"result"`
	Building    bool          `json:"building"`
	Timestamp   int64         `json:"timestamp"`
	Duration    int64         `json:"duration"`
	BuiltOn     string        `json:"builtOn"`
	Actions     []rawAction   `json:"actions"`
	Artifacts   []rawArtifact `json:"artifacts"`
}

type rawAction struct {
	Causes     []rawCause `json:"causes"`
	Parameters []rawParam `json:"parameters"`
}

type rawCause struct {
	ShortDescription string `json:"shortDescription"`
	UserID           string `json:"userId"`
	UserName         string `json:"userName"`
	UpstreamProject  string `json:"upstreamProject"`
	UpstreamBuild    int    `json:"upstreamBuild"`
}

type rawParam struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type rawArtifact struct {
	FileName     string `json:"fileName"`
	RelativePath string `json:"relativePath"`
}

const buildDetailTree = "number,url,result,building,timestamp,duration,displayName,builtOn," +
	"actions[causes[shortDescription,userId,userName,upstreamProject,upstreamBuild]," +
	"parameters[name,value]]"

// GetBuild returns one build by job path and ref (number or permalink keyword).
func (c *apiClient) GetBuild(ctx context.Context, path, ref string) (*Build, error) {
	q := url.Values{"tree": {buildDetailTree}}
	var rb rawBuild
	if err := c.getJSON(ctx, jobPath(path)+"/"+buildRef(ref)+"/api/json", q, &rb); err != nil {
		return nil, err
	}
	b := toBuild(rb)
	return &b, nil
}

// Artifacts returns a build's archived artifacts.
func (c *apiClient) Artifacts(ctx context.Context, path, ref string) ([]Artifact, error) {
	q := url.Values{"tree": {"artifacts[fileName,relativePath]"}}
	var rb rawBuild
	base := jobPath(path) + "/" + buildRef(ref)
	if err := c.getJSON(ctx, base+"/api/json", q, &rb); err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(rb.Artifacts))
	for _, a := range rb.Artifacts {
		out = append(out, Artifact{
			FileName:     a.FileName,
			RelativePath: a.RelativePath,
			URL:          c.baseURL + base + "/artifact/" + a.RelativePath,
		})
	}
	return out, nil
}

// toBuild normalizes a rawBuild into a Build, deriving status and human times.
func toBuild(r rawBuild) Build {
	b := Build{
		Number:      r.Number,
		DisplayName: r.DisplayName,
		URL:         r.URL,
		Result:      r.Result,
		Building:    r.Building,
		BuiltOn:     r.BuiltOn,
		TimestampMs: r.Timestamp,
		DurationMs:  r.Duration,
	}
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
	for _, a := range r.Actions {
		for _, cause := range a.Causes {
			b.Causes = append(b.Causes, Cause{
				ShortDescription: cause.ShortDescription,
				UserID:           cause.UserID,
				UserName:         cause.UserName,
				UpstreamProject:  cause.UpstreamProject,
				UpstreamBuild:    cause.UpstreamBuild,
			})
		}
		for _, p := range a.Parameters {
			b.Parameters = append(b.Parameters, KV{Name: p.Name, Value: p.Value})
		}
	}
	return b
}

// durationHuman renders a millisecond duration compactly (e.g. 1m30s). Sub-
// second durations keep millisecond precision; longer ones round to the second.
func durationHuman(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d >= time.Second {
		d = d.Round(time.Second)
	}
	return d.String()
}
