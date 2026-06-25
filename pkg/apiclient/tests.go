package apiclient

import (
	"context"
	"net/url"
)

type rawTestReport struct {
	FailCount  int `json:"failCount"`
	SkipCount  int `json:"skipCount"`
	PassCount  int `json:"passCount"`
	TotalCount int `json:"totalCount"`
	Suites     []struct {
		Name  string `json:"name"`
		Cases []struct {
			ClassName       string  `json:"className"`
			Name            string  `json:"name"`
			Status          string  `json:"status"`
			Duration        float64 `json:"duration"`
			ErrorDetails    string  `json:"errorDetails"`
			ErrorStackTrace string  `json:"errorStackTrace"`
		} `json:"cases"`
	} `json:"suites"`
}

const testReportTree = "failCount,skipCount,passCount,totalCount," +
	"suites[name,cases[className,name,status,duration,errorDetails,errorStackTrace]]"

// Tests returns a build's test report (Jenkins testReport). Builds without test
// results yield a 404, surfaced as a structured not-found error by the caller.
func (c *apiClient) Tests(ctx context.Context, path, ref string) (*TestReport, error) {
	q := url.Values{"tree": {testReportTree}}
	var rr rawTestReport
	if err := c.getJSON(ctx, jobPath(path)+"/"+buildRef(ref)+"/testReport/api/json", q, &rr); err != nil {
		return nil, err
	}
	report := &TestReport{
		Total:   rr.TotalCount,
		Passed:  rr.PassCount,
		Failed:  rr.FailCount,
		Skipped: rr.SkipCount,
	}
	for _, s := range rr.Suites {
		for _, tc := range s.Cases {
			report.Cases = append(report.Cases, TestCase{
				Suite:        s.Name,
				ClassName:    tc.ClassName,
				Name:         tc.Name,
				Status:       tc.Status,
				DurationSec:  tc.Duration,
				ErrorDetails: tc.ErrorDetails,
				ErrorStack:   tc.ErrorStackTrace,
			})
		}
	}
	return report, nil
}
