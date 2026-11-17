// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// Re-prioritising a planned request moves its to-dos not started to the new
// priority, so the queue follows; one a worker holds keeps its own.
func TestReprioritisingARequestMovesItsTodos(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Priority: "P2", By: person})
	held := stepOf(t, ctx, r.ID, api.StepIntake)
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: held.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Priority: "P0", By: intake}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"analyse", "implement", "auto-review", "module-update", "review"} {
		if got := stepOf(t, ctx, r.ID, s).Priority; got != "P0" {
			t.Errorf("%s: %s; want P0, the request's new priority", s, got)
		}
	}
	if got := stepOf(t, ctx, r.ID, api.StepIntake).Priority; got != "P2" {
		t.Errorf("the pulled intake: %s; want it to keep P2", got)
	}
	// Planned again with a priority: the same.
	if _, err := plansOf(t, ctx).Plan(ctx, api.PlanNextInput{ID: r.ID, Priority: "P1", By: person}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, r.ID, "implement").Priority; got != "P1" {
		t.Errorf("after plans plan --priority: %s; want P1", got)
	}
}
