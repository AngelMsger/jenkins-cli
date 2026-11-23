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

func TestJenkinsSignInChallengeAndPermissions(t *testing.T) {
	const login = `<html><head><meta http-equiv="refresh" content="1;url=/ci/securityRealm/commenceLogin?from=private-sentinel"></head></html>`
	for _, tc := range []struct {
		name, body, as, code string
		status               int
	}{
		{"login challenge", login, "", "AUTH_LOGIN_REQUIRED", 403},
		{"HTML login on success", login, "", "AUTH_LOGIN_REQUIRED", 200},
		{"anonymous denied", "", "anonymous", "AUTH_LOGIN_REQUIRED", 403},
		{"named user denied", login, "alice", "HTTP_FORBIDDEN", 403},
		{"proxy denied", "<html>Access Denied</html>", "", "HTTP_FORBIDDEN", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html;charset=utf-8")
				w.Header().Set("X-You-Are-Authenticated-As", tc.as)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			err := client.Ping(context.Background())
			ce := cerrors.AsCLIError(err)
			if err == nil || ce.Code != tc.code || ce.HTTPStatus != tc.status || strings.Contains(ce.Message, "private-sentinel") {
				t.Fatalf("unexpected diagnosis: %+v", ce)
			}
			if tc.code == "AUTH_LOGIN_REQUIRED" && !strings.Contains(strings.Join(ce.NextSteps, " "), "auth login --browser") {
				t.Fatalf("missing sign-in recovery: %+v", ce)
			}
		})
	}
}

func TestBuildClientStopsBeforeExternalIdentityProvider(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the CLI contacted the identity provider")
	}))
	defer idp.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, idp.URL+"/authorize?state=private-sentinel", http.StatusFound)
	}))
	defer server.Close()
	client, err := BuildClient(BuildParams{BaseURL: server.URL, AuthDecorator: func(r *http.Request) { r.SetBasicAuth("alice", "fixture-token") }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WhoAmI(context.Background())
	ce := cerrors.AsCLIError(err)
	if err == nil || !ce.IsRedirect() || ce.HTTPStatus != 302 || strings.Contains(ce.Message+ce.Hint, "private-sentinel") {
		t.Fatalf("unexpected redirect: %+v", ce)
	}
}

func TestSignInRoutesAreRelativeToJenkinsRoot(t *testing.T) {
	const root = "/team/ci"
	for _, tc := range []struct{ path, code string }{
		{root + "/login/", "AUTH_LOGIN_REQUIRED"},
		{root + "/securityRealm/commenceLogin", "AUTH_LOGIN_REQUIRED"},
		{"/other/securityRealm/commenceLogin", "DECODE"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == root+"/api/json" {
					http.Redirect(w, r, tc.path, http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html>sign in</html>"))
			}))
			defer server.Close()
			client := New(Config{BaseURL: server.URL + root, Transport: transport.New(transport.Options{})})
			err := client.Ping(context.Background())
			if err == nil || cerrors.AsCLIError(err).Code != tc.code {
				t.Fatalf("redirect to %s: got %v, want %s", tc.path, err, tc.code)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case root + "/crumbIssuer/api/json":
			http.NotFound(w, r)
		case root + "/job/securityRealm/api/json":
			http.Error(w, "permission denied", http.StatusForbidden)
		case root + "/job/securityRealm/7/stop":
			_, _ = w.Write([]byte("<html>stopped</html>"))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := New(Config{BaseURL: server.URL + root, Transport: transport.New(transport.Options{})})
	if _, err := client.GetJob(context.Background(), "securityRealm"); err == nil || cerrors.AsCLIError(err).Code != "HTTP_FORBIDDEN" {
		t.Fatalf("permission check on securityRealm job: got %v, want HTTP_FORBIDDEN", err)
	}
	if err := client.StopBuild(context.Background(), "securityRealm", "7"); err != nil {
		t.Fatalf("stop on securityRealm job: %v", err)
	}
}
