// Package apiclient is the Jenkins API surface used by the CLI. It builds
// requests against the Jenkins remote access API (the api/json, consoleText and
// wfapi endpoints), decodes normalized models, and converts non-2xx responses
// into structured *errors.CLIError values.
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	cerrors "github.com/angelmsger/jenkins-cli/internal/errors"
	"github.com/angelmsger/jenkins-cli/internal/transport"
)

// Client is the Jenkins API surface used by the CLI. Read methods cover the
// developer's inspection workflow; the three write methods (TriggerBuild,
// StopBuild, CancelQueueItem) are the only mutating calls and are gated by the
// read-only wrapper.
type Client interface {
	// BaseURL returns the normalized Jenkins server root (no trailing slash).
	BaseURL() string

	// Ping verifies connectivity and credentials against the instance root.
	Ping(ctx context.Context) error
	// WhoAmI returns the authenticated user (Jenkins /me).
	WhoAmI(ctx context.Context) (*User, error)

	// ListJobs returns the jobs under folder ("" = instance root), recursing
	// nested folders / multibranch projects up to depth levels.
	ListJobs(ctx context.Context, folder string, depth int) ([]Job, error)
	// GetJob returns one job by human path (folder/job[/branch]), including its
	// parameters, health, last-build pointers and (for containers) child jobs.
	GetJob(ctx context.Context, path string) (*Job, error)
	// JobBuilds returns a job's recent build history, newest first.
	JobBuilds(ctx context.Context, path string, limit int) ([]Build, error)

	// GetBuild returns one build. ref is a number or a permalink keyword
	// (last, lastSuccessful, lastFailed, lastCompleted, lastStable).
	GetBuild(ctx context.Context, path, ref string) (*Build, error)
	// Console returns a slice of a build's console output starting at byte
	// offset start (0 = from the top).
	Console(ctx context.Context, path, ref string, start int64) (*ConsoleChunk, error)
	// Stages returns a Pipeline build's per-stage status (wfapi/describe).
	Stages(ctx context.Context, path, ref string) (*PipelineRun, error)
	// Tests returns a build's test report.
	Tests(ctx context.Context, path, ref string) (*TestReport, error)
	// Changes returns a build's SCM changeset.
	Changes(ctx context.Context, path, ref string) (*ChangeSet, error)
	// Artifacts returns a build's archived artifacts.
	Artifacts(ctx context.Context, path, ref string) ([]Artifact, error)

	// ListQueue returns the build queue (pending / blocked builds).
	ListQueue(ctx context.Context) ([]QueueItem, error)
	// GetQueueItem returns one queue item by id.
	GetQueueItem(ctx context.Context, id int) (*QueueItem, error)

	// TriggerBuild starts a build of a job, with optional parameters. It returns
	// a pointer to the resulting queue item.
	TriggerBuild(ctx context.Context, path string, params map[string]string) (*QueueRef, error)
	// StopBuild aborts a running build.
	StopBuild(ctx context.Context, path, ref string) error
	// CancelQueueItem removes a still-queued build before it starts.
	CancelQueueItem(ctx context.Context, id int) error
}

