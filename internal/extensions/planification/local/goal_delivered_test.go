// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A GOAL IS DELIVERED BY ITS FEATURES: a goal an
// increment ships counts done when every feature serving it is done - never
// by its request's plan, which a goal added by a delta carries and ends.
func TestAGoalCountsByTheFeaturesServingIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Sources chosen the same way everywhere")
	create := func(o cwmapi.Object) string {
		t.Helper()
		o.Parent = "billing"
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
		if err != nil {
			t.Fatalf("create %s %q: %v", o.Type, o.Name, err)
		}
		return out.Object.ID
	}
	g := cwmapi.Object{Type: "goal", Name: "Sources are chosen the same way"}
	_ = g.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "asymptotic"})
	// Its request ended done, as the build recipe's review ended it.
	_ = g.SetCapability(api.CapRequest, api.RequestFields{Kind: api.KindBuild})
	_ = g.SetCapability(api.CapPlan, map[string]any{"state": api.StateDone})
	goal := create(g)
	// A feature is delivered the way a recorded one is: realized, then
	// confirmed by a person.
	deliver := func(id string) {
		t.Helper()
		for _, to := range []string{"realized", "done"} {
			if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: person, Force: true, Note: "built"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	feature := func(name, state string) string {
		t.Helper()
		f := cwmapi.Object{Type: "feature", Name: name}
		_ = f.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
		id := create(f)
		if state == "done" {
			deliver(id)
		}
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: id, To: goal, Kind: api.EdgeServes}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: goal, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	doneOf := func() bool {
		t.Helper()
		sc, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, tg := range sc.Targets {
			if tg.ID == goal {
				// Its state says what it counts by: done only when done.
				if tg.Done != (tg.State == api.TargetDone) {
					t.Errorf("the goal shows %q but counts done=%v", tg.State, tg.Done)
				}
				return tg.Done
			}
		}
		t.Fatalf("the goal is not in the increment's scope: %+v", sc.Targets)
		return false
	}
	if doneOf() {
		t.Error("a goal with no feature counts done (its request's plan ended)")
	}
	// Something not a feature serving the goal (here the increment itself) neither delivers nor holds it.
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: in.ID, To: goal, Kind: api.EdgeServes}); err != nil {
		t.Fatal(err)
	}
	a := feature("One picker", "done")
	b := feature("One source per connector", "")
	if doneOf() {
		t.Error("a goal counts done while one of its features is not")
	}
	// A dropped feature holds nothing back.
	dropped := feature("A third picker", "")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: dropped, To: "dropped", By: person, Force: true, Note: "no"}); err != nil {
		t.Fatal(err)
	}
	deliver(b)
	if !doneOf() {
		t.Errorf("a goal whose features (%s, %s) are done does not count done", a, b)
	}
}
