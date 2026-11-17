// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : a goal or a feature added to a running increment is a
// small proposal on it holding only the addition - nothing is aimed until the
// person approves it; approved, it is aimed; sent back, nothing is. Before the
// increment runs, it is aimed at once.
func TestAnAdditionToARunningIncrementIsProposed(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shop")
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid", Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	in := out.Increment
	early := want(t, ctx, cwm, "feature", "Send a first reminder")
	if got, err := inc.Target(ctx, api.IncrementTargetInput{ID: early, Increment: in.ID, By: person}); err != nil || got.Proposed != "" || got.Release != in.ID {
		t.Fatalf("before it runs, the feature added = %+v (%v); want it aimed at once", got, err)
	}
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}

	f := want(t, ctx, cwm, "feature", "Mail the reminders")
	got, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if got.Proposed == "" || got.Release == in.ID {
		t.Fatalf("added to the running increment = %+v; want a proposal, nothing aimed", got)
	}
	g, err := req.Get(ctx, api.GetInput{ID: got.Proposed})
	if err != nil {
		t.Fatal(err)
	}
	if d := g.Request.Delta; len(d) != 1 || d[0].Op != api.OpIncrementTarget || d[0].Object != f || d[0].Increment != in.ID {
		t.Errorf("the proposal's delta %+v; want only the addition", d)
	}
	if g.Request.Requester.ID != person.ID {
		t.Errorf("the proposal's requester %+v; want the person who added it", g.Request.Requester)
	}
	if again, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID, By: person}); err != nil || again.Proposed != got.Proposed {
		t.Errorf("the same addition again = %+v (%v); want the open proposal %s", again, err, got.Proposed)
	}
	if scope, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); holds(scope, f) {
		t.Fatal("the feature is in the increment before the person approved it")
	}
	step := stepOf(t, ctx, got.Proposed, "approve-addition")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: step.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if scope, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); !holds(scope, f) {
		t.Error("approved, the feature is not in the increment")
	}

	// Sent back: nothing aimed.
	h := want(t, ctx, cwm, "feature", "Phone the late payers")
	got, err = inc.Target(ctx, api.IncrementTargetInput{ID: h, Increment: in.ID, By: person})
	if err != nil || got.Proposed == "" {
		t.Fatalf("a second addition = %+v (%v); want a proposal", got, err)
	}
	step = stepOf(t, ctx, got.Proposed, "approve-addition")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: step.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "not now", Details: "next increment", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	if scope, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); holds(scope, h) {
		t.Error("sent back, the feature is in the increment")
	}

	// Filed on a feature it does not hold yet: proposed, nothing joined.
	k := want(t, ctx, cwm, "feature", "Text the late payers")
	filed, err := req.File(ctx, api.FileInput{On: k, Kind: api.KindBuild, Increment: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if filed.Proposed == "" {
		t.Error("work filed on a feature the running increment does not hold joined it at once; want a proposal")
	}
	if scope, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); holds(scope, k) {
		t.Error("the feature filed on is in the increment before the person approved it")
	}

	// Approved after the increment closed: the proposal ends, nothing aimed.
	m := want(t, ctx, cwm, "feature", "Mail a final notice")
	late, err := inc.Target(ctx, api.IncrementTargetInput{ID: m, Increment: in.ID, By: person})
	if err != nil || late.Proposed == "" {
		t.Fatalf("a late addition = %+v (%v); want a proposal", late, err)
	}
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	step = stepOf(t, ctx, late.Proposed, "approve-addition")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: step.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Errorf("approving an addition to a closed increment: %v; want it ended, nothing aimed", err)
	}
	if lg, err := req.Get(ctx, api.GetInput{ID: late.Proposed}); err != nil || lg.Request.State != api.PlanDropped {
		t.Errorf("the late proposal: %s (%v); want dropped", lg.Request.State, err)
	}
}

func holds(scope api.ScopeOutput, id string) bool {
	for _, x := range scope.Targets {
		if x.ID == id {
			return true
		}
	}
	return false
}
