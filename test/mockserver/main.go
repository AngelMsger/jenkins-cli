// Command mockserver is a tiny stand-in for a Jenkins remote API, used by
// scripts/e2e.sh to exercise jenkins-cli end-to-end without a real Jenkins or
// credentials. It serves canned jobs, builds, console output, stages, tests and
// queue responses, and accepts the write POSTs.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	addr := "127.0.0.1:45080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", route)

	fmt.Fprintf(os.Stderr, "mockserver listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func route(w http.ResponseWriter, r *http.Request) {
	if !requireAuth(w, r) {
		return
	}
	path := r.URL.Path
	tree := r.URL.Query().Get("tree")

	// Writes: trigger / stop / cancel.
	if r.Method == http.MethodPost {
		switch {
		case strings.HasSuffix(path, "/build"), strings.HasSuffix(path, "/buildWithParameters"):
			w.Header().Set("Location", "http://"+r.Host+"/queue/item/77/")
			w.WriteHeader(http.StatusCreated)
		case strings.HasSuffix(path, "/stop"), path == "/queue/cancelItem":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unknown POST", http.StatusNotFound)
		}
		return
	}

	switch {
	case path == "/crumbIssuer/api/json":
		writeJSON(w, map[string]any{"crumbRequestField": "Jenkins-Crumb", "crumb": "test-crumb"})
	case path == "/me/api/json":
		writeJSON(w, map[string]any{"id": "alice", "fullName": "Alice Dev"})
	case path == "/api/json":
		if strings.Contains(tree, "jobs[") {
			writeJSON(w, jobList())
		} else {
			writeJSON(w, map[string]any{"mode": "NORMAL"})
		}
	case path == "/queue/api/json":
		writeJSON(w, queue())
	case strings.HasSuffix(path, "/logText/progressiveText"):
		w.Header().Set("X-More-Data", "false")
		body := "[Pipeline] sh\n+ make test\nFAIL: TestThing\nBuild step failed\n"
		w.Header().Set("X-Text-Size", strconv.Itoa(len(body)))
		w.Write([]byte(body))
	case strings.HasSuffix(path, "/wfapi/describe"):
		writeJSON(w, stages())
	case strings.HasSuffix(path, "/testReport/api/json"):
		writeJSON(w, testReport())
	case strings.HasSuffix(path, "/api/json"):
		switch {
		case strings.Contains(tree, "builds["):
			writeJSON(w, map[string]any{"builds": buildHistory()})
		case strings.Contains(tree, "artifacts["):
			writeJSON(w, map[string]any{"artifacts": []map[string]any{{"fileName": "app.jar", "relativePath": "dist/app.jar"}}})
		case strings.Contains(tree, "changeSets["):
			writeJSON(w, map[string]any{"changeSets": changeSets()})
		case strings.Contains(tree, "changeSet["):
			writeJSON(w, map[string]any{"changeSet": changeSets()[0]})
		case isBuildAPIPath(path):
			writeJSON(w, buildDetail())
		default:
			writeJSON(w, jobDetail())
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// isBuildAPIPath reports whether a /…/api/json path addresses a build (the
// segment before api/json is a number or a lastXxxBuild permalink) rather than
// a job.
func isBuildAPIPath(path string) bool {
	trimmed := strings.TrimSuffix(path, "/api/json")
	i := strings.LastIndex(trimmed, "/")
	if i < 0 {
		return false
	}
	seg := trimmed[i+1:]
	if _, err := strconv.Atoi(seg); err == nil {
		return true
	}
	return strings.HasPrefix(seg, "last")
}

func jobList() map[string]any {
	return map[string]any{"jobs": []map[string]any{
		{"_class": "org.jenkinsci.plugins.workflow.job.WorkflowJob", "name": "app",
			"url": "http://x/job/app/", "color": "red",
			"lastBuild": map[string]any{"number": 7, "url": "http://x/job/app/7/", "result": "FAILURE",
				"timestamp": 1700000000000, "duration": 91000, "building": false}},
		// A multibranch branch whose name contains a slash (feature/login) — Jenkins
		// encodes the slash in the job name as %2F.
		{"_class": "org.jenkinsci.plugins.workflow.job.WorkflowJob", "name": "feature%2Flogin",
			"url": "http://x/job/feature%252Flogin/", "color": "blue",
			"lastBuild": map[string]any{"number": 2, "url": "http://x/2/", "result": "SUCCESS",
				"timestamp": 1700000000000, "duration": 30000, "building": false}},
		{"_class": "com.cloudbees.hudson.plugins.folder.Folder", "name": "team", "url": "http://x/job/team/", "color": ""},
	}}
}

func jobDetail() map[string]any {
	return map[string]any{
		"_class": "org.jenkinsci.plugins.workflow.job.WorkflowJob", "name": "app",
		"url": "http://x/job/app/", "color": "red", "buildable": true,
		"description":  "the app pipeline",
		"healthReport": []map[string]any{{"score": 60, "description": "Build stability: 2 of the last 5 builds failed."}},
		"property": []map[string]any{{"parameterDefinitions": []map[string]any{
			{"name": "BRANCH", "type": "StringParameterDefinition", "description": "git branch",
				"defaultParameterValue": map[string]any{"value": "main"}}}}},
		"lastBuild":           map[string]any{"number": 7, "url": "http://x/job/app/7/", "result": "FAILURE"},
		"lastSuccessfulBuild": map[string]any{"number": 5, "url": "http://x/job/app/5/", "result": "SUCCESS"},
		"lastFailedBuild":     map[string]any{"number": 7, "url": "http://x/job/app/7/", "result": "FAILURE"},
	}
}

func buildDetail() map[string]any {
	return map[string]any{
		"number": 7, "url": "http://x/job/app/7/", "result": "FAILURE", "building": false,
		"timestamp": 1700000000000, "duration": 91000, "builtOn": "agent-1", "displayName": "#7",
		"actions": []map[string]any{
			{"causes": []map[string]any{{"shortDescription": "Started by user Alice Dev", "userId": "alice", "userName": "Alice Dev"}}},
			{"parameters": []map[string]any{{"name": "BRANCH", "value": "main"}}},
		},
	}
}

func buildHistory() []map[string]any {
	return []map[string]any{
		{"number": 7, "url": "http://x/job/app/7/", "result": "FAILURE", "building": false, "timestamp": 1700000000000, "duration": 91000},
		{"number": 6, "url": "http://x/job/app/6/", "result": "SUCCESS", "building": false, "timestamp": 1699990000000, "duration": 60000},
	}
}

func stages() map[string]any {
	return map[string]any{
		"name": "#7", "status": "FAILED", "startTimeMillis": 1700000000000, "durationMillis": 91000,
		"stages": []map[string]any{
			{"name": "Checkout", "status": "SUCCESS", "durationMillis": 3000},
			{"name": "Build", "status": "SUCCESS", "durationMillis": 40000},
			{"name": "Test", "status": "FAILED", "durationMillis": 48000},
		},
	}
}

func testReport() map[string]any {
	return map[string]any{
		"failCount": 1, "skipCount": 0, "passCount": 12, "totalCount": 13,
		"suites": []map[string]any{{"name": "unit", "cases": []map[string]any{
			{"className": "pkg.ThingTest", "name": "TestOK", "status": "PASSED", "duration": 0.1},
			{"className": "pkg.ThingTest", "name": "TestThing", "status": "FAILED", "duration": 0.2, "errorDetails": "expected 1 got 2"},
		}}},
	}
}

func changeSets() []map[string]any {
	return []map[string]any{{"kind": "git", "items": []map[string]any{
		{"commitId": "abc123", "timestamp": 1700000000000, "msg": "break the test",
			"author": map[string]any{"fullName": "Bob"}, "affectedPaths": []string{"pkg/thing.go"}},
	}}}
}

func queue() map[string]any {
	return map[string]any{"items": []map[string]any{
		{"id": 51, "why": "Waiting for next available executor", "blocked": true, "buildable": false,
			"inQueueSince": 1700000000000, "task": map[string]any{"name": "app", "url": "http://x/job/app/"}},
	}}
}

func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"missing credentials"}`))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
