package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"
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
// item created (parsed from the Location header).
func (c *apiClient) TriggerBuild(ctx context.Context, path string, params map[string]string) (*QueueRef, error) {
	resp, err := c.postForm(ctx, buildEndpoint(path, len(params) > 0), paramForm(params))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	ref := &QueueRef{QueueURL: resp.Header.Get("Location")}
	ref.QueueID = parseQueueID(ref.QueueURL)
	return ref, nil
}

// StopBuild aborts a running build.
func (c *apiClient) StopBuild(ctx context.Context, path, ref string) error {
	resp, err := c.postForm(ctx, jobPath(path)+"/"+buildRef(ref)+"/stop", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// CancelQueueItem removes a still-queued build before it starts.
func (c *apiClient) CancelQueueItem(ctx context.Context, id int) error {
	resp, err := c.postForm(ctx, "/queue/cancelItem", url.Values{"id": {strconv.Itoa(id)}})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// parseQueueID extracts the trailing numeric id from a queue item URL such as
// "http://host/queue/item/42/".
func parseQueueID(loc string) int {
	loc = strings.Trim(loc, "/")
	if i := strings.LastIndex(loc, "/"); i >= 0 {
		loc = loc[i+1:]
	}
	n, _ := strconv.Atoi(loc)
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

// StopBuildPlan returns the WritePlan a StopBuild call would execute.
func StopBuildPlan(baseURL, path, ref string) WritePlan {
	return WritePlan{DryRun: true, Method: "POST", URL: baseURL + jobPath(path) + "/" + buildRef(ref) + "/stop"}
}

// CancelQueuePlan returns the WritePlan a CancelQueueItem call would execute.
func CancelQueuePlan(baseURL string, id int) WritePlan {
	return WritePlan{DryRun: true, Method: "POST", URL: baseURL + "/queue/cancelItem?id=" + strconv.Itoa(id)}
}
