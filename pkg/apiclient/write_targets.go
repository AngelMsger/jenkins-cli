package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

// A write changes remote state, so a failure after the request was sent can
// still have applied it: a timed-out trigger may have queued the build. The
// family's write-outcome rule (cerrors.FromWrite) keeps such a failure from
// reading as retryable: only a definite refusal passes through; anything else
// says the outcome is unknown and points at the read below, so an agent
// reconciles before it triggers a second build. Each target names what the
// write changes and a bounded, read-only command that shows it.

// postWrite sends a write and classifies its failure with the write rule. The
// caller must close the returned body.
func (c *apiClient) postWrite(ctx context.Context, path string, form url.Values, target cerrors.WriteTarget) (*http.Response, error) {
	resp, err := c.postForm(ctx, path, form)
	if err != nil {
		return nil, cerrors.FromWrite(err, target)
	}
	return resp, nil
}

// triggerWriteTarget is for a new build: it has no number until it leaves the
// queue, so the reads are the queue and the job's newest builds.
func triggerWriteTarget(path string) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "build of job " + path,
		ReadSteps: []string{
			"jenkins-cli queue list",
			"jenkins-cli build list " + cerrors.ShellArg(path) + " --limit 5",
		},
	}
}

func buildWriteTarget(path, ref string) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "build " + ref + " of job " + path,
		ReadSteps:   []string{"jenkins-cli build get " + cerrors.ShellArg(path) + " " + cerrors.ShellArg(ref)},
	}
}

func queueWriteTarget(id int) cerrors.WriteTarget {
	return cerrors.WriteTarget{
		Description: "queue item " + strconv.Itoa(id),
		ReadSteps:   []string{"jenkins-cli queue get " + strconv.Itoa(id)},
	}
}
