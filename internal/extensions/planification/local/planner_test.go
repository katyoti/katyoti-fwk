// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE PLANNING WORLD — a CWM, the planification services and a fake task
// service that records the runs started.

type plannerWorld struct {
	t     *testing.T
	ctx   context.Context
	cwm   cwmapi.CWMService
	req   api.RequestService
	plans *plancore.Plans
	tasks *fakeTasks
}

func newPlannerWorld(t *testing.T) *plannerWorld {
	t.Helper()
	ctx, cwm, req := world(t)
	for _, m := range []string{"shipping", "stock", "orders", "pricing"} {
		cwmapp.Module(t, ctx, cwm, m)
	}
	plans := plansOf(t, ctx)
	return &plannerWorld{t: t, ctx: ctx, cwm: cwm, req: req, plans: plans, tasks: tasksOf(t, ctx)}
}

// planned files an object in module and plans it, at prio.
func (w *plannerWorld) planned(title, module, prio string) string {
	w.t.Helper()
	r := file(w.t, w.ctx, w.req, api.FileInput{Title: title, Module: module})
	if _, err := w.plans.Plan(w.ctx, api.PlanNextInput{ID: r.ID, Priority: prio, By: person}); err != nil {
		w.t.Fatal(err)
	}
	return r.ID
}

func (w *plannerWorld) plan(id string) api.PlannedObject {
	w.t.Helper()
	out, err := w.plans.Get(w.ctx, api.GetPlanInput{ID: id})
	if err != nil {
		w.t.Fatal(err)
	}
	return out.Plan
}

// A transition — of a plan, or of what one depends on — wakes the planner.
func TestATransitionWakesThePlanner(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	bus, err := application.UseService[eventbus.EventBus](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	woke := make(chan struct{}, 8)
	bus.Subscribe(engine.PlanChangedTopic(), func(string, any) {
		select { // the bus calls back in line: never block it
		case woke <- struct{}{}:
		default:
		}
	})
	w.planned("A", "billing", "")
	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("planning an object did not wake the planner")
	}
}

// The intake does none of a request's work: it may end one as dropped (a
// duplicate), never as done — ending its own step is completing it.
func TestTheIntakeDoesNotFinishARequestAsDone(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	r := file(t, w.ctx, w.req, api.FileInput{Title: "A", Module: "billing"})
	_, err := w.plans.Finish(w.ctx, api.FinishInput{ID: r.ID, By: intake, Note: "clear; recipe proceeds"})
	if err == nil || !strings.Contains(err.Error(), "only as dropped") {
		t.Fatalf("the intake finishing as done = %v, want refused", err)
	}
	if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: intake, Note: "a duplicate"}); err != nil {
		t.Fatalf("the intake dropping a duplicate = %v", err)
	}
}

// One open request per object: filed again once its request ended — the next
// rung of its work, a feature specified then built — the request is refiled
// (its fields the new ones, its plan ready again, the trail kept); filed while
// it is open, it is refused.
func TestARequestIsFiledAgainOnceItEnded(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	f := want(t, w.ctx, w.cwm, "feature", "Send by mail")
	file(t, w.ctx, w.req, api.FileInput{On: f, Kind: api.KindAnalyse})
	if _, err := w.req.File(w.ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err == nil || !strings.Contains(err.Error(), "open request") {
		t.Fatalf("filed while open = %v, want refused", err)
	}
	if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: f, By: builder, Note: "specified"}); err != nil {
		t.Fatal(err)
	}
	out, err := w.req.File(w.ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person})
	if err != nil {
		t.Fatalf("filed again once done: %v", err)
	}
	p := w.plan(f)
	refiled := false
	for _, m := range p.Trail {
		refiled = refiled || m.Transition == "refile"
	}
	if out.Request.Kind != api.KindBuild || p.State != api.PlanReady || !refiled {
		t.Fatalf("refiled: kind %s, plan %s, trail %v; want build, ready, a refile in the trail", out.Request.Kind, p.State, p.Trail)
	}
}

// A request refiled by another actor keeps who asked first and what its
// intake assessed (estimate, depth): no verb could set them back.
func TestARefiledRequestKeepsItsRequesterEstimateAndDepth(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	f := want(t, w.ctx, w.cwm, "feature", "Send by mail")
	file(t, w.ctx, w.req, api.FileInput{On: f, Kind: api.KindAnalyse, By: person})
	if _, err := w.req.Assess(w.ctx, api.AssessInput{ID: f, Estimate: 3, Depth: "quick", By: intake}); err != nil {
		t.Fatalf("assess: %v", err)
	}
	if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: f, By: builder, Note: "specified"}); err != nil {
		t.Fatal(err)
	}
	out, err := w.req.File(w.ctx, api.FileInput{On: f, Kind: api.KindBuild, By: builder})
	if err != nil {
		t.Fatalf("refiled by another actor: %v", err)
	}
	r := out.Request
	if r.Requester != person || r.Estimate != 3 || r.Depth != "quick" || r.Kind != api.KindBuild {
		t.Fatalf("refiled: requester %v, estimate %v, depth %q, kind %s; want %v, 3, quick, build", r.Requester, r.Estimate, r.Depth, r.Kind, person)
	}
}
