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
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// PLANNING, end to end on a real CWM: the release is the plan — its steps, its
// network and its gates.

func plannings(t *testing.T, ctx context.Context) api.PlanningService {
	t.Helper()
	svc, err := application.UseService[api.PlanningService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// planWorld: billing has a child module, shipping; docs stands alone. Release
// billing 1.4.0 is planned, with a budget.
func planWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.RequestService, api.PlanningService, api.Release) {
	t.Helper()
	ctx, cwm, req := world(t)
	child := cwmapi.NewContainerObject("shipping", "shipping", "module", "billing", "shipping", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shipping", Object: child}); err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "docs")
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0", BudgetUSD: 50})
	return ctx, cwm, req, plannings(t, ctx), r
}

// joinPlan puts a request in a release's plan: it targets the release.
func joinPlan(ctx context.Context, request, release string) error {
	rel, err := application.UseService[api.ReleaseService](ctx)
	if err != nil {
		return err
	}
	_, err = rel.Target(ctx, api.TargetInput{ID: request, Release: release})
	return err
}

// fileIn files a request in a module and puts it in the release's plan.
func fileIn(t *testing.T, ctx context.Context, req api.RequestService, _ api.PlanningService, module, title, rel string) string {
	t.Helper()
	r := file(t, ctx, req, api.FileInput{Title: title, Module: module})
	if err := joinPlan(ctx, r.ID, rel); err != nil {
		t.Fatalf("join %s: %v", r.ID, err)
	}
	return r.ID
}

// The release carries its plan: its steps (its children) and the requests
// targeting it — a goal targeting it with no request on it is scope, not work.
// A released release takes no new step.
func TestAReleaseCarriesItsPlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, pl, rel := planWorld(t)
	a := fileIn(t, ctx, req, pl, "billing", "fix invoices", rel.ID)
	b := fileIn(t, ctx, req, pl, "shipping", "route parcels", rel.ID)
	g := usecase(t, ctx, cwm, "Invoices are sent")
	if _, err := releases(t, ctx).Target(ctx, api.TargetInput{ID: g, Release: rel.ID}); err != nil {
		t.Fatal(err)
	}
	step, err := pl.AddStep(ctx, api.AddStepInput{Release: rel.ID, Name: "Sprint 1", After: []string{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := pl.GetPlan(ctx, api.GetInput{ID: rel.ID})
	if err != nil {
		t.Fatal(err)
	}
	if p := got.Plan; strings.Join(p.Requests, ",") != a+","+b || len(p.Steps) != 1 || p.Steps[0].ID != step.Step.ID || p.Steps[0].Release != rel.ID {
		t.Fatalf("plan = %+v; want %s and %s, and the step", p, a, b)
	}
	if rel.BudgetUSD != 50 {
		t.Errorf("budget = %v, want 50", rel.BudgetUSD)
	}
	r := releases(t, ctx)
	walk(t, ctx, cwm, a, api.PlanReady, api.StateDone)
	walk(t, ctx, cwm, b, api.PlanReady, api.StateDone)
	if _, err := r.Untarget(ctx, api.UntargetInput{ID: g}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Freeze(ctx, api.MoveInput{ID: rel.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, rel.ID)
	if _, err := r.Release(ctx, api.MoveInput{ID: rel.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.AddStep(ctx, api.AddStepInput{Release: rel.ID, Name: "late"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("a step for a released release: %v, want refused", err)
	}
}

// A step is reached only when every branch feeding it is done; a request
// waiting on a step is ready only once it is reached.
func TestAStepJoinsItsBranches(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, tr, inc := planWorld(t)
	a := fileIn(t, ctx, req, tr, "billing", "branch a", inc.ID)
	b := fileIn(t, ctx, req, tr, "shipping", "branch b", inc.ID)
	step, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "Sprint 1", Produces: "invoices shipped", After: []string{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	outside := file(t, ctx, req, api.FileInput{Title: "not in the plan"})
	if _, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "bad", After: []string{outside.ID}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a step after a request of no plan: %v, want refused", err)
	}
	reach := api.AdvanceInput{ID: step.Step.ID, To: api.StepReached, By: intake}
	walk(t, ctx, cwm, a, api.PlanReady, api.StateDone)
	if _, err := tr.Advance(ctx, reach); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), b) {
		t.Fatalf("reach with %s open: %v, want refused naming it", b, err)
	}
	walk(t, ctx, cwm, b, api.PlanReady, api.StateDone)
	if _, err := tr.Advance(ctx, reach); err != nil {
		t.Fatalf("reach with every branch done: %v", err)
	}
}

// fileEst files a request with an estimate in a module and puts it in the plan.
func fileEst(t *testing.T, ctx context.Context, req api.RequestService, _ api.PlanningService, module, title, rel string, est float64, after ...string) string {
	t.Helper()
	r := file(t, ctx, req, api.FileInput{Title: title, Module: module, Estimate: est, DependsOn: after})
	if err := joinPlan(ctx, r.ID, rel); err != nil {
		t.Fatalf("join %s: %v", r.ID, err)
	}
	return r.ID
}

func nodeOf(t *testing.T, net api.NetworkOutput, id string) api.NetNode {
	t.Helper()
	for _, n := range net.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("%s is not in the network", id)
	return api.NetNode{}
}

// The network answers what can start now, the critical path and its length,
// each node's slack, and what a step waits on — over the REMAINING work.
//
//	a(2) ──▶ c(3) ──┐
//	b(1) ───────────┴─▶ S          d(1) alone
func TestTheNetworkOfARelease(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, tr, inc := planWorld(t)
	a := fileEst(t, ctx, req, tr, "billing", "a", inc.ID, 2)
	b := fileEst(t, ctx, req, tr, "billing", "b", inc.ID, 1)
	c := fileEst(t, ctx, req, tr, "shipping", "c", inc.ID, 3, a)
	d := fileEst(t, ctx, req, tr, "shipping", "d", inc.ID, 0)
	s, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "Sprint 1", After: []string{b, c}})
	if err != nil {
		t.Fatal(err)
	}
	net, err := tr.Network(ctx, api.GetInput{ID: inc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(net.Runnable, ","); got != strings.Join([]string{a, b, d}, ",") {
		t.Errorf("runnable = %s, want %s,%s,%s (c waits on a)", got, a, b, d)
	}
	if got := strings.Join(net.CriticalPath, ","); got != a+","+c+","+s.Step.ID || net.Duration != 5 {
		t.Errorf("critical path = %s (%v), want %s,%s,%s (5)", got, net.Duration, a, c, s.Step.ID)
	}
	if n := nodeOf(t, net, b); n.Slack != 4 || n.Critical {
		t.Errorf("b: slack %v critical %v, want 4, not critical", n.Slack, n.Critical)
	}
	if n := nodeOf(t, net, d); n.Weight != 1 || n.Slack != 4 {
		t.Errorf("d (no estimate): weight %v slack %v, want 1 and 4", n.Weight, n.Slack)
	}
	if len(net.Steps) != 1 || strings.Join(net.Steps[0].WaitsOn, ",") != b+","+c || net.Steps[0].Reachable {
		t.Errorf("step = %+v, want waiting on %s and %s", net.Steps, b, c)
	}
	// a done: it weighs nothing any more, c can start, the rest shortens.
	walk(t, ctx, cwm, a, api.PlanReady, api.StateDone)
	net, err = tr.Network(ctx, api.GetInput{ID: inc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(net.Runnable, ","); got != strings.Join([]string{b, c, d}, ",") || net.Duration != 3 {
		t.Errorf("after a: runnable %s duration %v, want %s,%s,%s and 3", got, net.Duration, b, c, d)
	}
	if got := strings.Join(net.CriticalPath, ","); got != c+","+s.Step.ID {
		t.Errorf("after a: critical path %s, want %s,%s", got, c, s.Step.ID)
	}
}

// A cycle written behind the service's back is an error naming it, never a
// silently wrong schedule.
func TestANetworkCycleIsNamed(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, tr, inc := planWorld(t)
	a := fileEst(t, ctx, req, tr, "billing", "a", inc.ID, 1)
	b := fileEst(t, ctx, req, tr, "billing", "b", inc.ID, 1, a)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: a})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: b})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Network(ctx, api.GetInput{ID: inc.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), a) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Network over a cycle: %v, want an error naming %s", err, a)
	}
}

// Ordering across requests and steps refuses a cycle.
func TestNoCycleThroughASteps(t *testing.T) {
	t.Parallel()
	ctx, _, req, tr, inc := planWorld(t)
	a := fileIn(t, ctx, req, tr, "billing", "a", inc.ID)
	step, err := tr.AddStep(ctx, api.AddStepInput{Release: inc.ID, Name: "Sprint 1", After: []string{a}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: a, On: step.Step.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("a after its own step: %v, want a cycle refused", err)
	}
}

// A project release's network is also its module releases': work in two
// modules, one waiting on the other, makes one critical path across them.
func TestAProjectReleaseNetworkCrossesItsModules(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shipping")
	rel := releases(t, ctx)
	p := plan(t, ctx, rel, api.PlanInput{Version: "2026.10"})
	mb := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0", ShipsIn: p.ID})
	ms := plan(t, ctx, rel, api.PlanInput{Module: "shipping", Version: "2.0.0", ShipsIn: p.ID})
	a := file(t, ctx, req, api.FileInput{Title: "Totals in cents", Module: "billing", Estimate: 2})
	b := file(t, ctx, req, api.FileInput{Title: "Ship with the totals", Module: "shipping", Estimate: 3, DependsOn: []string{a.ID}})
	for id, r := range map[string]string{a.ID: mb.ID, b.ID: ms.ID} {
		if _, err := rel.Target(ctx, api.TargetInput{ID: id, Release: r}); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := application.UseService[api.PlanningService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	net, err := pl.Network(ctx, api.GetInput{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(net.Nodes) != 2 || strings.Join(net.CriticalPath, ",") != a.ID+","+b.ID || net.Duration != 5 {
		t.Fatalf("project network = %d nodes, path %v, duration %g; want both modules' requests, %s then %s, 5", len(net.Nodes), net.CriticalPath, net.Duration, a.ID, b.ID)
	}
}
