// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// AN APPLY IS COLLECTED TO ITS END BEFORE ITS REPORT IS ANSWERED
// (decision OD-017): planning the increment's work from
// the apply's collect runs no acceptance probe, so a verification that never
// answers holds nothing up; and an apply left reported (its collect cut) is
// collected again by the scheduler's tick.

// blockedVerification never answers until released: an acceptance probe that
// hangs.
type blockedVerification struct {
	verificationapi.VerificationService
	started chan<- struct{}
	release <-chan struct{}
}

func (v blockedVerification) Verify(ctx context.Context, _ verificationapi.VerifyInput) (verificationapi.VerifyOutput, error) {
	select {
	case v.started <- struct{}{}:
	default:
	}
	select {
	case <-v.release:
		return verificationapi.VerifyOutput{}, nil
	case <-ctx.Done():
		return verificationapi.VerifyOutput{}, ctx.Err()
	}
}

// demandAtApply files a demand on an increment, reconciles it into one
// feature and approves it: its apply is next.
func demandAtApply(t *testing.T) (ctx context.Context, s schedapi.TodoService, demand, feature, apply string) {
	t.Helper()
	ctx, cwm, requests := world(t)
	inc := increment(t, ctx, "Customers receive invoices by mail")
	d := file(t, ctx, requests, api.FileInput{Title: "Invoices are also mailed", Module: "billing", Kind: api.KindDemand, About: inc.ID, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s = schedOf(t, ctx)
	reconcile := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	if pulled, err := s.Pull(ctx, schedapi.PullInput{ID: reconcile, By: session}); err != nil || pulled.Todo == nil {
		t.Fatalf("pull reconcile: %+v, %v", pulled, err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: reconcile, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session,
		More: map[string]any{api.DeltaKey: []map[string]any{{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "why": "what the demand asks"}}}}}); err != nil {
		t.Fatal(err)
	}
	feature = want(t, ctx, cwm, "feature", "Invoices are mailed")
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	apply = stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	return ctx, s, d.ID, feature, apply
}

func TestAnApplyIsCollectedToItsEndWhileAnAcceptanceProbeHangs(t *testing.T) {
	t.Parallel()
	ctx, s, demand, feature, apply := demandAtApply(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	defer close(release)
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		blockedVerification{started: started, release: release})

	if pulled, err := s.Pull(ctx, schedapi.PullInput{ID: apply, By: session}); err != nil || pulled.Todo == nil {
		t.Fatalf("pull apply: %+v, %v", pulled, err)
	}
	reported := make(chan error, 1)
	go func() {
		_, err := s.Report(ctx, schedapi.ReportInput{ID: apply, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "applied", By: session,
			More: map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": feature}}}}})
		reported <- err
	}()
	select {
	case err := <-reported:
		if err != nil {
			t.Fatalf("report apply: %v", err)
		}
	case <-time.After(eventuallyWithin):
		t.Fatal("the apply's report waited for an acceptance probe")
	}
	select {
	case <-started:
		t.Error("the apply's collect ran an acceptance probe")
	default:
	}
	// Answered means collected to its end: nothing is read later.
	got, err := s.Get(ctx, schedapi.GetInput{ID: apply})
	if err != nil || got.Todo.Archived == "" {
		t.Fatalf("the apply once answered: %+v, %v; want archived", got.Todo, err)
	}
	if st := planState(t, ctx, demand); st != api.PlanDone {
		t.Fatalf("the demand once its apply answered: %s, want done", st)
	}
	stepOf(t, ctx, feature, api.StepIntake) // the increment's work is planned
}

func TestATickCollectsAnApplyLeftReportedAndSaysWhyOneFails(t *testing.T) {
	t.Parallel()
	ctx, s, demand, feature, apply := demandAtApply(t)
	// The collect its report started was cut: the to-do is left reported,
	// half an hour ago.
	got, err := s.Get(ctx, schedapi.GetInput{ID: apply})
	if err != nil {
		t.Fatal(err)
	}
	left := got.Todo
	left.State, left.Claim = schedapi.StateReported, nil
	left.Result = &schedapi.Result{Status: schedapi.StatusSuccess, Summary: "applied", By: session,
		At:   time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339Nano),
		More: map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": feature}}}}
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: left}); err != nil {
		t.Fatal(err)
	}
	// And one whose collect fails: its request is not there.
	broken := left
	broken.ID, broken.Key, broken.Request, broken.Target = "", "", "billing.RQ-999", "billing.RQ-999"
	putBroken, err := s.Put(ctx, schedapi.PutInput{Todo: broken})
	if err != nil {
		t.Fatal(err)
	}

	rep, err := s.Tick(ctx, schedapi.TickInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rep.Collected, apply) {
		t.Fatalf("collected by the tick: %v; want %s", rep.Collected, apply)
	}
	if st := planState(t, ctx, demand); st != api.PlanDone {
		t.Errorf("the demand after the tick: %s, want done", st)
	}
	said := false
	for _, e := range rep.Errors {
		said = said || strings.Contains(e, putBroken.Todo.ID)
	}
	if !said {
		t.Errorf("the tick's errors %v do not name %s, whose collect failed", rep.Errors, putBroken.Todo.ID)
	}
}
