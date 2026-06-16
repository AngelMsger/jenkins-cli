package apiclient

import (
	"net/url"
	"strconv"
	"strings"
)

// jobPath converts a human job path (folder/job[/branch]) into the Jenkins URL
// path (/job/folder/job/job/job/branch). Each slash-separated segment becomes
// one /job/<name> hop, so folders and multibranch branch jobs nest naturally.
// Each segment is URL-escaped; a branch name that itself contains a slash must
// be percent-encoded by the caller.
func jobPath(path string) string {
	var b strings.Builder
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if seg == "" {
			continue
		}
		b.WriteString("/job/")
		b.WriteString(url.PathEscape(seg))
	}
	return b.String()
}

// buildRef maps a friendly build reference to the Jenkins permalink path
// segment. A plain number passes through; the keywords map to Jenkins'
// permalink names. An empty ref means the latest build.
func buildRef(ref string) string {
	switch strings.ToLower(strings.TrimSpace(ref)) {
	case "", "last", "lastbuild":
		return "lastBuild"
	case "lastsuccessful", "lastsuccess", "lastsuccessfulbuild":
		return "lastSuccessfulBuild"
	case "lastfailed", "lastfailure", "lastfailedbuild":
		return "lastFailedBuild"
	case "lastcompleted", "lastcompletedbuild":
		return "lastCompletedBuild"
	case "laststable", "laststablebuild":
		return "lastStableBuild"
	case "lastunstable", "lastunstablebuild":
		return "lastUnstableBuild"
	default:
		if n, err := strconv.Atoi(strings.TrimSpace(ref)); err == nil {
			return strconv.Itoa(n)
		}
		// Unknown keyword: pass through and let Jenkins 404 with guidance.
		return ref
	}
}

// pathDisplay normalizes a human path for display (trims surrounding slashes).
func pathDisplay(path string) string { return strings.Trim(path, "/") }
