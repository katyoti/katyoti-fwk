// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// failPastTheCap fails an agent's step until its failure is the person's:
// past the cap of agent rounds, each failure retried by an agent before
func failPastTheCap(t *testing.T, ctx context.Context, id, summary string) {
	t.Helper()
	for i := 0; i <= plancore.MaxAgentRounds; i++ {
		if i > 0 {
			if _, err := workOf(t, ctx).Retry(ctx, api.RetryWorkInput{ID: id, By: session}); err != nil {
				t.Fatal(err)
			}
		}
		pullAndReport(t, ctx, id, session, schedapi.Result{Status: schedapi.StatusFail, Summary: summary})
	}
}

// AN AGENT'S FAILED CHECK HAS ITS REWORK PLANNED AT ONCE:
// an auto-review an agent fails is not left for the person to retry - the
// implementation is done again with the failure as feedback, then the
// auto-review asked again, what came after waiting on it; nothing of it is in
// « À faire ». Past the cap of agent rounds, the failure is the person's.
func TestAFailedAutoReviewPlansItsRework(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	failed := stepOf(t, ctx, r.ID, "auto-review")
	pullAndReport(t, ctx, failed.ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "TestRounding fails"})

	impl := stepOf(t, ctx, r.ID, "implement (rework)")
	if impl.State != schedapi.StateReady || impl.Packet.Params["feedback"] == nil {
		t.Errorf("the implementation done again: %s, feedback %v; want ready with the failure as feedback", impl.State, impl.Packet.Params["feedback"])
	}
	again := stepOf(t, ctx, r.ID, "auto-review")
	if again.ID == failed.ID || !slices.Contains(again.After, impl.ID) {
		t.Errorf("the auto-review asked again: %s after %v; want a new step after %s", again.ID, again.After, impl.ID)
	}
	if mu := stepOf(t, ctx, r.ID, "module-update"); !slices.Contains(mu.After, again.ID) || slices.Contains(mu.After, failed.ID) {
		t.Errorf("the module update comes after %v; want after the auto-review asked again", mu.After)
	}
	if items := itemsOf(personQueue(t, ctx, ""), r.ID); len(items) != 0 {
		t.Errorf("« À faire » carries %s for an agent's failed check: %+v", actionsOf(items), items)
	}

	// Rounds 2 and 3: the second rework is planned, the third failure is the person's.
	pullAndReport(t, ctx, impl.ID, session, schedapi.Result{Summary: "fixed"})
	pullAndReport(t, ctx, again.ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "TestRounding still fails"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement (rework) (rework)").ID, session, schedapi.Result{Summary: "fixed again"})
	last := stepOf(t, ctx, r.ID, "auto-review")
	pullAndReport(t, ctx, last.ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "TestRounding fails a third time"})
	if got := stepOf(t, ctx, r.ID, "auto-review"); got.ID != last.ID || got.State != schedapi.StateWaiting {
		t.Fatalf("past the cap the auto-review is %s %s; want %s left failed", got.ID, got.State, last.ID)
	}
	if got := actionsOf(itemsOf(personQueue(t, ctx, ""), r.ID)); got != api.ActionUnblock {
		t.Errorf("past the cap « À faire » carries %q; want one unblock", got)
	}
}

// A FAILED CHECK WITH NOTHING TO REDO STILL REACHES THE PERSON PAST THE CAP
// : an auto-review an agent fails with no agent work
// before it to do again plans no rework, so the request's rounds never grow;
// its own failures count, and past the cap the failure is the person's.
func TestAFailedCheckWithNothingToRedoIsThePersonsPastTheCap(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	check := cutAfter(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID)

	pullAndReport(t, ctx, check, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "TestRounding fails"})
	if got := steps(t, ctx, r.ID); slices.ContainsFunc(got, func(s string) bool { return strings.HasSuffix(s, "(rework)") }) {
		t.Fatalf("a check with nothing to redo planned work again: %v", got)
	}
	if items := itemsOf(personQueue(t, ctx, ""), r.ID); len(items) != 0 {
		t.Errorf("under the cap « À faire » carries %s for an agent's failed check: %+v", actionsOf(items), items)
	}

	for range plancore.MaxAgentRounds {
		if _, err := workOf(t, ctx).Retry(ctx, api.RetryWorkInput{ID: check, By: session}); err != nil {
			t.Fatal(err)
		}
		pullAndReport(t, ctx, check, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "TestRounding still fails"})
	}
	if got := stepOf(t, ctx, r.ID, "auto-review"); got.ID != check || got.Packet.Params["fail_past_cap"] != true {
		t.Errorf("after %d failures the check %s has params %v; want %s marked fail_past_cap", plancore.MaxAgentRounds+1, got.ID, got.Packet.Params, check)
	}
	if acts := actionsOf(itemsOf(personQueue(t, ctx, ""), r.ID)); acts != api.ActionUnblock {
		t.Errorf("past the cap « À faire » carries %q; want one unblock", acts)
	}
}
