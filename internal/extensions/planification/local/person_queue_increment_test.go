// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT'S APPROVE STEP IS IN « À FAIRE » - a
// run increment's proposal, reconciled, waits on the person's approval: the
// person's queue lists it as one validate item, about the increment, until
// it is approved.
func TestAnIncrementsApproveIsInThePersonQueue(t *testing.T) {
	t.Parallel()
	t.Run("alone", func(t *testing.T) { approveListed(t, false) })
	t.Run("after a running increment, across modules", func(t *testing.T) { approveListed(t, true) })
}

func approveListed(t *testing.T, after bool) {
	ctx, cwm, req := world(t)
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	inc := increments(t, ctx)
	module := "billing"
	if after {
		module = "" // sys-planification, like an increment across modules
	}
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: module, Stream: streamIn(t, ctx, module), Title: "Invoices get paid",
		Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	in := out.Increment
	if after {
		first, err := inc.Create(ctx, api.CreateIncrementInput{Stream: streamIn(t, ctx, ""), Title: "Invoices are sent", Goals: []string{"My invoices leave"}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inc.Run(ctx, api.IncrementRunInput{ID: first.Increment.ID, By: person}); err != nil {
			t.Fatal(err)
		}
		if _, err := req.Depend(ctx, api.DependInput{ID: in.ID, On: first.Increment.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, in.ID, api.StepIntake); st.ID != "" {
		pullAndReport(t, ctx, st.ID, session, schedapi.Result{Summary: "an increment's proposal"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, in.ID, "analyse").ID, session, schedapi.Result{Summary: "analysed"})
	pullAndReport(t, ctx, stepOf(t, ctx, in.ID, api.StepReconcile).ID, session, schedapi.Result{Summary: "reconciled",
		More: map[string]any{api.DeltaKey: []map[string]any{
			{"op": api.OpIncrementTarget, "object": g, "increment": in.ID, "for": "My customers pay me on time", "why": "the module already wants it"}}}})
	approve := stepOf(t, ctx, in.ID, "approve")
	if approve.State != schedapi.StateReady || approve.For != schedapi.ForPerson {
		t.Fatalf("the approve step is %s for %s; want ready for the person", approve.State, approve.For)
	}
	var found []api.PersonItem
	for _, it := range personQueue(t, ctx, "").Items {
		if it.Todo == approve.ID {
			found = append(found, it)
		}
	}
	if len(found) != 1 {
		t.Fatalf("« À faire » lists the increment's approve step %d times; want once", len(found))
	}
	if it := found[0]; it.Action != api.ActionValidate || it.Request != in.ID {
		t.Errorf("its item asks %q about %q; want validate about %s", it.Action, it.Request, in.ID)
	}
	if q := personQueue(t, ctx, in.ID); len(itemsOf(q, in.ID)) == 0 {
		t.Error("the increment's own « À faire » does not list its approve step")
	}
	// Its detail says the proposal awaits the approval - and, once approved,
	// its apply, never the approval again.
	proposal := func() string {
		d, err := inc.Detail(ctx, api.IncrementDetailInput{ID: in.ID})
		if err != nil {
			t.Fatal(err)
		}
		return d.Proposal
	}
	if p := proposal(); !strings.Contains(p, "await your approval") {
		t.Errorf("before the approval its proposal says %q; want it awaiting your approval", p)
	}
	pullAndReport(t, ctx, approve.ID, person, schedapi.Result{Summary: "ok", Verdict: api.VerdictApproved})
	if p := proposal(); !strings.Contains(p, "await their apply") || strings.Contains(p, "approval") {
		t.Errorf("approved, not applied: its proposal says %q; want it awaiting its apply, not an approval", p)
	}
	for _, it := range personQueue(t, ctx, "").Items {
		if it.Todo == approve.ID {
			t.Error("« À faire » still lists the approve step once approved")
		}
	}
}
