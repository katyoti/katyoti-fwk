// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A PERSON CONFIRMING A FEATURE DONE ENDS ITS PLAN BLOCKED BY THE REFUSED END
// : a realized feature's build ends with its to-dos
// done, but it cannot become done as an agent (confirm is a person's), so
// its plan stands blocked and « À faire » asks the person to
// settle it. The person confirms it done in its Lifecycle tab: the plan ends,
// and « À faire » no longer asks for it.
func TestAPersonConfirmingAFeatureDoneEndsItsBlockedPlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if err := move(ctx, cwm, f, "realized", person); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	// Its build ends as on the hub when the feature's verification gate does
	// not pass: its to-dos done, the move to done refused (confirm is a
	// person's), the plan blocked by the refused end with that note.
	refused := "its to-dos are done, but " + f + " cannot become done: cwm: transition refused: confirm: the guard \"by.kind == 'person'\" does not hold"
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: f, Note: refused, By: session}); err != nil {
		t.Fatal(err)
	}
	setBlockKind(t, ctx, cwm, f, api.BlockMoveRefused)
	if st := planState(t, ctx, f); st != api.PlanBlocked {
		t.Fatalf("the case needs the build's plan blocked by its refused end; it is %s", st)
	}
	asked := func() bool {
		for _, it := range personQueue(t, ctx, "").Items {
			if it.Action == api.ActionUnblock && it.Request == f {
				return true
			}
		}
		return false
	}
	if !asked() {
		t.Fatalf("the case needs « À faire » to ask the person to unblock %s", f)
	}

	// The person confirms it done, in its Lifecycle tab: the transition runs
	// planification's on_update hook, as on the hub.
	if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	// Its transitioned event ends the plan (EndConfirmed), shortly after.
	for i := 0; i < 50 && planState(t, ctx, f) != api.PlanDone; i++ {
		time.Sleep(40 * time.Millisecond)
	}
	if st := planState(t, ctx, f); st != api.PlanDone {
		t.Errorf("its plan after the person confirmed it done: %s; want done", st)
	}
	if asked() {
		t.Errorf("« À faire » still asks the person to unblock %s, confirmed done: %+v", f, personQueue(t, ctx, "").Items)
	}
}
