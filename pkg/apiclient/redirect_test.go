package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/angelmsger/agent-cli-kit/doctor"
	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
	"github.com/angelmsger/jenkins-cli/pkg/transport"
)

// A read that loops through redirects ends at the tenth hop, which is
// reported with the kit's redirect rule: its own code (HTTP_FOUND), not
// retryable, a hint naming --base-url and doctor first. 0.13.0 reported
// NETWORK (exit 8, retryable). doctor reports it as redirected.
func TestReadRedirectLoopIsReportedNotRetried(t *testing.T) {
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, Transport: transport.New(transport.Options{Timeout: 5 * time.Second, MaxRetries: 2})})
	err := c.Ping(context.Background())
	ce := cerrors.AsCLIError(err)
	if ce == nil || ce.Code != "HTTP_FOUND" || ce.HTTPStatus != http.StatusFound || ce.Retryable ||
		!strings.Contains(ce.Hint, "check --base-url first") ||
		len(ce.NextSteps) == 0 || ce.NextSteps[0] != "jenkins-cli doctor" {
		t.Fatalf("got %+v; want HTTP_FOUND 302, not retryable", ce)
	}
	if got := sent.Load(); got != 10 {
		t.Fatalf("requests = %d, want 10 (no retry)", got)
	}
	if got := doctor.Status(err); got != doctor.StatusRedirected {
		t.Fatalf("doctor.Status = %q, want redirected", got)
	}
	if got := doctor.Detail(err); !strings.Contains(got, "Jenkins returned HTTP 302. The server redirected the request to ") {
		t.Fatalf("doctor.Detail = %q", got)
	}
}

// A console read that gets a 3xx back (a 304, or the tenth hop of a loop) is
// an error, not an empty log. 0.13.0 took any status below 400 as the log.
func TestConsoleRedirectIsAnError(t *testing.T) {
	for _, status := range []int{http.StatusNotModified, http.StatusFound} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if status == http.StatusFound {
				http.Redirect(w, r, r.URL.Path, status)
				return
			}
			w.WriteHeader(status)
		})
		chunk, err := c.Console(context.Background(), "fx", "1", 0)
		ce := cerrors.AsCLIError(err)
		if ce == nil || ce.HTTPStatus != status || ce.Retryable || chunk != nil {
			t.Fatalf("%d: got %+v, %+v; want the %d as an error", status, chunk, ce, status)
		}
	}
}
