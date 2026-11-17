// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
)

var planner = cwmapi.Actor{ID: "planner", Kind: "agent"}

func plansOf(t *testing.T, ctx context.Context) *plancore.Plans {
	t.Helper()
	svc, err := application.UseService[api.PlanService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := svc.(*plancore.Plans)
	if !ok {
		t.Fatalf("the plan service is a %T", svc)
	}
	return p
}

// Planning an object gives it a plan, ready — at P2 unless a priority is
// given — and who planned it is in its trail. Its steps are its to-dos: the
// plan names none.
func TestPlanningAnObjectMakesItReady(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	plans := plansOf(t, ctx)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing"})

	out, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	p := out.Plan
	if p.State != api.PlanReady || p.Priority != api.DefaultPriority || len(p.Trail) == 0 || p.Trail[len(p.Trail)-1].By.ID != person.ID {
		t.Fatalf("plan = %+v, want ready, P2, planned by the person", p)
	}

	for name, in := range map[string]api.PlanNextInput{
		"bad prio": {ID: r.ID, Priority: "urgent", By: person},
		"nobody":   {ID: r.ID},
	} {
		if _, err := plans.Plan(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
}

// A plan's life: planned, blocked with what blocks it, planned again (the
// block's note cleared, its priority kept), finished. A finished plan is not
// planned again. Whether a step runs is not a plan state: no move reaches
// running.
func TestAPlanMovesThroughItsLife(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	plans := plansOf(t, ctx)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing"})
	state := func(out api.PlannedOutput, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return out.Plan.State
	}
	if st := state(plans.Plan(ctx, api.PlanNextInput{ID: r.ID, Priority: "P1", By: person})); st != api.PlanReady {
		t.Fatalf("planned: %s", st)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: r.ID, To: "running", By: planner, Capability: api.CapPlan}); !errors.Is(err, cwmapi.ErrTransitionRefused) && !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a plan moved to running: %v, want refused (no such state)", err)
	}
	if _, err := plans.Block(ctx, api.BlockInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("block without a note: %v, want refused", err)
	}
	if st := state(plans.Block(ctx, api.BlockInput{ID: r.ID, Note: "the spec is wrong", By: person})); st != api.PlanBlocked {
		t.Fatalf("blocked: %s", st)
	}
	if st := state(plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: person})); st != api.PlanReady {
		t.Fatalf("planned after blocked: %s", st)
	}
	out, err := plans.Get(ctx, api.GetPlanInput{ID: r.ID})
	if state(out, err) != api.PlanReady || out.Plan.Priority != "P1" || out.Plan.Note != "" {
		t.Errorf("get = %+v, want ready, P1 kept, the block's note cleared", out.Plan)
	}
	if st := state(plans.Finish(ctx, api.FinishInput{ID: r.ID, By: person})); st != api.PlanDone {
		t.Fatalf("finished: %s", st)
	}
	if _, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("planning a done plan: %v, want refused", err)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, Outcome: "maybe", By: person}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("finish with outcome maybe: %v, want refused", err)
	}
}

// The plans are listed by priority, then id; filtered by state.
func TestPlansAreListedByPriority(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	plans := plansOf(t, ctx)
	ids := map[string]string{}
	for _, c := range []struct{ name, prio string }{{"later", "P3"}, {"urgent", "P0"}, {"next", "P1"}, {"also urgent", "P0"}} {
		r := file(t, ctx, req, api.FileInput{Title: c.name, Module: "billing"})
		ids[c.name] = r.ID
		if _, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, Priority: c.prio, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plans.Block(ctx, api.BlockInput{ID: ids["next"], Note: "waits on a person", By: person}); err != nil {
		t.Fatal(err)
	}

	got, err := plans.List(ctx, api.ListPlansInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, p := range got.Plans {
		order = append(order, p.Title)
	}
	want := []string{"urgent", "also urgent", "next", "later"} // P0 by id (filed first), P1, P3
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("listed %v, want %v (priority, then id)", order, want)
	}
	ready, _ := plans.List(ctx, api.ListPlansInput{Module: "billing", State: api.PlanReady})
	if len(ready.Plans) != 3 {
		t.Errorf("ready plans = %d, want 3", len(ready.Plans))
	}
}

// Who planned it is in the trail, a re-plan of a ready plan included: the
// plan carries no separate "planned by".
func TestTheTrailSaysWhoPlannedIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	plans := plansOf(t, ctx)
	r := file(t, ctx, req, api.FileInput{Title: "Re-planned", Module: "billing"})
	other := cwmapi.Actor{ID: "planner-session", Kind: "agent"}
	out, err := plans.Plan(ctx, api.PlanNextInput{ID: r.ID, By: other})
	if err != nil {
		t.Fatal(err)
	}
	trail := out.Plan.Trail
	if out.Plan.State != api.PlanReady || len(trail) == 0 || trail[len(trail)-1].By.ID != other.ID {
		t.Fatalf("re-planning a ready plan: last trail entry %+v, want one by %s", trail[len(trail)-1], other.ID)
	}
}
