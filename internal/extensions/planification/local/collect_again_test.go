// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PLANNING CHANGE IS SAFE TO RETRY: a collect run
// again on the same report - a cut collect the scheduler hands back - adds no
// second set of the steps the report added, files no second follow-up and
// records the step once.
func TestACollectRunAgainOnTheSameReportAddsNothingTwice(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	impl := stepOf(t, ctx, r.ID, "implement")
	pullAndReport(t, ctx, impl.ID, session, schedapi.Result{Summary: "implemented",
		More: map[string]any{
			"add": []map[string]any{
				{"step": "gui-test", "task": "coder", "instructions": "Walk the invoice screen."},
				{"step": "gui-shot", "task": "coder", "instructions": "Capture the invoice screen."},
			},
			"follow_ups": []map[string]any{{"title": "The totals round twice", "details": "Seen in the invoice list.", "kind": "fix"}},
		}})

	got, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: impl.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Todo.Result == nil || got.Todo.Archived == "" {
		t.Fatalf("the implement was not collected: %+v", got.Todo)
	}
	again := got.Todo
	again.State = schedapi.StateReported
	if err := plancore.Collect(ctx, again); err != nil {
		t.Fatalf("collect the same report again: %v", err)
	}

	open, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"gui-test", "gui-shot"} {
		n := 0
		for _, x := range open.Todos {
			if x.Packet.Step == step {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d %s steps after the collect ran again; want 1", n, step)
		}
	}
	listed, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range listed.Requests {
		if x.Title == "The totals round twice" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d follow-ups « The totals round twice » after the collect ran again; want 1", n)
	}
	g, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	recs := 0
	for _, s := range g.Request.Plan.Steps {
		if s.Step == impl.Packet.Template+"/implement" {
			recs++
		}
	}
	if recs != 1 {
		t.Errorf("the implement recorded %d times in the request's steps; want once: %+v", recs, g.Request.Plan.Steps)
	}
}
