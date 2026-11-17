// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : assessing an increment answers - the case seen on
// : its intake, held by the worker that assesses it
// sets kind demand, P0 and an estimate while the increment already holds work.
// The call answers within seconds, and the work the increment holds is pulled
// at the increment's priority (an agent keeps the one it carries, RQ-266).
func TestAssessAnIncrementAnswers(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid",
		Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Increment.ID
	// The work it holds: a feature it targets, with its build past its intake.
	f := want(t, ctx, cwm, "feature", "Reminders are sent")
	taken(t, ctx, cwm, id, f)
	accepted(t, ctx, cwm, f)
	b := file(t, ctx, req, api.FileInput{On: f, Kind: api.KindBuild, Priority: "P3", By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	runIncrement(t, ctx, id)

	// Its intake, held by the worker that assesses it.
	intake := stepOf(t, ctx, id, api.StepIntake)
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: intake.ID, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the increment's intake: %v %+v", err, got)
	}

	done := make(chan error, 1)
	go func() {
		_, err := req.Assess(ctx, api.AssessInput{ID: id, Kind: api.KindDemand, Priority: "P0", Estimate: 40, By: session})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("assess the increment: %v", err)
		}
	case <-time.After(eventuallyWithin):
		t.Fatalf("assessing an increment did not answer within %s", eventuallyWithin)
	}

	got, err := inc.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	p := got.Increment.Priority
	if p == "" {
		t.Fatal("the increment carries no priority after its assessment")
	}
	work, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range work.Todos {
		if td.Packet.Phase != api.PhaseRequests && td.Priority != p {
			t.Errorf("the build's %s: priority %s; want %s, its increment's", td.Packet.Step, td.Priority, p)
		}
	}
}
