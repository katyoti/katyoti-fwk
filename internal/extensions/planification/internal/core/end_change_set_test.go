// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A REQUEST'S END IS ONE CHANGE SET (F-085): its
// node's move and its plan's end are written together or not at all. A cut
// between the two writes neither and archives none of its to-dos; the end
// done again writes both.

// endPack declares what an end writes: a use case on the manual workflow
// (its node, done reachable from anywhere) and a request carrying a plan.
func endPack() fstest.MapFS {
	return fstest.MapFS{
		"capabilities/container.yaml": {Data: []byte("name: container\n")},
		"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		"capabilities/module.yaml":    {Data: []byte("name: module\n")},
		"capabilities/want.yaml":      {Data: []byte("name: want\n")},
		"capabilities/request.yaml":   {Data: []byte("name: request\n")},
		"capabilities/plan.yaml":      {Data: []byte("name: plan\nworkflow: {name: plan, state: state, history: trail}\n")},
		"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container, module]\n")},
		"object-types/use-case.yaml":  {Data: []byte("name: use-case\nfolder: use-cases\nid_prefix: UC\ncapabilities: [container, want, lifecycle]\nworkflow: manual\n")},
		"object-types/request.yaml":   {Data: []byte("name: request\nfolder: .ktw/requests\nid_prefix: RQ\ncapabilities: [request]\n")},
		"workflows/manual.yaml": {Data: []byte(`name: manual
initial: idea
states: [{name: idea}, {name: active}, {name: done}]
transitions:
  - {name: start, from: ["*"], to: active}
  - {name: finish, from: ["*"], to: done}
`)},
		// The plan workflow, its finish's module-updated gate left out: the
		// end's writes are what is tested here.
		"workflows/plan.yaml": {Data: []byte(`name: plan
initial: idle
states: [{name: idle}, {name: ready}, {name: blocked}, {name: split}, {name: done}, {name: dropped}]
transitions:
  - {name: plan, from: [idle, ready, blocked], to: ready}
  - {name: block, from: [idle, ready], to: blocked}
  - {name: finish, from: [idle, ready, blocked, split], to: done}
  - {name: drop, from: [idle, ready, blocked, split], to: dropped}
`)},
	}
}

func TestAnEndCutBetweenItsNodeMoveAndItsPlanEndWritesNeither(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		cwmapp.Pack(endPack()),
		schedlocal.Extension(),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	by := cwmapi.Actor{ID: "builder", Kind: "agent"}

	uc, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: cwmapi.Object{
		Type: "use-case", Parent: "billing", Name: "Pay by card",
	}})
	if err != nil {
		t.Fatal(err)
	}
	node := uc.Object.ID
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: node, To: "active", By: by}); err != nil {
		t.Fatal(err)
	}
	rq, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: cwmapi.Object{
		Type: "request", Parent: "billing", Name: "Build pay by card",
		Capabilities: map[string]map[string]any{
			api.CapRequest: {"kind": api.KindBuild},
			api.CapPlan:    {"priority": api.DefaultPriority},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := rq.Object.ID
	if _, err := planTransition(ctx, cwm, request, api.PlanReady, "plan", by, "planned"); err != nil {
		t.Fatal(err)
	}
	// A to-do of its still open: an end archives it, a cut end never does.
	if _, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{Module: "billing", Request: request, Target: node, Title: "implement", For: schedapi.ForAgent, Start: schedapi.StartManual, Priority: api.DefaultPriority}}); err != nil {
		t.Fatal(err)
	}
	state := func() (string, string, int) {
		t.Helper()
		n, err := loadObject(ctx, cwm, node)
		if err != nil {
			t.Fatal(err)
		}
		r, err := loadObject(ctx, cwm, request)
		if err != nil {
			t.Fatal(err)
		}
		open, err := todosOf(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return ownState(n), planState(r), len(open)
	}

	cut := errors.New("cut between the node's move and the plan's end")
	endCut = func(string) error { return cut }
	after, refused, _, err := endWrites(ctx, cwm, NewPlans(), request, node, api.StateDone, by)
	endCut = nil
	if !errors.Is(err, cut) || refused != "" || after != nil {
		t.Fatalf("a cut end = (after %v, refused %q, %v), want the cut's error", after != nil, refused, err)
	}
	if n, p, open := state(); n != "active" || p != api.PlanReady || open != 1 {
		t.Fatalf("after a cut end: node %s, plan %s, %d to-dos open - want active, ready, 1: neither written", n, p, open)
	}

	// Done again, the end writes both, and what follows it archives its to-dos.
	after, refused, _, err = endWrites(ctx, cwm, NewPlans(), request, node, api.StateDone, by)
	if err != nil || refused != "" {
		t.Fatalf("the end done again = (refused %q, %v)", refused, err)
	}
	after(ctx)
	if n, p, open := state(); n != api.StateDone || p != api.PlanDone || open != 0 {
		t.Fatalf("after the end: node %s, plan %s, %d to-dos open - want done, done, 0", n, p, open)
	}
}
