// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REJECTED REVIEW'S REWORK KEEPS THE TEMPLATE'S ORDER:
// the work done again comes first - here the step added after the
// implementation, the one the review sat right after - and what checks or
// records it, the auto-review and the module update, waits on it: it never
// reviews the unfixed code.
func TestAReworkDoesTheWorkBeforeItsChecks(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "implemented",
		More: map[string]any{"add": []map[string]any{{"step": "gui-test", "task": "coder", "instructions": "Walk the invoice screen."}}}})
	for _, step := range []string{"gui-test", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "review").ID, session, schedapi.Result{Summary: "the rounding is wrong", Verdict: api.VerdictRejected})

	work := []string{stepOf(t, ctx, r.ID, "gui-test (rework)").ID}
	for _, check := range []string{"auto-review (rework)", "module-update (rework)"} {
		c := stepOf(t, ctx, r.ID, check)
		for _, w := range work {
			if !slices.Contains(c.After, w) {
				t.Errorf("%s comes after %v; want after every step done again (%v)", check, c.After, work)
			}
		}
	}
	for _, w := range []string{"gui-test (rework)"} {
		if s := stepOf(t, ctx, r.ID, w); slices.Contains(s.After, stepOf(t, ctx, r.ID, "auto-review (rework)").ID) {
			t.Errorf("%s waits on the auto-review done again", w)
		}
	}
	// The review done again comes last: after the work and its checks.
	review := stepOf(t, ctx, r.ID, "review")
	for _, s := range []string{"gui-test (rework)", "auto-review (rework)", "module-update (rework)"} {
		if !slices.Contains(review.After, stepOf(t, ctx, r.ID, s).ID) {
			t.Errorf("the review done again comes after %v; want after %s", review.After, s)
		}
	}
}
