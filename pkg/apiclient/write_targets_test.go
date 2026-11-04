package apiclient

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// writeHandler serves the crumb issuer and answers every write with status.
func writeHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"c"}`))
			return
		}
		w.WriteHeader(status)
	}
}

// A write that fails after it was sent may have been applied (a timed-out
// trigger may have queued the build), so its error must never read as
// retryable: it says the outcome is unknown and names the read that shows it.
func TestWriteFailuresAreUnknownOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		write  func(Client) error
		target cerrors.WriteTarget
	}{
		{"trigger", func(c Client) error {
			_, err := c.TriggerBuild(context.Background(), "team/app", map[string]string{"BRANCH": "main"})
			return err
		}, triggerWriteTarget("team/app")},
		{"stop", func(c Client) error {
			return c.StopBuild(context.Background(), "team/app", "7")
		}, buildWriteTarget("team/app", "7")},
		{"cancel", func(c Client) error {
			return c.CancelQueueItem(context.Background(), 77)
		}, queueWriteTarget(77)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, writeHandler(http.StatusServiceUnavailable))
			ce := cerrors.AsCLIError(tc.write(c))
			if ce.Category != cerrors.CategoryServer || ce.HTTPStatus != 503 || ce.Retryable {
				t.Fatalf("got category %s, status %d, retryable %v; want a non-retryable server 503",
					ce.Category, ce.HTTPStatus, ce.Retryable)
			}
			if !strings.HasPrefix(ce.Message, "Write outcome is unknown for "+tc.target.Description) {
				t.Fatalf("message %q does not name the write target", ce.Message)
			}
			if len(ce.NextSteps) != len(tc.target.ReadSteps) || ce.NextSteps[0] != tc.target.ReadSteps[0] {
				t.Fatalf("next steps %q, want the target's reads %q", ce.NextSteps, tc.target.ReadSteps)
			}
		})
	}
}

// A definite refusal shows nothing was written, so it keeps its own code and
// guidance.
func TestWriteRefusalPassesThrough(t *testing.T) {
	c, _ := newTestClient(t, writeHandler(http.StatusNotFound))
	err := c.StopBuild(context.Background(), "app", "7")
	ce := cerrors.AsCLIError(err)
	if ce.Category != cerrors.CategoryNotFound || strings.Contains(ce.Message, "Write outcome") {
		t.Fatalf("got %s %q; want the 404 unchanged", ce.Category, ce.Message)
	}
}

// Jenkins answered the trigger, so the build is queued even when the response
// names no queue item: the error says the write happened, not to send it again.
// Before, the CLI reported success with an empty queue ref.
func TestTriggerWithoutQueueLocation(t *testing.T) {
	for name, location := range map[string]string{"missing": "", "not a queue item": "http://x/job/app/"} {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && location != "" {
					w.Header().Set("Location", location)
				}
				w.WriteHeader(http.StatusCreated)
			})
			_, err := c.TriggerBuild(context.Background(), "app", nil)
			ce := cerrors.AsCLIError(err)
			if ce.Code != cerrors.CodeWriteSucceededResponseInvalid || ce.Retryable ||
				!strings.HasPrefix(ce.Message, "Write succeeded for build of job app") {
				t.Fatalf("got %s retryable=%v %q; want %s", ce.Code, ce.Retryable, ce.Message,
					cerrors.CodeWriteSucceededResponseInvalid)
			}
		})
	}
}

// Only a 2xx acknowledges a write. A redirect the HTTP client does not follow
// used to read as success.
func TestWriteRedirectIsNotSuccess(t *testing.T) {
	c, _ := newTestClient(t, writeHandler(http.StatusFound))
	err := c.CancelQueueItem(context.Background(), 77)
	if ce := cerrors.AsCLIError(err); err == nil || ce.Retryable || !strings.HasPrefix(ce.Message, "Write outcome is unknown") {
		t.Fatalf("got %v; want an unknown write outcome", err)
	}
}

func TestTriggerReturnsTheQueueItem(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Location", "http://x/queue/item/42/")
		}
		w.WriteHeader(http.StatusCreated)
	})
	ref, err := c.TriggerBuild(context.Background(), "app", nil)
	if err != nil || ref.QueueID != 42 {
		t.Fatalf("got %+v, %v; want queue item 42", ref, err)
	}
}

// A permalink names whichever build is newest when it is read. Stopping
// "last" used to send /lastBuild/stop and name "last" in the read after an
// uncertain stop, which by then could show a newer build. The ref is resolved
// to its number first, so the stop and the read name the same build.
func TestStopResolvesAPermalink(t *testing.T) {
	stops := map[string]func(Client) (int, error){
		"StopBuildNumber": func(c Client) (int, error) {
			return StopBuildNumber(context.Background(), c, "app", "last")
		},
		// A library caller of StopBuild gets the same build stopped.
		"StopBuild": func(c Client) (int, error) {
			return 12, c.StopBuild(context.Background(), "app", "last")
		},
	}
	for stopName, stop := range stops {
		for name, status := range map[string]int{"stopped": http.StatusOK, "uncertain": http.StatusServiceUnavailable} {
			t.Run(stopName+" "+name, func(t *testing.T) {
				var posts []string
				c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost:
						posts = append(posts, r.URL.Path)
						w.WriteHeader(status)
					case r.URL.Path == "/job/app/lastBuild/api/json":
						w.Write([]byte(`{"number":12}`))
					default:
						w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"c"}`))
					}
				})
				n, err := stop(c)
				if len(posts) != 1 || posts[0] != "/job/app/12/stop" {
					t.Fatalf("stop requests %q; want /job/app/12/stop", posts)
				}
				if status == http.StatusOK {
					if err != nil || n != 12 {
						t.Fatalf("got %d, %v; want build 12 stopped", n, err)
					}
					return
				}
				ce := cerrors.AsCLIError(err)
				if ce == nil || !strings.HasPrefix(ce.Message, "Write outcome is unknown for build 12 of job app") ||
					len(ce.NextSteps) == 0 || ce.NextSteps[0] != "jenkins-cli build get 'app' '12'" {
					t.Fatalf("got %+v; want an unknown outcome naming build 12", ce)
				}
			})
		}
	}
}

