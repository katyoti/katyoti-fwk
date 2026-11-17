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

// AN AGENT REVIEWS A BUILD UNLESS A PERSON IS ASKED - a
// build's review is the reviewer's: it answers a verdict, rejected has the
// work done again with its note, approved ends the request. A person reviews
// where the request asks it (at filing, or later: the review not started
// becomes theirs, and back), where its release's stream asks it, or where a
// deliverable only a person can test is still open.
func TestABuildIsReviewedByAnAgentUnlessAPersonIsAsked(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	reviewer := func(id string) (string, string) {
		t.Helper()
		r := stepOf(t, ctx, id, "review")
		return r.For, r.Packet.Task
	}

	// By default: the reviewer's, with a verdict.
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild})
	if who, task := reviewer(r.ID); who != schedapi.ForAgent || task != "reviewer" {
		t.Fatalf("a build's review: %s %q; want the reviewer's", who, task)
	}
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: review.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "looked", By: session}}); err == nil {
		t.Error("the reviewer's review collected without a verdict")
	}
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: review.ID, By: session}); err != nil {
		t.Fatal(err) // a refused report gives the to-do back
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "the totals are wrong", Verdict: api.VerdictRejected, By: session}}); err != nil {
		t.Fatal(err)
	}
	redo := stepOf(t, ctx, r.ID, "implement (rework)")
	if redo.Packet.Params["feedback"] != "the totals are wrong" {
		t.Fatalf("rejected by the reviewer: rework %+v; want the implementation done again with its note", redo.Packet)
	}
	for _, step := range []string{"implement (rework)", "auto-review (rework)", "module-update (rework)"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	again := stepOf(t, ctx, r.ID, "review")
	if again.For != schedapi.ForAgent {
		t.Errorf("the review asked again: %s; want the reviewer's again", again.For)
	}
	pullAndReport(t, ctx, again.ID, session, schedapi.Result{Summary: "good", Verdict: api.VerdictApproved})
	if got, _ := req.Get(ctx, api.GetInput{ID: r.ID}); got.Request.State != api.StateDone {
		t.Errorf("approved by the reviewer: %s; want done", got.Request.State)
	}

	// Asked at filing, or later - and back.
	asked := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	if who, _ := reviewer(asked.ID); who != schedapi.ForPerson {
		t.Errorf("review: person at filing: %s; want a person's", who)
	}
	later := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: later.ID, Review: api.ReviewPerson, By: person}); err != nil {
		t.Fatal(err)
	}
	if who, _ := reviewer(later.ID); who != schedapi.ForPerson {
		t.Errorf("review: person set later: %s; want the review not started a person's", who)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: later.ID, Review: api.ReviewAgent, By: person}); err != nil {
		t.Fatal(err)
	}
	if who, task := reviewer(later.ID); who != schedapi.ForAgent || task != "reviewer" {
		t.Errorf("review: agent set again: %s %q; want the reviewer's", who, task)
	}

	// Asked by the stream its release is in.
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Regulated", StreamFields: api.StreamFields{ReleaseMode: api.ModeManual, Review: api.ReviewPerson}})
	if err != nil {
		t.Fatal(err)
	}
	rl := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.0", Stream: st.Stream.ID})
	in := increment(t, ctx, "Audited invoices")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: rl.ID}); err != nil {
		t.Fatal(err)
	}
	uc := usecase(t, ctx, cwm, "Keep an audit trail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	audited := file(t, ctx, req, api.FileInput{Title: "Write the audit trail", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: audited.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	if who, _ := reviewer(audited.ID); who != schedapi.ForPerson {
		t.Errorf("its stream asks a person: %s; want a person's review", who)
	}

	// A deliverable only a person can test.
	byHand := file(t, ctx, req, api.FileInput{Title: "Print a sample", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "The print looks right", ByHand: "print it and look: no agent sees paper"}}})
	if who, _ := reviewer(byHand.ID); who != schedapi.ForPerson {
		t.Errorf("a by-hand deliverable: %s; want a person's review", who)
	}
	// ... once an agent has tried to test it: an auto-test comes before that review.
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, byHand.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	at := stepOf(t, ctx, byHand.ID, api.StepAutoTest)
	if titles := at.Packet.Params["deliverables"]; at.ID == "" || !slices.Contains(toStrings(titles), "The print looks right") || !slices.Contains(stepOf(t, ctx, byHand.ID, "review").After, at.ID) {
		t.Errorf("a by-hand deliverable before its review: auto-test %+v; want it tried by an agent first, the review after", at.Packet)
	}
}

