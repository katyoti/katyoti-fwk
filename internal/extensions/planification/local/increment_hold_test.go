// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT ON HOLD — put aside saying why, its work is not pulled (each
// to-do waits saying so) and it leaves the release it shipped in, which no
// longer waits for it; its hold shows on it. Resumed, its work is pulled
// again. A hold says why; only what is held is resumed.
func TestAnIncrementIsPutOnHoldAndResumed(t *testing.T) {
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
	pullAndReport(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	next := ""
	for _, name := range steps(t, ctx, f) {
		if name != api.StepIntake && stepOf(t, ctx, f, name).State == schedapi.StateReady {
			next = name
			break
		}
	}
	if next == "" {
		t.Fatalf("no step of the feature's build is ready before the hold: %v", steps(t, ctx, f))
	}

	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: in.ID, By: person}); err == nil {
		t.Error("an increment was held without saying why")
	}
	held, err := inc.Hold(ctx, api.IncrementHoldInput{ID: in.ID, Reason: "the mail provider is not chosen", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if held.Increment.Held == nil || held.Increment.Held.Reason != "the mail provider is not chosen" {
		t.Errorf("the increment held: %+v; want its hold shown with its reason", held.Increment)
	}
	if held.Increment.ShipsIn != "" {
		t.Errorf("held, it still ships in %s; want it out of the release", held.Increment.ShipsIn)
	}
	if s, _ := releases(t, ctx).Scope(ctx, api.GetInput{ID: r.ID}); s.Total != 0 {
		t.Errorf("the release still waits for %d target(s) of the held increment", s.Total)
	}
	st := stepOf(t, ctx, f, next)
	if st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, "on hold: the mail provider is not chosen") {
		t.Fatalf("the held increment's work: %s (%q); want waiting, saying the increment is on hold and why", st.State, st.Waits)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: in.ID}); got.Increment.Held == nil {
		t.Error("the increment read back shows no hold")
	}

	if _, err := inc.Resume(ctx, api.IncrementHoldInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, f, next); st.State != schedapi.StateReady {
		t.Errorf("resumed, the work: %s (%q); want ready again", st.State, st.Waits)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: in.ID}); got.Increment.Held != nil {
		t.Error("resumed, the increment still shows a hold")
	}
	if _, err := inc.Resume(ctx, api.IncrementHoldInput{ID: in.ID, By: person}); err == nil {
		t.Error("an increment not on hold was resumed")
	}
}

// A settle that waited behind others decides on what is scheduled now, never
// on its own older list: a to-do collected meanwhile holds nothing, so the
// step after it stays ready - and an increment's hold reason is never written
// over with "after <the collected to-do>" by a late settle (a slow runner).
func TestALateSettleNeverHoldsWorkAfterACollectedTodo(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild, Priority: "P1"})
	stale, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	if st := stepOf(t, ctx, r.ID, "analyse"); st.State != schedapi.StateReady {
		t.Fatalf("the intake collected, the analyse: %s (%q); want ready", st.State, st.Waits)
	}
	if err := plancore.SettleTodos(ctx, stale.Todos); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, r.ID, "analyse"); st.State != schedapi.StateReady {
		t.Errorf("settled from a list taken before the intake was collected, the analyse: %s (%q); want still ready", st.State, st.Waits)
	}
}

// An increment says what it depends on: its depends-on edges, read back.
func TestAnIncrementSaysWhatItDependsOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	first, second := increment(t, ctx, "Invoices are mailed"), increment(t, ctx, "Reminders are mailed")
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: second.ID, To: first.ID, Kind: cwmapi.EdgeType(api.EdgeDependsOn)}); err != nil {
		t.Fatal(err)
	}
	got, err := inc.Get(ctx, api.GetInput{ID: second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Increment.DependsOn) != 1 || got.Increment.DependsOn[0] != first.ID {
		t.Errorf("%s depends on %v; want %s", second.ID, got.Increment.DependsOn, first.ID)
	}
}