// foreignClient is a Client this package did not build, such as a library
// caller's fake. Only the methods StopBuildNumber may call are implemented.
type foreignClient struct {
	Client
	build   *Build
	stopErr error
	stopped []string
}

func (f *foreignClient) GetBuild(_ context.Context, path, ref string) (*Build, error) {
	return f.build, nil
}

func (f *foreignClient) StopBuild(_ context.Context, path, ref string) error {
	f.stopped = append(f.stopped, path+"#"+ref)
	return f.stopErr
}

// StopBuildNumber works with any Client: it reads the build a permalink names
// with GetBuild, then stops it by number. A number needs no read.
func TestStopBuildNumberWithAnotherClient(t *testing.T) {
	f := &foreignClient{build: &Build{Number: 12}}
	if n, err := StopBuildNumber(context.Background(), f, "app", "last"); err != nil || n != 12 ||
		len(f.stopped) != 1 || f.stopped[0] != "app#12" {
		t.Fatalf("got %d, %v, stops %q; want build 12 stopped by number", n, err, f.stopped)
	}
	f = &foreignClient{}
	if n, err := StopBuildNumber(context.Background(), f, "app", "7"); err != nil || n != 7 || f.stopped[0] != "app#7" {
		t.Fatalf("got %d, %v, stops %q; want build 7 stopped without a read", n, err, f.stopped)
	}
	f = &foreignClient{build: &Build{}}
	_, err := StopBuildNumber(context.Background(), f, "app", "lastFailed")
	if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "BUILD_NOT_FOUND" || len(f.stopped) != 0 {
		t.Fatalf("got %+v, stops %q; want BUILD_NOT_FOUND and no stop", ce, f.stopped)
	}
}

