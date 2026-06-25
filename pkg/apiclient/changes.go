package apiclient

import (
	"context"
	"net/url"
	"time"

	"github.com/angelmsger/jenkins-cli/pkg/timeutil"
)

type rawChangeItem struct {
	CommitID  string `json:"commitId"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
	Comment   string `json:"comment"`
	Author    struct {
		FullName string `json:"fullName"`
	} `json:"author"`
	AffectedPaths []string `json:"affectedPaths"`
	Paths         []struct {
		File string `json:"file"`
	} `json:"paths"`
}

type rawChangeSetObj struct {
	Kind  string          `json:"kind"`
	Items []rawChangeItem `json:"items"`
}

const changeItemsTree = "kind,items[commitId,timestamp,msg,comment,author[fullName],affectedPaths,paths[file]]"

// Changes returns a build's SCM changeset. Pipeline / multibranch builds expose
// changeSets (plural); freestyle builds expose changeSet (singular). We try the
// plural shape first and fall back to the singular one, so the same command
// works for both job types.
func (c *apiClient) Changes(ctx context.Context, path, ref string) (*ChangeSet, error) {
	base := jobPath(path) + "/" + buildRef(ref) + "/api/json"

	var plural struct {
		ChangeSets []rawChangeSetObj `json:"changeSets"`
	}
	if err := c.getJSON(ctx, base, url.Values{"tree": {"changeSets[" + changeItemsTree + "]"}}, &plural); err == nil {
		return mergeChangeSets(plural.ChangeSets), nil
	}

	var single struct {
		ChangeSet rawChangeSetObj `json:"changeSet"`
	}
	if err := c.getJSON(ctx, base, url.Values{"tree": {"changeSet[" + changeItemsTree + "]"}}, &single); err != nil {
		return nil, err
	}
	return mergeChangeSets([]rawChangeSetObj{single.ChangeSet}), nil
}

func mergeChangeSets(sets []rawChangeSetObj) *ChangeSet {
	out := &ChangeSet{Commits: []Commit{}}
	for _, s := range sets {
		if out.Kind == "" {
			out.Kind = s.Kind
		}
		for _, it := range s.Items {
			out.Commits = append(out.Commits, toCommit(it))
		}
	}
	return out
}

func toCommit(it rawChangeItem) Commit {
	msg := it.Msg
	if msg == "" {
		msg = it.Comment
	}
	paths := it.AffectedPaths
	if len(paths) == 0 {
		for _, p := range it.Paths {
			if p.File != "" {
				paths = append(paths, p.File)
			}
		}
	}
	cm := Commit{
		ID:            it.CommitID,
		Author:        it.Author.FullName,
		Message:       msg,
		AffectedPaths: paths,
	}
	if it.Timestamp > 0 {
		cm.AuthoredAt = timeutil.FromMillis(it.Timestamp).Format(time.RFC3339)
	}
	return cm
}
