package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// buildEndpoint returns the trigger path for a job and whether parameters are
// being passed (which selects buildWithParameters over build).
func buildEndpoint(path string, hasParams bool) string {
	if hasParams {
		return jobPath(path) + "/buildWithParameters"
	}
	return jobPath(path) + "/build"
}

func paramForm(params map[string]string) url.Values {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	return form
}

// TriggerBuild starts a build of a job. With parameters it posts to
// buildWithParameters; without, to build. It returns a pointer to the queue
// item created, parsed from the Location header. Jenkins accepted the build
// once it answers, so a response without a queue item location reports the
// write as done but unidentified, never as something to send again.
func (c *apiClient) TriggerBuild(ctx context.Context, path string, params map[string]string) (*QueueRef, error) {
	target := triggerWriteTarget(path)
	resp, err := c.postWrite(ctx, buildEndpoint(path, len(params) > 0), paramForm(params), target)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Jenkins answers a trigger itself (201 and a Location), never with a
	// redirect. One that was followed turned the POST into a GET, so the
	// response came from that GET (often a login page), and whether the build
	// was queued is unknown.
	if resp.Request != nil && resp.Request.Method != http.MethodPost {
		return nil, cerrors.FromWrite(redirectedTrigger(resp), target)
	}
	ref := &QueueRef{QueueURL: resp.Header.Get("Location")}
	ref.QueueID = parseQueueID(ref.QueueURL)
	if ref.QueueID == 0 {
		return nil, cerrors.MissingWriteIdentity(target)
	}
	return ref, nil
}

// redirectedTrigger is the cause of a trigger whose response came from a
// followed redirect. The same request is redirected the same way again, so it
// is not retryable.
func redirectedTrigger(resp *http.Response) error {
	e := cerrors.Newf(cerrors.CategoryServer, "HTTP_REDIRECTED",
		"Jenkins redirected the POST request to %s", resp.Request.URL.Redacted()).
		WithHint("A redirect after a trigger usually means a login page, a single sign-on proxy, " +
			"or a base URL that should use https. Check --base-url, then run doctor.")
	e.Retryable = false
	return e
}

// StopBuild aborts a running build and returns its number. A permalink such
// as last names whichever build is newest when it is read, so it is resolved
// to a number first: the stop, and the read that reconciles an uncertain
// stop, then name the same build even when a newer one starts in between.
func (c *apiClient) StopBuild(ctx context.Context, path, ref string) (int, error) {
	n, err := c.buildNumber(ctx, path, ref)
	if err != nil {
		return 0, err
	}
	num := strconv.Itoa(n)
	resp, err := c.postWrite(ctx, jobPath(path)+"/"+num+"/stop", nil, buildWriteTarget(path, num))
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return n, nil
}

// buildNumber resolves a build ref to its number. A number is returned as is;
// a permalink is read from Jenkins.
func (c *apiClient) buildNumber(ctx context.Context, path, ref string) (int, error) {
	r := buildRef(ref)
	if n, err := strconv.Atoi(r); err == nil {
		return n, nil
	}
	var rb struct {
		Number int `json:"number"`
	}
	if err := c.getJSON(ctx, jobPath(path)+"/"+r+"/api/json", url.Values{"tree": {"number"}}, &rb); err != nil {
		return 0, err
	}
	if rb.Number <= 0 {
		return 0, cerrors.Newf(cerrors.CategoryNotFound, "BUILD_NOT_FOUND",
			"job %s has no build for %q", pathDisplay(path), ref).
			WithNextSteps("jenkins-cli build list " + cerrors.ShellArg(path) + " --limit 5")
	}
	return rb.Number, nil
}

// CancelQueueItem removes a still-queued build before it starts.
func (c *apiClient) CancelQueueItem(ctx context.Context, id int) error {
	resp, err := c.postWrite(ctx, "/queue/cancelItem", url.Values{"id": {strconv.Itoa(id)}}, queueWriteTarget(id))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// queueItemPath matches a queue item's path, under any context path Jenkins
// is served from ("/jenkins/queue/item/42/").
var queueItemPath = regexp.MustCompile(`/queue/item/([0-9]+)/?$`)

// parseQueueID extracts the id from a queue item URL such as
// "http://host/queue/item/42/". Any other URL, such as a build or job page
// that merely ends in a number, yields 0: it does not name a queue item.
func parseQueueID(loc string) int {
	u, err := url.Parse(loc)
	if err != nil {
		return 0
	}
	m := queueItemPath.FindStringSubmatch(u.Path)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// TriggerBuildPlan returns the WritePlan a TriggerBuild call would execute. It
// sends nothing — it backs --dry-run as a credentials-free preview.
func TriggerBuildPlan(baseURL, path string, params map[string]string) WritePlan {
	p := WritePlan{DryRun: true, Method: "POST", URL: baseURL + buildEndpoint(path, len(params) > 0)}
	if len(params) > 0 {
		p.Body = paramForm(params).Encode()
	}
	return p
}

// StopBuildPlan returns the WritePlan a StopBuild call would execute. It is
// built offline, so a permalink such as last stays in the URL; StopBuild
// resolves it to the build's number when it sends the stop.
func StopBuildPlan(baseURL, path, ref string) WritePlan {
	return WritePlan{DryRun: true, Method: "POST", URL: baseURL + jobPath(path) + "/" + buildRef(ref) + "/stop"}
}

// CancelQueuePlan returns the WritePlan a CancelQueueItem call would execute.
func CancelQueuePlan(baseURL string, id int) WritePlan {
	return WritePlan{DryRun: true, Method: "POST", URL: baseURL + "/queue/cancelItem?id=" + strconv.Itoa(id)}
}
