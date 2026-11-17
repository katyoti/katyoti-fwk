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

// APPROVING A BUILD'S REVIEW VERIFIES WHAT ONLY A PERSON CAN CHECK — an
// agent's auto-test runs before the review when deliverables no check verifies
// were never given to one (rule R-002), only the review waiting on it; what it
// could not test is verified by the person's approval; a deliverable marked for
// a person's hand test is tried by that auto-test too and
// kept by hand, is its own test, never the approval's. The request then ends.
func TestReviewVerifiesWhatOnlyAPersonCanCheck(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Deliverables: []api.Deliverable{
		{Title: "it builds", Check: "go version"}, {Title: "looks right"}, {Title: "reads well", ByHand: "Print one; read it."}}})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	at := stepOf(t, ctx, r.ID, api.StepAutoTest)
	if titles := toStrings(at.Packet.Params["deliverables"]); !slices.Equal(titles, []string{"looks right", "reads well"}) {
		t.Fatalf("the auto-test before the review is given %v; want the untested deliverable and the one marked by hand", titles)
	}
	if mu := stepOf(t, ctx, r.ID, "module-update"); slices.Contains(mu.After, at.ID) {
		t.Error("the module update waits on the auto-test; only the review does")
	}
	review := stepOf(t, ctx, r.ID, "review")
	if !slices.Contains(review.After, at.ID) || review.Packet.Verifies != api.VerifiesUnchecked {
		t.Fatalf("the review: after %v, verifies %q; want it after the auto-test, verifying the unchecked", review.After, review.Packet.Verifies)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module updated"})
	// The agent could not test it: nothing recorded, the person judges it.
	pullAndReport(t, ctx, at.ID, session, schedapi.Result{Summary: "no check can see it"})
	if _, err := req.Test(ctx, api.TestInput{ID: r.ID, Deliverable: "reads well", Result: api.TestPasses, Note: "read it", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, r.ID, "review").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if d := deliverableOf(t, ctx, req, r.ID, "looks right"); !d.Verified || d.VerifiedBy == nil || d.VerifiedBy.ID != person.ID {
		t.Fatalf("after the approval: %+v; want verified by the person who approved", d)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("approved: %s; want done", st)
	}
	if steps := stepsNamed(t, ctx, req, r.ID, api.StepAutoTest); steps != 1 {
		t.Errorf("%d auto-tests; want the one before the review, none at the end", steps)
	}
}

// stepsNamed counts the steps of a request recorded under a name.
func stepsNamed(t *testing.T, ctx context.Context, req api.RequestService, id, name string) int {
	t.Helper()
	g, err := req.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range g.Request.Plan.Steps {
		if s.Step == name || strings.HasSuffix(s.Step, "/"+name) {
			n++
		}
	}
	return n
}
