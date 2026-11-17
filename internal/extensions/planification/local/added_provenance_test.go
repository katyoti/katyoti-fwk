// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// EVERY TO-DO ADDED BEYOND A REQUEST'S RECIPE SAYS WHO ADDED IT, WHY AND ITS
// GENERATION: the reporting actor, the report's line, the
// adding to-do's generation plus one - a recipe step is generation 0 and says
// nothing. A rejected validation's rework, a blocked report's added steps and
// a step added by hand (work add) carry it too, and the request's steps show it.
func TestAnAddedStepSaysWhoAddedItWhyAndItsGeneration(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "implemented\nthe screen needs a walk",
		More: map[string]any{"add": []map[string]any{
			{"step": "gui-test", "task": "coder", "instructions": "Walk the invoice screen."},
			{"step": "gui-shot", "task": "coder", "instructions": "Capture the invoice screen."},
		}}})
	for _, s := range []string{"gui-test", "gui-shot"} {
		got := stepOf(t, ctx, r.ID, s).Packet
		if got.AddedBy == nil || *got.AddedBy != session || got.AddedWhy != "implemented" || got.Generation != 1 {
			t.Errorf("%s added by the implement's report: by %v, why %q, generation %d; want %s, %q, 1", s, got.AddedBy, got.AddedWhy, got.Generation, session.ID, "implemented")
		}
	}
	for _, s := range []string{"auto-review", "module-update", "review"} {
		if got := stepOf(t, ctx, r.ID, s).Packet; got.AddedBy != nil || got.AddedWhy != "" || got.Generation != 0 {
			t.Errorf("the recipe step %s says it was added: by %v, why %q, generation %d", s, got.AddedBy, got.AddedWhy, got.Generation)
		}
	}

	// A step added by a generation-1 to-do is generation 2.
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "gui-test").ID, another, schedapi.Result{Summary: "a total is cut off",
		More: map[string]any{"add": []map[string]any{{"step": "gui-fix", "task": "coder", "instructions": "Widen the total column."}}}})
	if got := stepOf(t, ctx, r.ID, "gui-fix").Packet; got.AddedBy == nil || *got.AddedBy != another || got.AddedWhy != "a total is cut off" || got.Generation != 2 {
		t.Errorf("gui-fix added by a generation-1 step: by %v, why %q, generation %d; want %s, %q, 2", got.AddedBy, got.AddedWhy, got.Generation, another.ID, "a total is cut off")
	}

	// The request's steps record it, and its view shows it.
	g, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	recs := g.Request.Plan.Steps
	i := slices.IndexFunc(recs, func(s api.StepRecord) bool { return s.Step == "gui-test" })
	if i < 0 || recs[i].AddedBy == nil || *recs[i].AddedBy != session || recs[i].AddedWhy != "implemented" || recs[i].Generation != 1 {
		t.Fatalf("the gui-test record in the request's steps: %+v; want added by %s for %q, generation 1", recs, session.ID, "implemented")
	}
	if j := slices.IndexFunc(recs, func(s api.StepRecord) bool { return strings.HasSuffix(s.Step, "/implement") }); j < 0 || recs[j].Generation != 0 || recs[j].AddedBy != nil {
		t.Errorf("the implement record (a recipe step) says it was added: %+v", recs)
	}
	if panel := requestPanel(t, ctx, r.ID); !strings.Contains(panel, "Added beyond the recipe by session-a, generation 1: implemented") ||
		!strings.Contains(panel, "gui-test — success (session-b) · added, generation 1") {
		t.Errorf("the request's steps do not show who added gui-test, why and its generation: %s", panel)
	}

	// A rejected validation's rework: added by who rejected, for the rejection's line.
	for _, step := range []string{"gui-fix", "gui-shot", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	pullAndReport(t, ctx, review.ID, another, schedapi.Result{Summary: "the rounding is wrong\nhalf-up, not banker's", Verdict: api.VerdictRejected})
	for _, s := range []string{"auto-review (rework)", "module-update (rework)"} {
		if got := stepOf(t, ctx, r.ID, s).Packet; got.AddedBy == nil || *got.AddedBy != another || got.AddedWhy != "the rounding is wrong" || got.Generation != 1 {
			t.Errorf("%s: by %v, why %q, generation %d; want %s, %q, 1", s, got.AddedBy, got.AddedWhy, got.Generation, another.ID, "the rounding is wrong")
		}
	}
	again := stepOf(t, ctx, r.ID, "review")
	if again.ID == review.ID || again.Packet.AddedBy == nil || *again.Packet.AddedBy != another || again.Packet.Generation != 1 {
		t.Errorf("the review asked again: %s by %v, generation %d; want a new to-do added by %s, generation 1", again.ID, again.Packet.AddedBy, again.Packet.Generation, another.ID)
	}

	// Added by hand: by its caller, for the reason given, after the furthest step it follows.
	fix := stepOf(t, ctx, r.ID, "auto-review (rework)")
	added, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: r.ID, Step: "load-test", Task: "coder", Instructions: "Load-test the rounding.",
		After: []string{fix.ID}, Why: "the person wants a load test", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if got := added.Todo.Packet; got.AddedBy == nil || *got.AddedBy != person || got.AddedWhy != "the person wants a load test" || got.Generation != 2 {
		t.Errorf("load-test added by hand: by %v, why %q, generation %d; want %s, %q, 2", got.AddedBy, got.AddedWhy, got.Generation, person.ID, "the person wants a load test")
	}
	plain, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: r.ID, Step: "lint", Task: "coder", Instructions: "Lint it.", By: session})
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Todo.Packet; got.AddedBy == nil || *got.AddedBy != session || got.AddedWhy == "" || got.Generation != 1 {
		t.Errorf("lint added by hand after nothing: by %v, why %q, generation %d; want %s, a why, 1", got.AddedBy, got.AddedWhy, got.Generation, session.ID)
	}
}

// A blocked report's added steps say who added them and why, one generation on.
func TestABlockedReportsAddedStepSaysWhoAddedIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "which numbering: yearly or continuous?",
		More: map[string]any{"add": []map[string]any{{"step": "decide-numbering", "doer": "person", "interaction": "validate",
			"instructions": "Choose the invoice numbering: yearly or continuous."}}}})
	if got := stepOf(t, ctx, r.ID, "decide-numbering").Packet; got.AddedBy == nil || *got.AddedBy != session ||
		got.AddedWhy != "which numbering: yearly or continuous?" || got.Generation != 1 {
		t.Errorf("the step a blocked report added: by %v, why %q, generation %d; want %s, its line, 1", got.AddedBy, got.AddedWhy, got.Generation, session.ID)
	}
}
