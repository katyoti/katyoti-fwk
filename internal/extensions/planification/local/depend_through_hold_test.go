// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REVIEW NEVER WAITS ON THE WORK IT HOLDS: a review
// a trigger raises holds the to-dos of the work about its object (held_by), so
// that work cannot end before the review has; ordering the review after that
// work closes a cycle - refused, naming it, as a depends-on cycle is. An edge
// laid by hand all the same is named a cycle by the blockers check.
func TestAReviewOrderedAfterTheWorkItHoldsIsACycle(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	g := want(t, ctx, cwm, "goal", "Invoices are sent")
	work := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild, About: g})
	pullAndReport(t, ctx, stepOf(t, ctx, work.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	raised, err := triggersOf(t, ctx).Raise(ctx, api.RaiseInput{Source: api.RequestSource{Kind: api.SourceTrigger, Name: "goal-changed", Object: g, Change: "its body changed"},
		Kind: api.KindReview, Title: "Review the work on the changed goal", Holds: true})
	if err != nil || !raised.Filed || raised.Held == 0 {
		t.Fatalf("raised %+v, %v; want the work's to-dos held by the review", raised, err)
	}
	review := raised.Request

	_, err = req.Depend(ctx, api.DependInput{ID: review, On: work.ID})
	if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "held") {
		t.Fatalf("the review ordered after the work it holds: %v; want refused, naming the cycle through the hold", err)
	}

	// The same edge laid by hand: the check names the cycle, never a plain wait.
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: review, To: work.ID, Kind: cwmapi.EdgeType(api.EdgeDependsOn)}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, review, api.StepIntake).ID, session, schedapi.Result{Summary: "a review"})
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	for _, id := range []string{work.ID, review} {
		if b := blockerOf(t, out, id); b.Kind != api.BlockerIncident || b.Cause != api.CauseCycle {
			t.Errorf("%s: %s %s (%s); want an incident, a cycle", id, b.Kind, b.Cause, b.Reason)
		}
	}
}
