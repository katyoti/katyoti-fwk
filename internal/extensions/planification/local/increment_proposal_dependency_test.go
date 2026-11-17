// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT'S DEPENDENCY HOLDS BACK ONLY ITS BUILD, NEVER ITS PROPOSAL
// : B depends on A, A not even run. B runs: its
// proposal - analysed, reconciled, approved, applied - goes on at once,
// whatever A's own preparation; only the work B holds waits on A, from its
// build on (TestAnIncrementsPriorityAndDependenciesReachTheWorkItHolds).
func TestAnIncrementsProposalNeverWaitsOnTheIncrementItDependsOn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc := increments(t, ctx)
	create := func(title string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{title}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	a, b := create("Invoices are drafted"), create("Invoices reach their customers")
	if _, err := req.Depend(ctx, api.DependInput{ID: b, On: a}); err != nil {
		t.Fatal(err)
	}
	runIncrement(t, ctx, b)
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: b})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range got.Todos {
		if td.Packet.Phase == api.PhaseRequests {
			pullAndReport(t, ctx, td.ID, session, schedapi.Result{Summary: "an increment"})
		}
	}
	analyse := stepOf(t, ctx, b, "analyse")
	if analyse.State != schedapi.StateReady {
		t.Errorf("B's analyse: %s (%s); want ready - its proposal does not wait on %s", analyse.State, analyse.Waits, a)
	}
	if strings.Contains(analyse.Waits, a) {
		t.Errorf("B's analyse waits %q; want no wait on %s", analyse.Waits, a)
	}
}