func toStrings(v any) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		out = x
	case []any:
		for _, e := range x {
			out = append(out, fmt.Sprint(e))
		}
	}
	return out
}

// A FIX IS VERIFIED BY THE REVIEWER TOO - its verify
// step answers a verdict: rejected, the fix is done again with its note;
// approved, the request ends - no person's step on the way.
func TestAFixIsVerifiedByTheReviewer(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Totals are rounded wrong", Module: "billing", Kind: api.KindFix})
	verify := stepOf(t, ctx, r.ID, "verify")
	if verify.For != schedapi.ForAgent || verify.Packet.Task != "reviewer" || verify.Packet.Interaction != api.InteractionValidate {
		t.Fatalf("a fix's verify: %s %q %q; want the reviewer's validation", verify.For, verify.Packet.Task, verify.Packet.Interaction)
	}
	for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "verify").ID, session, schedapi.Result{Summary: "half-cents still round up", Verdict: api.VerdictRejected})
	if redo := stepOf(t, ctx, r.ID, "implement (rework)"); redo.Packet.Params["feedback"] != "half-cents still round up" {
		t.Fatalf("rejected by the reviewer: %+v; want the fix done again with its note", redo.Packet)
	}
	for _, step := range []string{"implement (rework)", "auto-review (rework)", "module-update (rework)"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "verify").ID, session, schedapi.Result{Summary: "closed", Verdict: api.VerdictApproved})
	if got, _ := req.Get(ctx, api.GetInput{ID: r.ID}); got.Request.State != api.StateDone {
		t.Errorf("approved by the reviewer: %s; want done", got.Request.State)
	}
}

// THE PERSON APPROVES AN INCREMENT'S DELIVERY - once
// the last of its work ends, the person is asked to approve its delivery on
// its pane; rejected, the note becomes a fix inside the increment and the
// approval is asked again once that fix ends; approved, the increment is done.
func TestThePersonApprovesAnIncrementsDelivery(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	work := func(title string) api.Request {
		t.Helper()
		r := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindFix, Increment: in.ID})
		for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
			pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
		}
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "verify").ID, session, schedapi.Result{Summary: "closed", Verdict: api.VerdictApproved})
		return r
	}
	approval := func() api.WorkTodo {
		t.Helper()
		reviewDelivery(t, ctx, in.ID, nil) // the agent's delivery review first
		a := stepOf(t, ctx, in.ID, "approve-delivery")
		if a.For != schedapi.ForPerson || a.Packet.Template != "increment-accept" || a.Packet.Interaction != api.InteractionValidate {
			t.Fatalf("the delivery's approval: %+v; want the person's validation (increment-accept)", a)
		}
		return a
	}
	work("Totals are rounded wrong")
	a := approval()
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: a.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "The reminders are not mailed", Details: "goal 2 is not met", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementOpen {
		t.Fatalf("its delivery sent back: %s; want it open", got.Increment.State)
	}
	list, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var sentBack string
	for _, r := range list.Requests {
		if r.Title == "The reminders are not mailed" && r.Kind == api.KindFix {
			sentBack = r.ID
		}
	}
	if sentBack == "" {
		t.Fatalf("the person's note was filed as no fix: %+v", list.Requests)
	}
	for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, sentBack, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, sentBack, "verify").ID, session, schedapi.Result{Summary: "mailed", Verdict: api.VerdictApproved})
	again := approval()
	if again.ID == a.ID {
		t.Fatal("the approval was not asked again once the fix ended")
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: again.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "delivered", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementDone {
		t.Errorf("its delivery approved: %s; want done", got.Increment.State)
	}
}

