// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PERSON REMOVES A WAIT NO RULE ENDS: a step
// blocked on work nobody works on - a K6 of its increment's Blockers tab - is
// back in the queue once the person removes its wait, and the tab no longer
// lists it; a wait taken from a depends-on edge drops that edge.
func TestAPersonRemovesAWaitOnWorkNobodyWorksOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in := increment(t, ctx, "Invoices are printed")
	nobody := want(t, ctx, cwm, "feature", "Number the invoices")
	f := want(t, ctx, cwm, "feature", "Print the numbers")
	accepted(t, ctx, cwm, f)
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	intake := stepOf(t, ctx, f, api.StepIntake).ID
	reportBlocked(t, ctx, intake, "it needs the numbering", &schedapi.HeldOn{Kind: schedapi.HeldObject, Target: nobody, Reason: "the numbering first"})

	g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident)
	if g == nil || !slices.ContainsFunc(g.Blockers, func(b api.IncrementBlocker) bool { return b.Todo == intake && b.Cause == api.CauseNobodyWorks }) {
		t.Fatalf("the K6 before: %+v; want %s waiting on %s nobody works on", g, intake, nobody)
	}
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: intake, By: session}); err == nil {
		t.Error("an agent removed the wait")
	}
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	if _, err := plansOf(t, ctx).Unwait(agent, api.UnwaitInput{Todo: intake, By: person}); err == nil {
		t.Error("an agent's session removed the wait under the person's name")
	}
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: intake, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, f, api.StepIntake); got.State != schedapi.StateReady || got.Held != nil {
		t.Fatalf("the step once its wait is removed: %s (waits %q, held %+v); want ready, holding nothing", got.State, got.Waits, got.Held)
	}
	if g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident); g != nil && slices.ContainsFunc(g.Blockers, func(b api.IncrementBlocker) bool { return b.Todo == intake }) {
		t.Errorf("the Blockers tab still lists %s as K6: %+v", intake, g)
	}
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: intake, By: person}); err == nil {
		t.Error("a to-do with no wait had its wait removed")
	}
}

func TestRemovingADependencyWaitDropsItsEdge(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing", Kind: api.KindBuild})
	b := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Depend(ctx, api.DependInput{ID: a.ID, On: b.ID}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, a.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	td := stepOf(t, ctx, a.ID, "analyse")
	if td.State != schedapi.StateWaiting {
		t.Fatalf("the step of a request depending on another: %s (%q); want waiting", td.State, td.Waits)
	}
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: td.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, a.ID, "analyse"); got.State != schedapi.StateReady {
		t.Errorf("the step once its wait is removed: %s (%q); want ready", got.State, got.Waits)
	}
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range o.Object.Edges {
		if e.Kind == api.EdgeDependsOn && e.To == b.ID {
			t.Errorf("%s still depends on %s", a.ID, b.ID)
		}
	}
}

// A blocked report that names no kind - an unclassified K6 - is the person's
// to remove too: its stop is cleared and its step back in the queue.
func TestAPersonRemovesAnUnclassifiedBlock(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in := increment(t, ctx, "Invoices are archived")
	f := want(t, ctx, cwm, "feature", "Archive the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	intake := stepOf(t, ctx, f, api.StepIntake).ID
	reportBlocked(t, ctx, intake, "something is off", nil)
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: intake, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, f, api.StepIntake); got.State != schedapi.StateReady || got.Waits != "" || got.Result != nil {
		t.Fatalf("the unclassified block once removed: %s (waits %q, result %+v); want ready", got.State, got.Waits, got.Result)
	}
	if g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident); g != nil && slices.ContainsFunc(g.Blockers, func(b api.IncrementBlocker) bool { return b.Todo == intake }) {
		t.Errorf("the Blockers tab still lists %s as K6: %+v", intake, g)
	}
}

