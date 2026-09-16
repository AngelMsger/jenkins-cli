package app

import (
	"strconv"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/jenkins-cli/pkg/errors"
)

func TestMissingBuildDataRecovery(t *testing.T) {
	for _, tc := range []struct {
		code string
		hint func(error, string, string) error
	}{
		{"NOT_PIPELINE", notPipelineHint},
		{"NO_TEST_REPORT", noTestsHint},
	} {
		t.Run(tc.code, func(t *testing.T) {
			path, ref := "team/app's feature%2Flogin", "128"
			source := cerrors.New(cerrors.CategoryNotFound, "HTTP_NOT_FOUND", "not found").WithHTTPStatus(404)
			got := cerrors.AsCLIError(tc.hint(source, path, ref))
			if got.Code != tc.code || got.Category != cerrors.CategoryNotFound || got.HTTPStatus != 404 {
				t.Fatalf("error = %+v", got)
			}
			if len(got.NextSteps) != 2 {
				t.Fatalf("next steps = %v", got.NextSteps)
			}
			for _, step := range got.NextSteps {
				if !strings.Contains(step, strconv.Quote(path)) || !strings.Contains(step, strconv.Quote(ref)) {
					t.Errorf("recovery lost the selected target: %q", step)
				}
			}
			denied := cerrors.New(cerrors.CategoryPermission, "HTTP_FORBIDDEN", "denied")
			if got := tc.hint(denied, path, ref); got != denied {
				t.Errorf("unrelated failure was replaced: %v", got)
			}
		})
	}
}