// AN APPROVED DELIVERY IS ALWAYS COLLECTED - the
// person's approval is recorded and its step archived whatever the increment
// came to after the approval was asked: closed done by its verb meanwhile, the
// plan ends and nothing fails (no done-to-done move); work that joined it
// meanwhile holds it open - the approval's plan ends and the delivery is asked
// again once that work ends.
func TestAnApprovedDeliveryIsAlwaysCollected(t *testing.T) {
	t.Parallel()
	deliver := func(t *testing.T) (context.Context, api.RequestService, api.Increment, api.WorkTodo) {
		ctx, _, req := world(t)
		in := increment(t, ctx, "Invoices reach their customers")
		workIn(t, ctx, req, in.ID, "Totals are rounded wrong")
		reviewDelivery(t, ctx, in.ID, nil)
		return ctx, req, in, stepOf(t, ctx, in.ID, "approve-delivery")
	}
	approve := func(t *testing.T, ctx context.Context, a api.WorkTodo) {
		t.Helper()
		out, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: a.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
			Summary: "delivered", Verdict: api.VerdictApproved, By: person}})
		if err != nil {
			t.Fatalf("the approval was not collected: %v", err)
		}
		if !strings.HasPrefix(out.Note, "collected") {
			t.Fatalf("the approval: %q; want it collected", out.Note)
		}
	}
	settled := func(t *testing.T, ctx context.Context, req api.RequestService, in string) {
		t.Helper()
		open, err := schedOf(t, ctx).List(ctx, schedapi.ListInput{Request: in})
		if err != nil {
			t.Fatal(err)
		}
		if len(open.Todos) > 0 {
			t.Fatalf("its approval collected, %d to-do(s) left: %+v", len(open.Todos), open.Todos[0])
		}
		got, err := req.Get(ctx, api.GetInput{ID: in})
		if err != nil {
			t.Fatal(err)
		}
		if got.Request.State != api.StateDone {
			t.Fatalf("its approval collected, its plan is %q; want done", got.Request.State)
		}
	}
	t.Run("closed done by its verb before the approval is collected", func(t *testing.T) {
		ctx, req, in, a := deliver(t)
		if _, err := increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Note: "closed for the person"}); err != nil {
			t.Fatal(err)
		}
		approve(t, ctx, a)
		settled(t, ctx, req, in.ID)
		if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementDone {
			t.Errorf("its delivery approved after it was closed: %s; want done", got.Increment.State)
		}
	})
	t.Run("work joined it after its delivery was asked", func(t *testing.T) {
		ctx, req, in, a := deliver(t)
		late := file(t, ctx, req, api.FileInput{Title: "Reminders are not mailed", Module: "billing", Kind: api.KindFix, Increment: in.ID})
		approve(t, ctx, a)
		settled(t, ctx, req, in.ID)
		if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementOpen {
			t.Fatalf("approved while %s is open: %s; want the increment open", late.ID, got.Increment.State)
		}
		workFix(t, ctx, late.ID)
		reviewDelivery(t, ctx, in.ID, nil)
		again := stepOf(t, ctx, in.ID, "approve-delivery")
		if again.ID == a.ID {
			t.Fatal("the delivery was not asked again once the late work ended")
		}
		approve(t, ctx, again)
		if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementDone {
			t.Errorf("its delivery approved again: %s; want done", got.Increment.State)
		}
	})
}

// workIn files a fix in an increment and works it to its end.
func workIn(t *testing.T, ctx context.Context, req api.RequestService, inc, title string) api.Request {
	t.Helper()
	r := file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Kind: api.KindFix, Increment: inc})
	workFix(t, ctx, r.ID)
	return r
}
