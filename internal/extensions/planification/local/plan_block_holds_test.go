// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REQUEST BLOCKED BY HAND IS NO AGENT'S WORK: the
// block verb holds the request's agent to-dos - each waits naming the block's
// note, offered to no agent - and resuming it (Plan) offers them again.
func TestARequestBlockedByHandOffersNoAgentItsWork(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	plans := plansOf(t, ctx)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing"})
	td := stepOf(t, ctx, r.ID, api.StepIntake)
	if td.State != schedapi.StateReady {
		t.Fatalf("the intake once filed: %s, want ready", td.State)
	}
	takeable := func() bool {
		t.Helper()
		q, err := schedOf(t, ctx).Queue(ctx, schedapi.Filter{Request: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range q.Queue {
			if e.Todo.ID == td.ID && e.Takeable {
				return true
			}
		}
		return false
	}

	if _, err := plans.Block(ctx, api.BlockInput{ID: r.ID, Note: "out of this increment", By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, r.ID, api.StepIntake); got.State != schedapi.StateWaiting || !strings.Contains(got.Waits, "out of this increment") {
		t.Fatalf("the intake of a blocked request: %s, waits %q; want waiting on the block's note", got.State, got.Waits)
	}
	if takeable() {
		t.Fatal("the intake of a request blocked by hand is offered to agents")
	}
	// The one reader puts the block on the person who lifts it: their act
	// « Débloquer », never an unclassified wait on the system.
	b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), r.ID)
	if b.Kind != api.BlockerAct || b.Act != api.ActUnblock || b.ActorKind != api.ActorPerson || !strings.Contains(b.Reason, "out of this increment") {
		t.Errorf("the blocker of a request blocked by hand: %s %s on %s (%q); want the person's unblock act naming the block's note", b.Kind, b.Act, b.ActorKind, b.Reason)
	}

	// Other work settles the workspace meanwhile: the block still holds.
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	pullAndReport(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "intake done"})
	if takeable() {
		t.Fatal("a settle offered the blocked request's intake again")
	}

	// Resumed, it is offered again.
	if _, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, r.ID, api.StepIntake); got.State != schedapi.StateReady || got.Waits != "" {
		t.Fatalf("the intake once resumed: %s, waits %q; want ready", got.State, got.Waits)
	}
	if !takeable() {
		t.Fatal("the resumed request's intake is not offered to agents")
	}
}
