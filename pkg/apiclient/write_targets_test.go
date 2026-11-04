package apiclient

import (
	"context"
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
	ce := cerrors.AsCLIError(c.StopBuild(context.Background(), "app", "7"))
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
