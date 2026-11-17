// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A request split at its intake keeps no step of its own: its parts carry its
// work, and planning it again is refused.
func TestARequestSplitAtIntakeKeepsNoStepOfItsOwn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild, By: person})
	in := stepOf(t, ctx, r.ID, "intake")
	s := schedOf(t, ctx)
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: in.ID, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the intake: %v %+v", err, got)
	}
	split, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: session, Note: "billing owns it", Parts: []api.SplitPart{
		{Module: "billing", Title: "Invoices carry the mail address"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: in.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "split", By: session}}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Errorf("the split request still holds %v; want none of its own steps", left)
	}
	if part := stepOf(t, ctx, split.Parts[0].ID, "intake"); part.State != schedapi.StateReady {
		t.Errorf("the part's intake is %s; want ready", part.State)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, By: session, Again: true}); err == nil {
		t.Error("planning a split request again was accepted; want it refused")
	}
}

// A to-do queued for a request that already ended leaves the queue at the
// next settle, never handed out.
func TestAnEndedRequestsLeftoverStepsLeaveTheQueue(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild, By: person})
	other := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	leftover := stepOf(t, ctx, r.ID, "analyse").Todo
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: r.ID, To: api.PlanDropped, Capability: api.CapPlan, By: person, Note: "dropped"}); err != nil {
		t.Fatal(err)
	}
	// A to-do queued for it all the same (as a bulk planning once did).
	leftover.ID, leftover.State, leftover.Waits, leftover.After = "", schedapi.StateReady, "", nil
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: leftover}); err != nil {
		t.Fatal(err)
	}
	if len(steps(t, ctx, r.ID)) == 0 {
		t.Fatal("setup: the ended request should hold the leftover before a settle")
	}
	// Any collect settles the queue: the other request's intake.
	pullAndReport(t, ctx, stepOf(t, ctx, other.ID, "intake").ID, session, schedapi.Result{})
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Errorf("the ended request still holds %v; want its to-dos gone", left)
	}
}

// Split after its intake ran, a request's own steps leave the queue at once,
// and the request ends when its parts have.
func TestASplitAfterIntakeLeavesNothingTakeableAndEndsWithItsParts(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Mail and print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "intake").ID, session, schedapi.Result{})
	if left := steps(t, ctx, r.ID); len(left) == 0 {
		t.Fatal("setup: the request should hold its next steps after its intake")
	}
	split, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: person, Note: "two parts", Parts: []api.SplitPart{
		{Module: "billing", Title: "Mail the invoices"}, {Module: "billing", Title: "Print the invoices"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Fatalf("right after the split it still holds %v; want none of its own steps", left)
	}
	for _, p := range split.Parts {
		if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: p.ID, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
			t.Fatal(err)
		}
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDropped {
		t.Errorf("its parts dropped, the split request is %s; want dropped with them", st)
	}
}

// A person's step already asked when its request ends leaves no question
// pending in the Inbox.
func TestAnEndedRequestsAskedQuestionIsClosed(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson, By: person})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{})
	}
	eventually(t, "the review asked", func() (string, bool) {
		rv := stepOf(t, ctx, r.ID, "review")
		return rv.ID, rv.Packet.Ask != nil
	})
	asked := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict).Packet.AskedIn
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its question closed", func() (string, bool) {
		q, err := inbox.Get(ctx, inboxapi.GetInput{ID: asked})
		if err != nil {
			return err.Error(), false
		}
		return q.Entry.Status, q.Entry.Status != inboxapi.StatusPending
	})
}

// An intake that changes the request's kind and splits it, then reports, is
// collected: nothing is planned again on the split request, and none of its
// steps stays behind.
func TestSplitAtIntakeWithAKindChangeIsCollected(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix and mail invoices", Module: "billing", Kind: api.KindBuild, By: person})
	in := stepOf(t, ctx, r.ID, "intake")
	s := schedOf(t, ctx)
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: in.ID, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the intake: %v %+v", err, got)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Kind: api.KindFix, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: session, Note: "billing owns it", Parts: []api.SplitPart{
		{Module: "billing", Title: "Fix the invoices"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: in.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fix, split", By: session}}); err != nil {
		t.Fatalf("the intake's report: %v; want it collected", err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Errorf("the split request still holds %v; want none of its own steps", left)
	}
}
