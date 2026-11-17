// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// A check runner's report restates its summary (go-checks writes the summary
// as the report's first paragraph, and as its whole body when the change
// touched no Go file). CWM refuses an object whose body repeats one of its
// fields, so filing keeps each datum once: the summary in its field, the body
// without it — the review is filed, whatever the runner wrote. A finding's
// evidence that restates its statement is filed the same way.
func TestReviewBodyNeverRepeatsItsSummary(t *testing.T) {
	w := setup(t)
	summary := "Build, vet, tests, package layout, dead code and lint: 0 critical, 1 high, 0 medium, 0 low."
	statement := "go vet reports an unreachable return in the release step"
	cases := []struct {
		name, report string
		keep         string // what the body still holds
	}{
		{"the summary is the whole body", "# Go checks\n\n" + summary + "\n", "# Go checks"},
		{"the summary opens the body", "# Go checks\n\n" + summary + "\n\n## Findings\n\n- vet: a.go:3\n", "## Findings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := w.svc.File(w.ctx, api.FileInput{Object: "m", Type: "go-checks", Trigger: api.TriggerAutomatic,
				RequestedBy: "m.RQ-001", Result: api.ReviewResult{Rating: "needs-attention", Summary: summary, Report: tc.report,
					Findings: []api.ResultFinding{{Severity: "high", Statement: statement, Location: "a.go:3", Key: "vet",
						Evidence: "vet: " + statement + "\n\nfrom `go vet ./...`"}}}})
			if err != nil {
				t.Fatalf("a go-checks review whose report restates its summary is filed: %v", err)
			}
			rv, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: out.Review})
			if err != nil {
				t.Fatal(err)
			}
			if rv.Object.Fields["summary"] != summary {
				t.Errorf("the summary lives in its field: %v", rv.Object.Fields["summary"])
			}
			if strings.Contains(rv.Object.Body, summary) || !strings.Contains(rv.Object.Body, tc.keep) {
				t.Errorf("the body keeps what the summary does not say, never the summary: %q", rv.Object.Body)
			}
			for _, id := range append(out.New, out.Persisting...) {
				f, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(f.Object.Body, statement) || !strings.Contains(f.Object.Body, "go vet ./...") {
					t.Errorf("the finding's evidence keeps what its statement does not say: %q", f.Object.Body)
				}
			}
		})
	}
}
