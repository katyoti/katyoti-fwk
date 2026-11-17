// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE WAITS THAT ARE NOT EDGES, SWEPT WHOLE - a cycle of
// stored waits (blocked reports holding each other's object) is named whole,
// once, from its root, with the person to break it - in the census and in a
// scope it leaves; the reconciler raises it once; a wait on nothing that
// exists, and a wait on work outside the release, each say who acts.

// The two requests' blocked reports hold each other: a cycle no write refused.
func TestACycleOfStoredWaitsIsNamedWholeOnceFromItsRoot(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	b := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing"})
	c := file(t, ctx, req, api.FileInput{Title: "Mail the numbered invoices", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, a.ID, api.StepIntake).ID, "needs the numbers printed",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: b.ID, Reason: "the printed numbers"})
	reportBlocked(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, "needs the numbering",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: a.ID, Reason: "the numbering"})
	// c is held by the cycle without being in it.
	reportBlocked(t, ctx, stepOf(t, ctx, c.ID, api.StepIntake).ID, "needs the printed numbers",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: b.ID, Reason: "the printed numbers"})

	root, other := a.ID, b.ID
	if other < root {
		root, other = other, root
	}
	want := "a cycle: " + root + " → " + other + " → " + root
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	for _, m := range []string{a.ID, b.ID} {
		got := blockerOf(t, out, m)
		wantKind(t, got, api.BlockerIncident, api.ActorPerson, "break one of its waits")
		if got.Reason != want || got.Cycle != root {
			t.Errorf("%s in the cycle: %q, cycle %q; want %q named from its root %s", m, got.Reason, got.Cycle, want, root)
		}
	}
	held := blockerOf(t, out, c.ID)
	if held.Kind != api.BlockerIncident || held.Cycle != root || !strings.Contains(held.Reason, "held by "+want) || held.ActorKind != api.ActorPerson {
		t.Errorf("work held by the cycle: %+v; want an incident held by %q, the person acting", held, want)
	}

	// The reconciler raises the cycle once: one prepared decision on its
	// root, nothing on the other member nor on what it holds.
	sweep := reconcile(t, ctx, api.ReconcileInput{})
	var on []string
	for _, r := range sweep.Repairs {
		if r.Repair == api.RepairDecideWait {
			on = append(on, r.Work)
		}
	}
	for _, i := range sweep.Incidents {
		on = append(on, i.Check+":"+i.Work)
	}
	if len(on) != 1 || on[0] != root {
		t.Errorf("the sweep raises %v on the cycle; want one decision on its root %s", on, root)
	}
}

// A cycle that leaves the increment through work outside it is still a cycle
// in the increment's check, never a wait outside.
func TestACycleLeavingTheIncrementIsNamedInItsCheck(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, "needs the totals",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: other.ID, Reason: "the rounded totals"})
	reportBlocked(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, "needs the mail",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: f, Reason: "the mailed invoices"})

	got := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Increment: in.ID}), f)
	wantKind(t, got, api.BlockerIncident, api.ActorPerson, "break one of its waits")
	if !strings.HasPrefix(got.Reason, "a cycle: ") || !strings.Contains(got.Reason, other.ID) || got.On != other.ID {
		t.Errorf("the increment's work in a cycle through %s: %q on %q; want the cycle named", other.ID, got.Reason, got.On)
	}
}

// A blocked report holding an object that does not exist: nothing will end
// it - an incident the person clears.
func TestAWaitOnNothingThatExistsIsAnIncident(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, a.ID, api.StepIntake).ID, "needs the ledger",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: "billing.RQ-999", Reason: "the ledger"})
	got := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), a.ID)
	wantKind(t, got, api.BlockerIncident, api.ActorPerson, "remove the wait")
	if !strings.Contains(got.Reason, "billing.RQ-999 cannot be read") {
		t.Errorf("a wait on nothing: %q; want it named unreadable", got.Reason)
	}
}

// The release's work waiting on work it does not ship: outside, naming where
// that work is scheduled.
func TestAWaitOutsideTheReleaseNamesWhereItIs(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, "needs the totals",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: other.ID, Reason: "the rounded totals"})
	got := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Release: r.ID}), f)
	wantKind(t, got, api.BlockerOutside, api.ActorOutside, other.ID+", in no increment or release")
	if got.On != other.ID {
		t.Errorf("the outside wait is on %q; want %s", got.On, other.ID)
	}
}

// A wait on an increment is followed to its first open work; when that work
// waits back, through other work, on the increment, the walk comes back to
// where it began: a cycle named, never a walk without end.
func TestAWaitBackOnAnIncrementIsACycle(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	reportBlocked(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, "needs the totals",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: other.ID, Reason: "the rounded totals"})
	reportBlocked(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, "needs the mailing increment",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: in.ID, Reason: "the mailed invoices"})

	got := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), other.ID)
	wantKind(t, got, api.BlockerIncident, api.ActorPerson, "break one of its waits")
	if !strings.HasPrefix(got.Reason, "a cycle: ") || !strings.Contains(got.Reason, in.ID) {
		t.Errorf("work waiting back on the increment it holds: %q; want a cycle through %s", got.Reason, in.ID)
	}
}
