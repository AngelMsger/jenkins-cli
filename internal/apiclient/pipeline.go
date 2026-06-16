package apiclient

import (
	"context"
	"time"

	"github.com/angelmsger/jenkins-cli/internal/timeutil"
)

type rawPipeline struct {
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	StartTimeMillis int64      `json:"startTimeMillis"`
	DurationMillis  int64      `json:"durationMillis"`
	Stages          []rawStage `json:"stages"`
}

type rawStage struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	StartTimeMillis int64  `json:"startTimeMillis"`
	DurationMillis  int64  `json:"durationMillis"`
}

// Stages returns a Pipeline build's per-stage status (Jenkins wfapi/describe).
// The endpoint is Pipeline-only; a non-Pipeline job yields a 404, surfaced as a
// structured not-found error by the caller.
func (c *apiClient) Stages(ctx context.Context, path, ref string) (*PipelineRun, error) {
	var rp rawPipeline
	if err := c.getJSON(ctx, jobPath(path)+"/"+buildRef(ref)+"/wfapi/describe", nil, &rp); err != nil {
		return nil, err
	}
	run := &PipelineRun{
		Name:       rp.Name,
		Status:     rp.Status,
		DurationMs: rp.DurationMillis,
	}
	if rp.DurationMillis > 0 {
		run.Duration = durationHuman(rp.DurationMillis)
	}
	if rp.StartTimeMillis > 0 {
		run.StartedAt = timeutil.FromMillis(rp.StartTimeMillis).Format(time.RFC3339)
	}
	for _, s := range rp.Stages {
		stage := Stage{Name: s.Name, Status: s.Status, DurationMs: s.DurationMillis}
		if s.DurationMillis > 0 {
			stage.Duration = durationHuman(s.DurationMillis)
		}
		if s.StartTimeMillis > 0 {
			stage.StartedAt = timeutil.FromMillis(s.StartTimeMillis).Format(time.RFC3339)
		}
		run.Stages = append(run.Stages, stage)
	}
	return run, nil
}
