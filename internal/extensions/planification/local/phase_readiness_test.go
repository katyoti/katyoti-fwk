// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// READINESS READS THE PHASE — a request depending on another: its intake
// never waits; its Preparation step waits only for the other's preparation;
// its Build step waits for the other's end.
func TestReadinessReadsThePhase(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild})
	b := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Depend(ctx, api.DependInput{ID: b.ID, On: a.ID}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, b.ID, api.StepIntake).State; st != schedapi.StateReady {
		t.Fatalf("its intake: %s; want ready (an intake never waits)", st)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "assessed"})
	if st := stepOf(t, ctx, b.ID, "analyse").State; st == schedapi.StateReady {
		t.Fatal("its analysis is ready before the other's preparation is done")
	}
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, a.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if st := stepOf(t, ctx, b.ID, "analyse").State; st != schedapi.StateReady {
		t.Fatalf("its analysis once the other's preparation is done: %s; want ready", st)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, "analyse").ID, session, schedapi.Result{Summary: "analysed"})
	if st := stepOf(t, ctx, b.ID, "implement").State; st == schedapi.StateReady {
		t.Fatal("its build is ready while the other is still being built")
	}
}
