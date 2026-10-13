package apiclient

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/angelmsger/jenkins-cli/pkg/timeutil"
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
	// The Git plugin's BuildData action: one per repository the build
	// checked out.
	LastBuiltRevision *rawRevision `json:"lastBuiltRevision"`
	RemoteURLs        []string     `json:"remoteUrls"`
}

type rawRevision struct {
	SHA1   string `json:"SHA1"`
	Branch []struct {
		Name string `json:"name"`
	} `json:"branch"`
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

// buildDetailTree leaves out BuildData's buildsByBranchName, which grows with
// every branch a job has ever built.
const buildDetailTree = "number,url,result,building,timestamp,duration,displayName,builtOn," +
	"actions[causes[shortDescription,userId,userName,upstreamProject,upstreamBuild]," +
	"parameters[name,value],lastBuiltRevision[SHA1,branch[name]],remoteUrls]"

// jobRemotesTree reads the Git remotes a job declares: a freestyle job's SCM,
// or the source of a multibranch branch job.
const jobRemotesTree = "scm[userRemoteConfigs[url]],property[branch[scm[userRemoteConfigs[url]]]]"

// GetBuild returns one build by job path and ref (number or permalink keyword).
func (c *apiClient) GetBuild(ctx context.Context, path, ref string) (*Build, error) {
	q := url.Values{"tree": {buildDetailTree}}
	var rb rawBuild
	if err := c.getJSON(ctx, jobPath(path)+"/"+buildRef(ref)+"/api/json", q, &rb); err != nil {
		return nil, err
	}
	b := toBuild(rb)
	b.Revision = c.buildRevision(ctx, path, gitCheckouts(rb))
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

// gitCheckouts returns the build's Git checkouts (BuildData actions with a
// revision) in the order the Git plugin recorded them.
func gitCheckouts(r rawBuild) []rawAction {
	var out []rawAction
	for _, a := range r.Actions {
		if a.LastBuiltRevision != nil && a.LastBuiltRevision.SHA1 != "" {
			out = append(out, a)
		}
	}
	return out
}

// buildRevision picks the commit the job itself built. A build with one Git
// checkout has its answer. Pipeline shared libraries and extra checkout steps
// add more, so with several the job's own remote decides, and the last
// checkout of it wins, as in the Git plugin. Only freestyle and multibranch
// jobs show their remote; for any other job, or when nothing matches, the
// revision is left out rather than guessed, because a shared library is
// often checked out before the job's own repository.
func (c *apiClient) buildRevision(ctx context.Context, path string, checkouts []rawAction) *Revision {
	switch len(checkouts) {
	case 0:
		return nil
	case 1:
		return toRevision(checkouts[0], "")
	}
	own := c.jobRemotes(ctx, path)
	for i := len(checkouts) - 1; i >= 0; i-- {
		for _, remote := range checkouts[i].RemoteURLs {
			if own[normalizeRemote(remote)] {
				return toRevision(checkouts[i], remote)
			}
		}
	}
	return nil
}

// jobRemotes returns the normalized Git remotes a job declares. A job that
// cannot be read declares none: the build itself was already read.
func (c *apiClient) jobRemotes(ctx context.Context, path string) map[string]bool {
	type rawSCM struct {
		UserRemoteConfigs []struct {
			URL string `json:"url"`
		} `json:"userRemoteConfigs"`
	}
	var rj struct {
		SCM      *rawSCM `json:"scm"`
		Property []struct {
			Branch *struct {
				SCM *rawSCM `json:"scm"`
			} `json:"branch"`
		} `json:"property"`
	}
	if err := c.getJSON(ctx, jobPath(path)+"/api/json", url.Values{"tree": {jobRemotesTree}}, &rj); err != nil {
		return nil
	}
	scms := []*rawSCM{rj.SCM}
	for _, p := range rj.Property {
		if p.Branch != nil {
			scms = append(scms, p.Branch.SCM)
		}
	}
	own := map[string]bool{}
	for _, scm := range scms {
		if scm == nil {
			continue
		}
		for _, rc := range scm.UserRemoteConfigs {
			own[normalizeRemote(rc.URL)] = true
		}
	}
	return own
}

// normalizeRemote drops the trailing slashes and ".git" that make two
// spellings of one repository differ, as the Git plugin does.
func normalizeRemote(u string) string {
	return strings.TrimSuffix(strings.TrimRight(u, "/"), ".git")
}

// toRevision reports a checkout's commit, its first branch (the one the Git
// plugin puts in GIT_BRANCH) and the remote it came from.
func toRevision(a rawAction, remote string) *Revision {
	r := &Revision{SHA: a.LastBuiltRevision.SHA1, Remote: remote}
	if len(a.LastBuiltRevision.Branch) > 0 {
		r.Branch = a.LastBuiltRevision.Branch[0].Name
	}
	if r.Remote == "" && len(a.RemoteURLs) == 1 {
		r.Remote = a.RemoteURLs[0]
	}
	return r
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
