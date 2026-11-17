// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A BLOCKED REPORT STAYS BLOCKED, AND THE STEPS IT ADDS ARE PLANNED
// : a to-do reported blocked is offered to no agent
// until it is retried, whatever settles meanwhile; a blocked report that adds a
// person's step plans it - in the person's queue - and the blocked to-do waits
// on it, back in the queue once it has ended.
func TestABlockedToDoStaysBlockedUntilRetried(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing"})
	td := stepOf(t, ctx, r.ID, api.StepIntake)
	pullAndReport(t, ctx, td.ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "the numbering is unclear"})

	// Other work settles the workspace meanwhile.
	other := file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing"})
	pullAndReport(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "intake done"})

	if got := stepOf(t, ctx, r.ID, api.StepIntake); got.State != schedapi.StateWaiting || !strings.HasPrefix(got.Waits, schedapi.StatusBlocked) {
		t.Fatalf("the blocked intake after a settle: %s, waits %q; want waiting, blocked", got.State, got.Waits)
	}
	q, err := schedOf(t, ctx).Queue(ctx, schedapi.Filter{Request: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range q.Queue {
		if e.Todo.ID == td.ID && e.Takeable {
			t.Fatalf("the blocked intake is offered to agents: %+v", e)
		}
	}

	// Retried, it comes back.
	if _, err := workOf(t, ctx).Retry(ctx, api.RetryWorkInput{ID: td.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, r.ID, api.StepIntake); got.State != schedapi.StateReady {
		t.Fatalf("the retried intake: %s, waits %q; want ready", got.State, got.Waits)
	}
}

func TestABlockedReportPlansThePersonsStepItAdds(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing"})
	td := stepOf(t, ctx, r.ID, api.StepIntake)
	pullAndReport(t, ctx, td.ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "which numbering: yearly or continuous?",
		More: map[string]any{"add": []map[string]any{{"step": "decide-numbering", "doer": "person", "interaction": "validate",
			"instructions": "Choose the invoice numbering: yearly or continuous."}}}})

	decide := stepOf(t, ctx, r.ID, "decide-numbering")
	if decide.For != schedapi.ForPerson || decide.State != schedapi.StateReady {
		t.Fatalf("the person's step the blocked report added: for %s, %s (waits %q); want the person's, ready", decide.For, decide.State, decide.Waits)
	}
	found := false
	for _, it := range personQueue(t, ctx, "").Items {
		if it.Todo == decide.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the person's step %s is not in « À faire »", decide.ID)
	}
	blocked := stepOf(t, ctx, r.ID, api.StepIntake)
	if blocked.State != schedapi.StateWaiting || !slices.Contains(blocked.After, decide.ID) {
		t.Fatalf("the blocked intake: %s after %v (waits %q); want waiting on %s", blocked.State, blocked.After, blocked.Waits, decide.ID)
	}

	// The person decides: the intake comes back to the queue.
	pullAndReport(t, ctx, decide.ID, person, schedapi.Result{Summary: "yearly", Verdict: api.VerdictApproved})
	if got := stepOf(t, ctx, r.ID, api.StepIntake); got.State != schedapi.StateReady {
		t.Fatalf("the intake once the person decided: %s, waits %q; want ready", got.State, got.Waits)
	}
}