// A foreign Client's failed stop may still have stopped the build, so it is
// reported as this package's clients report one: an unknown outcome, not
// retryable, with the read of the stopped build. Before, its retryable 503
// came back as it was, inviting a second stop. A definite refusal, such as a
// read-only wrapper's, still comes back unchanged.
func TestStopBuildNumberClassifiesAnotherClientsStop(t *testing.T) {
	unavailable := cerrors.New(cerrors.CategoryServer, "HTTP_503", "service unavailable").WithHTTPStatus(503)
	unavailable.Retryable = true
	for name, stopErr := range map[string]error{
		"classified": unavailable,
		"raw":        errors.New("connection reset by peer"),
	} {
		t.Run(name, func(t *testing.T) {
			f := &foreignClient{build: &Build{Number: 12}, stopErr: stopErr}
			n, err := StopBuildNumber(context.Background(), f, "app", "last")
			ce := cerrors.AsCLIError(err)
			if n != 12 || ce == nil || ce.Retryable ||
				!strings.HasPrefix(ce.Message, "Write outcome is unknown for build 12 of job app") ||
				len(ce.NextSteps) == 0 || ce.NextSteps[0] != "jenkins-cli build get 'app' '12'" {
				t.Fatalf("got %d, %+v; want an unknown outcome naming build 12", n, ce)
			}
		})
	}
	refused := cerrors.New(cerrors.CategoryPermission, "READONLY_BLOCKED", "read-only")
	f := &foreignClient{stopErr: refused}
	if _, err := StopBuildNumber(context.Background(), f, "app", "7"); err != refused {
		t.Fatalf("got %v; want the refusal unchanged", err)
	}
}

// A decorator around one of this package's clients is a foreign Client too.
// Its stop already reports the write outcome, so StopBuildNumber's own
// classification leaves it as it is instead of nesting it in a second one.
func TestStopBuildNumberThroughADecorator(t *testing.T) {
	inner, _ := newTestClient(t, writeHandler(http.StatusServiceUnavailable))
	decorated := struct{ Client }{inner}
	_, err := StopBuildNumber(context.Background(), decorated, "app", "12")
	ce := cerrors.AsCLIError(err)
	if ce == nil || ce.Retryable ||
		!strings.HasPrefix(ce.Message, "Write outcome is unknown for build 12 of job app") ||
		strings.Count(ce.Message, "Write outcome is unknown") != 1 {
		t.Fatalf("got %+v; want one unknown outcome naming build 12", ce)
	}
}

// Only a queue item's URL names the queued build. A Location such as a build
// or job page that ends in a number used to be read as a queue id.
func TestParseQueueID(t *testing.T) {
	for loc, want := range map[string]int{
		"http://x/queue/item/42/":         42,
		"http://x/queue/item/42":          42,
		"http://x/jenkins/queue/item/42/": 42,
		"http://x/job/app/42/":            0,
		"http://x/queue/item/abc/":        0,
		"":                                0,
	} {
		if got := parseQueueID(loc); got != want {
			t.Errorf("parseQueueID(%q) = %d, want %d", loc, got, want)
		}
	}
}

