// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A VALIDATE STEP'S REPORT TAKES A VERDICT, HOWEVER THE STEP WAS PLANNED
// : a review planned before an agent's review carried
// its interaction has none in its packet; reported
// with no verdict it is refused as one planned today is - never collected as
// an approval; approved it passes; rejected the work is done again.

// workedToReview files a build request and works it to its review: the
// review's to-do, its packet as planned before RQ-1080 when old.
func workedToReview(t *testing.T, title string, old bool) (context.Context, string, api.WorkTodo) {
	t.Helper()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	if old {
		delete(review.Work, "interaction")
		if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: review.Todo}); err != nil {
			t.Fatal(err)
		}
		if got := stepOf(t, ctx, r.ID, "review"); got.Packet.Interaction != "" {
			t.Fatalf("the review planned before RQ-1080 still asks %q", got.Packet.Interaction)
		}
	}
	return ctx, r.ID, stepOf(t, ctx, r.ID, "review")
}

func TestAValidateWithNoVerdictIsRefusedHoweverItWasPlanned(t *testing.T) {
	t.Parallel()
	for _, old := range []bool{false, true} {
		ctx, id, review := workedToReview(t, "Send invoices", old)
		s := schedOf(t, ctx)
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: review.ID, By: another}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
			Summary: "Rejected: the totals are wrong", By: another}})
		if err == nil || !strings.Contains(err.Error(), "a validate takes a verdict") {
			t.Fatalf("planned before RQ-1080 %v: a review reported with no verdict: %v; want it refused, saying « a validate takes a verdict »", old, err)
		}
		if st := planState(t, ctx, id); st == api.PlanDone {
			t.Fatalf("planned before RQ-1080 %v: a review with no verdict ended the request", old)
		}
		// Refused, it is the worker's again to report with a verdict.
		if got := stepOf(t, ctx, id, "review"); got.State != schedapi.StateClaimed && got.State != schedapi.StateReady {
			t.Errorf("planned before RQ-1080 %v: the refused review is %s; want it to be reported again", old, got.State)
		}
	}
}

func TestAnApprovedValidatePlannedBeforeItsInteractionPasses(t *testing.T) {
	t.Parallel()
	ctx, id, review := workedToReview(t, "Send invoices", true)
	pullAndReport(t, ctx, review.ID, another, schedapi.Result{Summary: "it sends them", Verdict: api.VerdictApproved})
	if st := planState(t, ctx, id); st != api.PlanDone {
		t.Fatalf("an approved review left the request %s; want done", st)
	}
}

func TestARejectedValidatePlannedBeforeItsInteractionSendsTheWorkBack(t *testing.T) {
	t.Parallel()
	ctx, id, review := workedToReview(t, "Send invoices", true)
	pullAndReport(t, ctx, review.ID, another, schedapi.Result{Summary: "the totals are wrong", Verdict: api.VerdictRejected})
	if st := planState(t, ctx, id); st == api.PlanDone {
		t.Fatalf("a rejected review ended the request")
	}
	if left := steps(t, ctx, id); !slices.Contains(left, "implement (rework)") {
		t.Fatalf("a rejected review: %v scheduled; want the implement done again", left)
	}
}
