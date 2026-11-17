// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT'S PRIORITY AND DEPENDENCIES REACH THE WORK IT HOLDS — three
// increments, A P0, B P1 and C P0 depending on B, each holding a feature with
// its build: each build's to-dos are pulled at the higher of the build's and
// its increment's priority; A's and B's work is ready at once, in parallel;
// C's build waits until B is done or dropped, then it is pulled - its intake
// and its preparation (the analyse) run at once.
func TestAnIncrementsPriorityAndDependenciesReachTheWorkItHolds(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	type held struct {
		inc   api.Increment
		build string
	}
	make := func(title, priority string) held {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{title}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := req.Assess(ctx, api.AssessInput{ID: out.Increment.ID, Priority: priority, By: session}); err != nil {
			t.Fatal(err)
		}
		f := want(t, ctx, cwm, "feature", "Feature of "+title)
		accepted(t, ctx, cwm, f)
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: out.Increment.ID}); err != nil {
			t.Fatal(err)
		}
		b := file(t, ctx, req, api.FileInput{On: f, Kind: api.KindBuild, Priority: "P3", By: person})
		pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
		return held{inc: out.Increment, build: b.ID}
	}
	a, b, c := make("A first", "P0"), make("B second", "P1"), make("C after B", "P0")
	if _, err := req.Depend(ctx, api.DependInput{ID: c.inc.ID, On: b.inc.ID}); err != nil {
		t.Fatal(err)
	}
	next := func(h held) api.WorkTodo {
		t.Helper()
		got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: h.build})
		if err != nil {
			t.Fatal(err)
		}
		for _, td := range got.Todos {
			if td.Packet.Phase != api.PhaseRequests && (td.State == schedapi.StateReady || td.State == schedapi.StateWaiting) {
				return td
			}
		}
		t.Fatalf("%s has no step after its intake: %+v", h.build, got.Todos)
		return api.WorkTodo{}
	}
	for _, x := range []struct {
		h        held
		priority string
		ready    bool
	}{{a, "P0", true}, {b, "P1", true}, {c, "P0", true}} {
		td := next(x.h)
		if td.Priority != x.priority {
			t.Errorf("%s's %s: priority %s; want %s, its increment's (the build asked P3)", x.h.build, td.Packet.Step, td.Priority, x.priority)
		}
		if got := td.State == schedapi.StateReady; got != x.ready {
			t.Errorf("%s's %s: %s (%s); want ready %v", x.h.build, td.Packet.Step, td.State, td.Waits, x.ready)
		}
	}
	// C's build step waits on B.
	fix := heldFix(t, ctx, cwm, req, c.inc.ID, "A fix C holds")
	if fix.State != schedapi.StateWaiting || !strings.Contains(fix.Waits, b.inc.ID) {
		t.Errorf("the implement of a fix C holds: %s (%s); want it waiting on %s", fix.State, fix.Waits, b.inc.ID)
	}
	// B dropped: C's work is no longer held.
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: b.inc.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: c.inc.ID, On: b.inc.ID}); err != nil {
		t.Fatal(err)
	}
	if td := stepOf(t, ctx, fix.Request, fix.Packet.Step); td.State != schedapi.StateReady {
		t.Errorf("C's work once B is dropped: %s (%s); want ready", td.State, td.Waits)
	}
}

// AN INCREMENT'S PRIORITY IS SET WHATEVER ITS PLAN — one with no request part
// (made before proposals) and one whose proposal has ended both take a
// priority, and the work they hold is pulled at it; one is ordered after the
// other; a kind is refused on such an increment.
func TestAnIncrementsPriorityIsSetWhateverItsPlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	plain := increment(t, ctx, "Made before proposals")
	ended, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Its proposal ended", Goals: []string{"A goal"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	runIncrement(t, ctx, ended.Increment.ID)
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: ended.Increment.ID, Outcome: api.PlanDone, By: person}); err != nil {
		t.Fatal(err)
	}
	hold := func(incID, title string) string {
		t.Helper()
		// Held from the start: added to a running increment it would be a
		// proposal.
		f := want(t, ctx, cwm, "feature", title)
		taken(t, ctx, cwm, incID, f)
		accepted(t, ctx, cwm, f)
		b := file(t, ctx, req, api.FileInput{On: f, Kind: api.KindBuild, Priority: "P3", By: person})
		pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
		return b.ID
	}
	fp, fe := hold(plain.ID, "Old work"), hold(ended.Increment.ID, "Late work")
	for id, p := range map[string]string{plain.ID: "P0", ended.Increment.ID: "P1"} {
		if _, err := req.Assess(ctx, api.AssessInput{ID: id, Priority: p, By: session}); err != nil {
			t.Fatalf("assess %s at %s: %v", id, p, err)
		}
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: plain.ID, Kind: api.KindBuild, By: session}); err == nil {
		t.Error("a kind on an increment with no live request: accepted; want it refused")
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: ended.Increment.ID, On: plain.ID}); err != nil {
		t.Fatalf("ordering an increment after one with no request part: %v", err)
	}
	next := func(build string) api.WorkTodo {
		t.Helper()
		got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: build})
		if err != nil {
			t.Fatal(err)
		}
		for _, td := range got.Todos {
			if td.Packet.Phase != api.PhaseRequests {
				return td
			}
		}
		t.Fatalf("%s has no step after its intake", build)
		return api.WorkTodo{}
	}
	if td := next(fp); td.Priority != "P0" {
		t.Errorf("the work of the increment with no request part: %s; want P0", td.Priority)
	}
	if td := next(fe); td.Priority != "P1" || td.State != schedapi.StateReady {
		t.Errorf("the preparation of the increment whose proposal ended: %s, %s (%s); want P1, ready (a dependency holds only the build)", td.Priority, td.State, td.Waits)
	}
	if td := heldFix(t, ctx, cwm, req, ended.Increment.ID, "A late fix"); td.Priority != "P1" || td.State != schedapi.StateWaiting || !strings.Contains(td.Waits, plain.ID) {
		t.Errorf("the build of the increment whose proposal ended: %s, %s (%s); want P1, waiting on %s", td.Priority, td.State, td.Waits, plain.ID)
	}
}

// heldFix files a fix an increment holds (direct work targeting it), its
// intake done, and answers its implement - its first build step.
func heldFix(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, req api.RequestService, inc, title string) api.WorkTodo {
	t.Helper()
	r := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindFix, Priority: "P3", By: person})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: r.ID, To: inc, Kind: api.EdgeTargets}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	return stepOf(t, ctx, r.ID, "implement")
}
