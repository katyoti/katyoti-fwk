// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A WORKER ADDS AT MOST 8 ITEMS TO ONE REQUEST ON ITS OWN:
// steps its reports add and follow-up requests they file, counted from their
// provenance. Eight plan as before; a ninth is not planned - a person's
// validate step lists it; a person's own adds are not counted; approved, what
// waited is added (by the person, so uncounted).
func TestAWorkerAddsAtMostEightItemsToARequestOnItsOwn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	// Six steps and two follow-ups: eight, all planned.
	var adds []map[string]any
	for i := 1; i <= 6; i++ {
		adds = append(adds, map[string]any{"step": fmt.Sprintf("s%d", i), "task": "coder", "instructions": fmt.Sprintf("Do part %d.", i)})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "implemented in parts",
		More: map[string]any{"add": adds, "follow_ups": []map[string]any{
			{"title": "Round the credit notes", "module": "billing"},
			{"title": "Round the quotes", "module": "billing"},
		}}})
	for i := 1; i <= 6; i++ {
		stepOf(t, ctx, r.ID, fmt.Sprintf("s%d", i))
	}
	if steps := stepNames(t, ctx, r.ID); slices.Contains(steps, "validate-adds") {
		t.Fatalf("eight agent-added items asked a person's validation: %v", steps)
	}
	for _, title := range []string{"Round the credit notes", "Round the quotes"} {
		if !hasRequest(t, ctx, req, title) {
			t.Errorf("the follow-up %q within the cap was not filed", title)
		}
	}

	// A person's own add is not counted.
	if _, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: r.ID, Step: "load-test", Task: "coder", Instructions: "Load-test it.", By: person}); err != nil {
		t.Fatal(err)
	}

	// The ninth and tenth wait on a person's validate step listing them.
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "s1").ID, another, schedapi.Result{Summary: "part 1 done",
		More: map[string]any{"add": []map[string]any{{"step": "s7", "task": "coder", "instructions": "Do part 7."}},
			"follow_ups": []map[string]any{{"title": "Round the receipts", "module": "billing"}}}})
	steps := stepNames(t, ctx, r.ID)
	if slices.Contains(steps, "s7") {
		t.Errorf("a ninth agent-added step was planned: %v", steps)
	}
	if hasRequest(t, ctx, req, "Round the receipts") {
		t.Errorf("a follow-up past the cap was filed")
	}
	v := stepOf(t, ctx, r.ID, "validate-adds")
	if v.Packet.Doer != api.DoerPerson || v.Packet.Interaction != api.InteractionValidate ||
		!strings.Contains(v.Packet.Instructions, "step s7: Do part 7.") || !strings.Contains(v.Packet.Instructions, "follow-up request: Round the receipts") {
		t.Fatalf("the validate step past the cap: doer %s, interaction %s, instructions %q", v.Packet.Doer, v.Packet.Interaction, v.Packet.Instructions)
	}
	if v.Packet.AddedBy == nil || v.Packet.AddedBy.ID != "planification" {
		t.Errorf("the validate step is not planification's own: added by %v", v.Packet.AddedBy)
	}

	// Approved, what waited is added - by the person.
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: v.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "fine, add them", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if got := stepOf(t, ctx, r.ID, "s7").Packet; got.AddedBy == nil || *got.AddedBy != person {
		t.Errorf("s7 approved: added by %v, want the person", got.AddedBy)
	}
	if !hasRequest(t, ctx, req, "Round the receipts") {
		t.Errorf("the follow-up approved was not filed")
	}
}

// A blocked report past the cap: its steps wait on the person's validate
// step, which the blocked to-do then waits on.
func TestABlockedReportPastTheCapWaitsOnAPersonsValidation(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	var adds []map[string]any
	for i := 1; i <= 8; i++ {
		adds = append(adds, map[string]any{"step": fmt.Sprintf("s%d", i), "task": "coder", "instructions": fmt.Sprintf("Do part %d.", i)})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "implemented in parts", More: map[string]any{"add": adds}})
	s1 := stepOf(t, ctx, r.ID, "s1")
	pullAndReport(t, ctx, s1.ID, another, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "needs a fixture",
		More: map[string]any{"add": []map[string]any{{"step": "fixture", "task": "coder", "instructions": "Write the fixture."}}}})
	if slices.Contains(stepNames(t, ctx, r.ID), "fixture") {
		t.Errorf("a ninth agent-added step was planned on a blocked report")
	}
	v := stepOf(t, ctx, r.ID, "validate-adds")
	if again := stepOf(t, ctx, r.ID, "s1"); !slices.Contains(again.After, v.ID) {
		t.Errorf("the blocked to-do does not wait on the validate step: after %v", again.After)
	}
}

// A REJECTED VALIDATE-ADDS STEP DROPS WHAT WAITED PAST THE CAP
// : no waiting step planned, no waiting follow-up
// filed, no rework of the validate step, and what came after it released.
func TestARejectedValidateAddsDropsWhatWaitedPastTheCap(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Lighter invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	var adds []map[string]any
	for i := 1; i <= 8; i++ {
		adds = append(adds, map[string]any{"step": fmt.Sprintf("s%d", i), "task": "coder", "instructions": fmt.Sprintf("Do part %d.", i)})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "implemented in parts", More: map[string]any{"add": adds}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "s1").ID, another, schedapi.Result{Summary: "part 1 done",
		More: map[string]any{"add": []map[string]any{{"step": "s9", "task": "coder", "instructions": "Do part 9."}},
			"follow_ups": []map[string]any{{"title": "Lighten the receipts", "module": "billing"}}}})
	v := stepOf(t, ctx, r.ID, "validate-adds")
	if s2 := stepOf(t, ctx, r.ID, "s2"); !slices.Contains(s2.After, v.ID) || s2.State == schedapi.StateReady {
		t.Fatalf("what came after does not wait on the validate step: state %s, after %v", s2.State, s2.After)
	}

	// Rejected, what waited is dropped and what came after goes on.
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: v.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "no, enough parts", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	steps := stepNames(t, ctx, r.ID)
	if slices.Contains(steps, "s9") {
		t.Errorf("a step rejected past the cap was planned: %v", steps)
	}
	for _, s := range steps {
		if strings.HasSuffix(s, " (rework)") {
			t.Errorf("a rejected validate-adds step planned a rework %q: %v", s, steps)
		}
	}
	if hasRequest(t, ctx, req, "Lighten the receipts") {
		t.Errorf("a follow-up rejected past the cap was filed")
	}
	if s2 := stepOf(t, ctx, r.ID, "s2"); s2.State != schedapi.StateReady {
		t.Errorf("what came after the rejected validate step is not released: state %s, after %v", s2.State, s2.After)
	}
}

func stepNames(t *testing.T, ctx context.Context, request string) []string {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, td := range got.Todos {
		out = append(out, td.Packet.Step)
	}
	return out
}

func hasRequest(t *testing.T, ctx context.Context, req api.RequestService, title string) bool {
	t.Helper()
	got, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range got.Requests {
		if x.Title == title {
			return true
		}
	}
	return false
}
