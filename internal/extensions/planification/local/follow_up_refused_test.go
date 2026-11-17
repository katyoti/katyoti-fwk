// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REPORT WHOSE FOLLOW-UP CANNOT BE FILED IS REFUSED:
// a follow-up with a kind no request may have, or in a module that does not
// exist, sends the report back - nothing recorded, nothing filed, the to-do
// still its reporter's to report again - never a step recorded with its
// follow-up lost and the report left uncollected.
func TestAReportWhoseFollowUpCannotBeFiledIsRefused(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	impl := stepOf(t, ctx, r.ID, "implement").ID
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: impl, By: session}); err != nil {
		t.Fatal(err)
	}
	for _, fu := range []map[string]any{
		{"title": "Round the credit notes", "module": "billing", "kind": "feature"},
		{"title": "Round the credit notes", "module": "no-such-module", "kind": "fix"},
		{"title": " ", "module": "billing", "kind": "fix"},
	} {
		bad := schedapi.Result{Status: schedapi.StatusSuccess, Summary: "implemented", By: session,
			More: map[string]any{"follow_ups": []map[string]any{fu}}}
		if _, err := s.Report(ctx, schedapi.ReportInput{ID: impl, Result: bad}); err == nil {
			t.Fatalf("a report with the follow-up %v was collected; want it refused", fu)
		}
		if got := stepOf(t, ctx, r.ID, "implement"); got.State != schedapi.StateClaimed || got.Claim == nil || got.Claim.By != session {
			t.Fatalf("refused for the follow-up %v, the implement is %s (%v); want it still claimed by its reporter", fu, got.State, got.Claim)
		}
		if hasRequest(t, ctx, req, "Round the credit notes") {
			t.Fatalf("a refused report filed its follow-up %v", fu)
		}
		if slices.Contains(doneStepNames(t, ctx, req, r.ID), "implement") {
			t.Fatalf("a refused report was recorded on the request")
		}
	}
	// Reported again with a follow-up that can be filed: collected.
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: impl, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "implemented", By: session,
		More: map[string]any{"follow_ups": []map[string]any{{"title": "Round the credit notes", "module": "billing", "kind": "fix"}}}}}); err != nil {
		t.Fatal(err)
	}
	if !hasRequest(t, ctx, req, "Round the credit notes") {
		t.Fatal("the follow-up reported again was not filed")
	}
	if !slices.Contains(doneStepNames(t, ctx, req, r.ID), "implement") {
		t.Fatalf("the implement reported again was not recorded on the request: %v", doneStepNames(t, ctx, req, r.ID))
	}
}

// doneStepNames names the steps recorded on a request, without their recipe.
func doneStepNames(t *testing.T, ctx context.Context, req api.RequestService, id string) []string {
	t.Helper()
	g, err := req.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range g.Request.Plan.Steps {
		_, step, _ := strings.Cut(s.Step, "/")
		if step == "" {
			step = s.Step
		}
		out = append(out, step)
	}
	return out
}