// User is the authenticated Jenkins user (subset of /me).
type User struct {
	ID       string `json:"id,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

// apiClient is the single Client implementation.
type apiClient struct {
	baseURL string // server root, no trailing slash
	http    *transport.Client

	crumbOnce sync.Once
	crumbName string
	crumbVal  string
}

// Config configures a Client.
type Config struct {
	BaseURL   string
	Transport *transport.Client
}

// New builds a Client. The transport must already carry the auth decorator.
func New(cfg Config) Client {
	return &apiClient{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		http:    cfg.Transport,
	}
}

func (c *apiClient) BaseURL() string { return c.baseURL }

// getJSON performs a GET and decodes the JSON body into out. path is appended
// to the base URL verbatim; query is optional.
func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return c.httpError(resp, path)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return decodeJSON(raw, out)
}

// getText performs a GET and returns the raw response body plus the response
// headers — used for the plain-text console endpoints.
func (c *apiClient) getText(ctx context.Context, path string, query url.Values) (body []byte, header http.Header, err error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, rerr := http.NewRequest(http.MethodGet, endpoint, nil)
	if rerr != nil {
		return nil, nil, cerrors.Wrap(rerr, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	resp, derr := c.http.Do(ctx, req)
	if derr != nil {
		return nil, nil, cerrors.Wrap(derr, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", endpoint))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, nil, c.httpError(resp, path)
	}
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.Header, nil
}

// postForm performs a POST with an application/x-www-form-urlencoded body,
// attaching a CSRF crumb. It returns the response so callers can read headers
// (e.g. the Location of a queued build). The caller must close the body.
func (c *apiClient) postForm(ctx context.Context, path string, form url.Values) (*http.Response, error) {
	var bodyReader io.Reader
	if len(form) > 0 {
		bodyReader = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if name, val := c.crumb(ctx); name != "" {
		req.Header.Set(name, val)
	}
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK",
			fmt.Sprintf("request to %s failed", c.baseURL+path))
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.httpError(resp, path)
	}
	return resp, nil
}

// crumb fetches (once, then caches) the CSRF crumb required for POST requests.
// A 404 means the instance has CSRF protection disabled — the crumb is then
// empty and POSTs proceed without it.
func (c *apiClient) crumb(ctx context.Context) (name, value string) {
	c.crumbOnce.Do(func() {
		var out struct {
			Field string `json:"crumbRequestField"`
			Crumb string `json:"crumb"`
		}
		if err := c.getJSON(ctx, "/crumbIssuer/api/json", nil, &out); err != nil {
			return // disabled or unreachable; proceed crumbless
		}
		c.crumbName, c.crumbVal = out.Field, out.Crumb
	})
	return c.crumbName, c.crumbVal
}

// decodeJSON unmarshals a server response body into out, surfacing a snippet on
// failure so a shape mismatch is diagnosable.
func decodeJSON(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return cerrors.Wrap(err, cerrors.CategoryParse, "DECODE",
			fmt.Sprintf("could not decode the server response: %v", err)).
			WithHint("The server's JSON did not match what jenkins-cli expected; "+
				"this is likely a client bug, not a failed request.").
			WithNextSteps(
				"Retry with --verbose to inspect the raw response.",
				"Report it with this snippet: "+snippet)
	}
	return nil
}

// httpError turns a non-2xx response into a classified CLIError. reqPath is the
// request path; it is reserved for path-specific guidance.
func (c *apiClient) httpError(resp *http.Response, reqPath string) error {
	_ = reqPath
	cat := cerrors.FromHTTPStatus(resp.StatusCode)
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	msg := fmt.Sprintf("Jenkins returned HTTP %d", resp.StatusCode)
	if detail := firstLine(string(snippet)); detail != "" {
		msg += ": " + detail
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return cerrors.New(cat, "HTTP_UNAUTHORIZED", msg).
			WithHTTPStatus(resp.StatusCode).
			WithHint("Jenkins rejected the credentials. Use your Jenkins username plus an API token "+
				"(User → Configure → API Token), not your web password.").
			WithNextSteps("jenkins-cli auth status", "jenkins-cli config init")
	case http.StatusForbidden:
		return cerrors.New(cat, "HTTP_FORBIDDEN", msg).
			WithHTTPStatus(resp.StatusCode).
			WithHint("Authenticated, but this user lacks permission for the resource, " +
				"or a CSRF crumb was required and missing.").
			WithNextSteps("Confirm the user has Job/Read (and Job/Build for writes) in Jenkins → Manage → Security.")
	case http.StatusNotFound:
		return cerrors.New(cat, "HTTP_NOT_FOUND", msg).
			WithHTTPStatus(resp.StatusCode).
			WithHint("No such job or build at that path. Job paths are folder/job[/branch]; "+
				"list to find the exact name.").
			WithNextSteps("jenkins-cli job list", "jenkins-cli job get <path>")
	default:
		return cerrors.New(cat, "HTTP_"+statusSlug(resp.StatusCode), msg).
			WithHTTPStatus(resp.StatusCode)
	}
}

func statusSlug(status int) string {
	if t := http.StatusText(status); t != "" {
		return strings.ToUpper(strings.ReplaceAll(t, " ", "_"))
	}
	return fmt.Sprintf("%d", status)
}

// firstLine returns the first non-empty trimmed line of s, truncated.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			if len(ln) > 200 {
				ln = ln[:200] + "…"
			}
			return ln
		}
	}
	return ""
}
