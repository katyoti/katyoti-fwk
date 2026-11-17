// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RELEASE'S TO-DOS CARRY ITS LABEL AND THEIR RANK — the work on a feature an
// increment holds is labelled with the increment and the release it ships in,
// ranked 1 on the critical path; moving the increment relabels it; direct work
// carries its release; a to-do rewritten keeps its labels; the work on a
// feature under a goal the increment holds carries them too.
func TestAReleasesTodosCarryItsLabelsAndRank(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	labels := func(id, step string) []string { return stepOf(t, ctx, id, step).Labels }
	for _, l := range []string{"increment:" + in.ID, "release:" + a.ID} {
		if !slices.Contains(labels(f, api.StepIntake), l) {
			t.Errorf("the feature's intake: labels %v; want %s", labels(f, api.StepIntake), l)
		}
	}
	if r := stepOf(t, ctx, f, api.StepIntake).Rank; r != 1 {
		t.Errorf("the only work of the release: rank %d; want 1 (the critical path)", r)
	}
	// A feature under a goal the increment holds: its work is the increment's.
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	child := cwmapi.Object{Type: "feature"}
	child.Parent = g
	child.Name = "Remind late payers"
	_ = child.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: child})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: made.Object.ID, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	if l := labels(made.Object.ID, api.StepIntake); !slices.Contains(l, "increment:"+in.ID) || !slices.Contains(l, "release:"+a.ID) {
		t.Errorf("a feature under the increment's goal: labels %v; want increment:%s and release:%s", l, in.ID, a.ID)
	}
	// Not ready: the increment moves to the next release, its work with it.
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: b.ID}); err != nil {
		t.Fatal(err)
	}
	if l := labels(f, api.StepIntake); !slices.Contains(l, "release:"+b.ID) || slices.Contains(l, "release:"+a.ID) {
		t.Errorf("after the increment moved: labels %v; want release:%s only", l, b.ID)
	}
	// Direct work carries the release it targets.
	fix := file(t, ctx, req, api.FileInput{Title: "Fix the label", Module: "billing", Kind: api.KindFix, By: person})
	if _, err := rel.Target(ctx, api.TargetInput{ID: fix.ID, Release: a.ID}); err != nil {
		t.Fatal(err)
	}
	if l := labels(fix.ID, api.StepIntake); !slices.Contains(l, "release:"+a.ID) {
		t.Errorf("direct work: labels %v; want release:%s", l, a.ID)
	}
	// Rewritten (a new priority), it keeps them.
	if _, err := req.Assess(ctx, api.AssessInput{ID: fix.ID, Priority: "P0", By: intake}); err != nil {
		t.Fatal(err)
	}
	if l := labels(fix.ID, api.StepIntake); !slices.Contains(l, "release:"+a.ID) {
		t.Errorf("after a new priority: labels %v; want them kept", l)
	}
}

// A REQUEST THAT JOINS AN INCREMENT IS LABELLED WITH IT — one that still
// targets a release (rescoped from a release's acceptance) and is made to serve
// a feature an increment holds carries the increment, its open to-dos
// relabelled; a follow-up of the increment's work (a request an agent files
// during its run, serving nothing itself) carries it from its first to-do.
func TestARequestThatJoinsAnIncrementIsLabelledWithIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	other := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.9.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	// Filed for another release's acceptance, then made to serve the feature.
	acc := file(t, ctx, req, api.FileInput{Title: "The acceptance, end to end", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := rel.Target(ctx, api.TargetInput{ID: acc.ID, Release: other.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: acc.ID, Serves: f, By: intake}); err != nil {
		t.Fatal(err)
	}
	if l := stepOf(t, ctx, acc.ID, api.StepIntake).Labels; !slices.Contains(l, "increment:"+in.ID) {
		t.Errorf("made to serve the increment's feature: labels %v; want increment:%s at once", l, in.ID)
	}
	if l := stepOf(t, ctx, acc.ID, api.StepIntake).Labels; !slices.Contains(l, "release:"+a.ID) {
		t.Errorf("the request joining the increment: labels %v; want the increment's release %s", l, a.ID)
	}

	// A follow-up of the increment's work, serving nothing itself.
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, session, schedapi.Result{Summary: "a build",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "The screenshot is blank", "details": "found on the way", "kind": "fix"}}}})
	fus, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: f, Kind: cwmapi.EdgeType(api.EdgeFollowsUp), Direction: "in"})
	if err != nil || len(fus.Edges) != 1 {
		t.Fatalf("the follow-up of %s: %+v %v", f, fus.Edges, err)
	}
	fu := fus.Edges[0].From
	if l := stepOf(t, ctx, fu, api.StepIntake).Labels; !slices.Contains(l, "increment:"+in.ID) {
		t.Errorf("the follow-up's first to-do: labels %v; want increment:%s from the start", l, in.ID)
	}
}

// A REQUEST SERVING SEVERAL WANTS TAKES THE INCREMENT OF ANY OF THEM — the
// first it serves is held by none, the second by an increment: its work is
// that increment's, its release the increment's, not its own old target.
func TestARequestServingSeveralWantsTakesTheIncrementOfAny(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	old := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.9.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	loose := want(t, ctx, cwm, "feature", "Print the invoices")
	held := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: held, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	acc := file(t, ctx, req, api.FileInput{Title: "The acceptance, end to end", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := rel.Target(ctx, api.TargetInput{ID: acc.ID, Release: old.ID}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{loose, held} {
		if _, err := req.Assess(ctx, api.AssessInput{ID: acc.ID, Serves: w, By: intake}); err != nil {
			t.Fatal(err)
		}
	}
	l := stepOf(t, ctx, acc.ID, api.StepIntake).Labels
	if !slices.Contains(l, "increment:"+in.ID) || !slices.Contains(l, "release:"+a.ID) {
		t.Errorf("serving %s then %s (held by %s): labels %v; want increment:%s and release:%s", loose, held, in.ID, l, in.ID, a.ID)
	}
}

// A RELEASE TAKES THE WORK OF THE INCREMENTS ITS INCREMENTS DEPEND ON — an
// increment of another release that one of its increments waits on labels its
// work with both releases, so a loop on the waiting release sees it
func TestAReleaseTakesTheWorkOfTheIncrementsItsIncrementsDependOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	c := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.6.0"})
	first, second, third := increment(t, ctx, "Invoices are mailed"), increment(t, ctx, "Reminders are mailed"), increment(t, ctx, "Dunning letters are mailed")
	for _, s := range []struct{ id, in string }{{first.ID, a.ID}, {second.ID, b.ID}, {third.ID, c.ID}} {
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: s.id, In: s.in}); err != nil {
			t.Fatal(err)
		}
	}
	// third waits on second, which waits on first.
	for _, e := range [][2]string{{second.ID, first.ID}, {third.ID, second.ID}} {
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: e[0], To: e[1], Kind: cwmapi.EdgeType(api.EdgeDependsOn)}); err != nil {
			t.Fatal(err)
		}
	}
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	l := stepOf(t, ctx, f, api.StepIntake).Labels
	for _, want := range []string{"increment:" + first.ID, "release:" + a.ID, "release:" + b.ID, "release:" + c.ID} {
		if !slices.Contains(l, want) {
			t.Errorf("the work of %s, which %s and then %s wait on: labels %v; want %s", first.ID, second.ID, third.ID, l, want)
		}
	}
}