// Jenkins never answers a trigger with a redirect, so one (here to a login
// page) leaves the outcome unknown, with the redirect's status. The transport
// returns it instead of following it: following turned the POST into a GET,
// and the login page it landed on read as "accepted, but no queue item".
func TestRedirectedTriggerIsAnUnknownOutcome(t *testing.T) {
	var landed int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			http.Redirect(w, r, "/login", http.StatusFound)
		case r.URL.Path == "/login":
			landed++
			w.Write([]byte("<html>sign in</html>"))
		default:
			w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"c"}`))
		}
	})
	_, err := c.TriggerBuild(context.Background(), "app", nil)
	ce := cerrors.AsCLIError(err)
	if ce == nil || ce.Retryable || ce.HTTPStatus != http.StatusFound || landed != 0 ||
		!strings.HasPrefix(ce.Message, "Write outcome is unknown for build of job app") ||
		!strings.Contains(ce.Hint, "answered it with a redirect") {
		t.Fatalf("got %+v after %d login page reads; want an unknown outcome with the 302", ce, landed)
	}
}

// Jenkins acknowledges a stop or a cancel with a redirect to "." of the
// request: the build page or the queue page. That redirect, under any host
// the servlet container names, is the write's success; so is a 2xx.
func TestStopAndCancelAcceptJenkinsAcknowledgment(t *testing.T) {
	cases := []struct {
		name, location string
		status         int
		write          func(Client) error
	}{
		{"stop, build page", "/job/app/7/", http.StatusFound, stopBuild7},
		{"stop, build page on the container's own host", "http://jenkins.internal:8080/job/app/7/", http.StatusFound, stopBuild7},
		{"stop, see other", "/job/app/7/", http.StatusSeeOther, stopBuild7},
		{"stop, 200", "", http.StatusOK, stopBuild7},
		{"cancel, queue page", "/queue/", http.StatusFound, cancelItem77},
		{"cancel, 204", "", http.StatusNoContent, cancelItem77},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, acknowledgingHandler(t, tc.status, tc.location))
			if err := tc.write(c); err != nil {
				t.Fatalf("got %v; want the acknowledgment accepted", err)
			}
		})
	}
}

// Any other redirect of a stop or a cancel says nothing about the write: a
// login page, a single sign-on proxy, an https upgrade (which keeps the
// request's own path), or a method-preserving 307 that would send the write
// again.
func TestStopAndCancelOtherRedirectsAreUnknownOutcomes(t *testing.T) {
	cases := []struct {
		name, location string
		status         int
		write          func(Client) error
	}{
		{"stop, login page", "/login?from=%2Fjob%2Fapp%2F7%2Fstop", http.StatusFound, stopBuild7},
		{"stop, single sign-on", "https://sso.example/authorize?rd=x", http.StatusFound, stopBuild7},
		{"stop, https upgrade", "https://jenkins.example/job/app/7/stop", http.StatusMovedPermanently, stopBuild7},
		{"stop, https upgrade as 302", "https://jenkins.example/job/app/7/stop", http.StatusFound, stopBuild7},
		{"stop, 307 to the build page", "/job/app/7/", http.StatusTemporaryRedirect, stopBuild7},
		{"cancel, login page", "/login?from=%2Fqueue%2FcancelItem", http.StatusFound, cancelItem77},
		{"cancel, another job", "/job/app/", http.StatusFound, cancelItem77},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, acknowledgingHandler(t, tc.status, tc.location))
			ce := cerrors.AsCLIError(tc.write(c))
			if ce == nil || ce.Retryable || ce.HTTPStatus != tc.status ||
				!strings.HasPrefix(ce.Message, "Write outcome is unknown for ") {
				t.Fatalf("got %+v; want an unknown outcome with the %d", ce, tc.status)
			}
		})
	}
}

func stopBuild7(c Client) error { return c.StopBuild(context.Background(), "app", "7") }

func cancelItem77(c Client) error { return c.CancelQueueItem(context.Background(), 77) }

// acknowledgingHandler answers every write with status and, when set, a
// Location; it fails the test if the client follows the redirect.
func acknowledgingHandler(t *testing.T, status int, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			if location != "" {
				w.Header().Set("Location", location)
			}
			w.WriteHeader(status)
		case r.URL.Path == "/crumbIssuer/api/json":
			w.Write([]byte(`{"crumbRequestField":"Jenkins-Crumb","crumb":"c"}`))
		default:
			t.Errorf("the client followed the redirect to %s", r.URL)
			w.WriteHeader(http.StatusOK)
		}
	}
}
