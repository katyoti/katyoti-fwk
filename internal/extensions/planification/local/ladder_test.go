// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
)

// THE LADDERS, on the workflows ktw-methods ships: a goal is decomposed before it
// is pursued; a feature is specified, then made ready by a person, before it is
// built; a release freezes only on a refined scope. And what starts: refine work
// first, then priority.

func ladderWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	return world(t)
}

// want creates a want of a type in billing, with its edges.
func want(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, typ, title string, edges ...cwmapi.ObjectEdge) string {
	t.Helper()
	o := cwmapi.Object{Type: typ, Edges: edges}
	o.Parent = "billing"
	o.Name = title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create %s %q: %v", typ, title, err)
	}
	return out.Object.ID
}

// move transitions an object, by an agent or a person.
func move(ctx context.Context, cwm cwmapi.CWMService, id, to string, by cwmapi.Actor) error {
	_, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: by})
	return err
}

func accepted(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	for _, to := range []string{"proposed", "in-review"} {
		if err := move(ctx, cwm, id, to, intake); err != nil {
			t.Fatal(err)
		}
	}
	if err := move(ctx, cwm, id, "accepted", person); err != nil {
		t.Fatal(err)
	}
}

// A goal is decomposed only once a want serves it.
func TestTheGoalLadder(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := ladderWorld(t)
	g := want(t, ctx, cwm, "goal", "Invoices are sent")
	accepted(t, ctx, cwm, g)
	if err := move(ctx, cwm, g, "active", intake); !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("an accepted goal pursued before it is decomposed: %v, want refused", err)
	}
	if err := move(ctx, cwm, g, "decomposed", intake); !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("decomposed with nothing serving it: %v, want refused", err)
	}
	want(t, ctx, cwm, "feature", "Send by mail", cwmapi.ObjectEdge{Kind: "serves", To: g})
	if err := move(ctx, cwm, g, "decomposed", intake); err != nil {
		t.Fatalf("decomposed once a feature serves it: %v", err)
	}
}

// A feature is specified once it wants a use case (ready-to-plan), and ready
// once a person confirmed they challenged it (ready-to-build); code is for a
// ready feature.
func TestTheFeatureLadder(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := ladderWorld(t)
	f := want(t, ctx, cwm, "feature", "Send by mail")
	accepted(t, ctx, cwm, f)
	if err := move(ctx, cwm, f, "specified", intake); !errors.Is(err, cwmapi.ErrTransitionRefused) {
		t.Fatalf("specified with no use case: %v, want refused", err)
	}
	uc := usecase(t, ctx, cwm, "A mail is sent for each invoice")
	planlocal.Settle(ctx, 5*time.Second) // the refusal's resolution is written on f first
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: f})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: "wants", To: uc})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, f, "specified", intake); err != nil {
		t.Fatalf("specified once it wants a use case: %v", err)
	}
	if err := move(ctx, cwm, f, "ready", person); !errors.Is(err, cwmapi.ErrTransitionRefused) || !strings.Contains(err.Error(), "feature-challenged") {
		t.Fatalf("ready before a person challenged it: %v, want refused naming feature-challenged", err)
	}
	planlocal.Settle(ctx, 5*time.Second) // the planner writes on the object after a refusal: CWM writes are not atomic (object-update-loses-concurrent-writes)
	if _, err := gatesOf(t, ctx).Confirm(ctx, gatesapi.ConfirmInput{ID: f, Gate: "ready-to-build", Requirement: "feature-challenged", By: person}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, f, "ready", intake); err != nil {
		t.Fatalf("ready once a person challenged it: %v", err)
	}
	if err := move(ctx, cwm, f, "active", intake); err != nil {
		t.Fatalf("a ready feature built: %v", err)
	}
}

// A release freezes only when its goals are decomposed and its features
// specified — its plan's dependencies known.
func TestAReleaseFreezesOnlyOnARefinedScope(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := ladderWorld(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	g := want(t, ctx, cwm, "goal", "Invoices are sent")
	accepted(t, ctx, cwm, g)
	// A release holds it through an increment.
	in := increment(t, ctx, "Invoices are sent")
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := increments(t, ctx).ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person})
	if !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), g) {
		t.Fatalf("freeze with an undecomposed goal: %v, want refused naming %s", err, g)
	}
	want(t, ctx, cwm, "feature", "Send by mail", cwmapi.ObjectEdge{Kind: "serves", To: g})
	if err := move(ctx, cwm, g, "decomposed", intake); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil {
		t.Fatalf("freeze once its goal is decomposed: %v", err)
	}
}

// A feature carries its own request: the work asked on it is planned — its
// plan ready, its intake first — while the feature keeps its own ladder; the
// plan ending does not move the feature.
func TestAFeatureCarriesItsOwnRequest(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := ladderWorld(t)
	f := want(t, ctx, cwm, "feature", "Send by mail")
	accepted(t, ctx, cwm, f)
	got, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, Priority: "P1", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if r := got.Request; r.State != api.PlanReady || r.Type != "feature" || r.Title != "Send by mail" || r.Priority != "P1" ||
		r.Plan == nil || r.Plan.Priority != "P1" {
		t.Fatalf("required = %+v, want a P1 request on the feature, its intake planned", r)
	}
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(eff.Capabilities, func(c cwmapi.CapabilityRef) bool { return c.ID == api.CapNetworkNode }) {
		t.Errorf("a required feature is no network node (depends-on needs one): %+v", eff.Capabilities)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, By: person}); !errors.Is(err, cwmapi.ErrAlreadyExists) {
		t.Errorf("required twice: %v, want already exists", err)
	}
	list, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil || len(list.Requests) != 1 || list.Requests[0].ID != f {
		t.Fatalf("billing's requests = %+v, %v; want the feature", list.Requests, err)
	}

	// Its plan ending leaves the feature's ladder where it is; the ladder
	// moves on its own.
	walk(t, ctx, cwm, f, api.PlanDone)
	obj, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := obj.Object.Lifecycle(); l.State != "accepted" {
		t.Errorf("the feature's lifecycle = %q; its plan's end must leave it accepted", l.State)
	}
	if err := move(ctx, cwm, f, "dropped", person); err != nil {
		t.Errorf("the feature's own lifecycle refused: %v", err)
	}
}

// A request object has no lifecycle of its own: its state is its plan's.
func TestARequestsStateIsItsPlans(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the rounding", Module: "billing", Kind: api.KindFix})
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Object.Lifecycle(); ok {
		t.Error("a request carries a lifecycle beside its plan")
	}
	if err := move(ctx, cwm, r.ID, api.StateDone, intake); err == nil {
		t.Error("a request moved by a lifecycle of its own; only its plan moves")
	}
	p, _, _ := cwmapi.DecodeCapability[api.PlanFields](got.Object, api.CapPlan)
	if r.State != api.PlanReady || p.State != api.PlanReady || len(p.Trail) != 1 || p.Trail[0].To != api.PlanReady {
		t.Errorf("request = %q, plan state %q trail %+v; want ready, with its move", r.State, p.State, p.Trail)
	}
}