// A wait on its increment's dependency is refused while the dependency
// holds - with a message naming the increment's act - and cleared once it
// no longer holds (the stale wait the reconciler reports wait-not-cleared).
func TestRemovingAnIncrementWaitNeedsItsDependencyGone(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	create := func(title string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{title}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	b, c := create("B first"), create("C after B")
	if _, err := req.Depend(ctx, api.DependInput{ID: c, On: b}); err != nil {
		t.Fatal(err)
	}
	fix := heldFix(t, ctx, cwm, req, c, "A fix C holds")
	if fix.State != schedapi.StateWaiting || fix.Waits != "its increment "+c+" waits on "+b {
		t.Fatalf("the fix C holds: %s (%q); want it waiting on %s", fix.State, fix.Waits, b)
	}
	_, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: fix.ID, By: person})
	if err == nil || !strings.Contains(err.Error(), "its increment "+c+" depends on "+b) || strings.Contains(err.Error(), "review") {
		t.Fatalf("removing a live increment wait: %v; want a refusal naming the increment's dependency", err)
	}
	if _, err := cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: c, To: b, Kind: api.EdgeDependsOn}); err != nil {
		t.Fatal(err)
	}
	s := schedOf(t, ctx)
	cur, err := s.Get(ctx, schedapi.GetInput{ID: fix.ID})
	if err != nil {
		t.Fatal(err)
	}
	stale := cur.Todo
	stale.State, stale.Waits = schedapi.StateWaiting, fix.Waits
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: stale}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: fix.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if td := stepOf(t, ctx, fix.Request, fix.Packet.Step); td.State != schedapi.StateReady || td.Waits != "" {
		t.Errorf("the stale increment wait once removed: %s (%q); want ready", td.State, td.Waits)
	}
}

// A WAIT NO STRUCTURED CASE OWNS: a waiting to-do
// with no blocked report - after a step gone, a line no reader classifies, a
// wait on an increment nobody can read - is listed K6 with « Remove the
// wait », and the person's Unwait clears it: the step is back in the queue
// and the tab no longer lists it.
func TestAPersonRemovesAWaitNoCaseOwns(t *testing.T) {
	t.Parallel()
	for _, waits := range []string{"after ops.TD-gone", "a wait no reader knows", "its increment billing.IN-999 waits on billing.IN-998"} {
		t.Run(waits, func(t *testing.T) {
			ctx, cwm, req := world(t)
			in := increment(t, ctx, "Invoices are sent")
			f := want(t, ctx, cwm, "feature", "Send the invoices")
			accepted(t, ctx, cwm, f)
			if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
				t.Fatal(err)
			}
			if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
				t.Fatal(err)
			}
			s := schedOf(t, ctx)
			cur, err := s.Get(ctx, schedapi.GetInput{ID: stepOf(t, ctx, f, api.StepIntake).ID})
			if err != nil {
				t.Fatal(err)
			}
			td := cur.Todo
			td.State, td.Waits = schedapi.StateWaiting, waits
			if _, err := s.Put(ctx, schedapi.PutInput{Todo: td}); err != nil {
				t.Fatal(err)
			}
			g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident)
			if g == nil || !slices.ContainsFunc(g.Blockers, func(b api.IncrementBlocker) bool { return b.Todo == td.ID && b.Unwait }) {
				t.Fatalf("the K6 before: %+v; want %s offered « Remove the wait »", g, td.ID)
			}
			if _, err := plansOf(t, ctx).Unwait(ctx, api.UnwaitInput{Todo: td.ID, By: person}); err != nil {
				t.Fatal(err)
			}
			if got := stepOf(t, ctx, f, api.StepIntake); got.State != schedapi.StateReady || got.Waits != "" {
				t.Fatalf("the step once its wait is removed: %s (%q); want ready", got.State, got.Waits)
			}
			if g := groupOf(blockersOf(t, ctx, in.ID), api.BlockerIncident); g != nil && slices.ContainsFunc(g.Blockers, func(b api.IncrementBlocker) bool { return b.Todo == td.ID }) {
				t.Errorf("the Blockers tab still lists %s as K6: %+v", td.ID, g)
			}
		})
	}
}
