// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// askedBefore plants an Inbox entry asking the step, as askPerson filed one
// before IN-039 (retires it): the routers that answer such an
// entry still follow it.
func askedBefore(t *testing.T, ctx context.Context, td api.WorkTodo, title, expects string) api.WorkTodo {
	t.Helper()
	q, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: title, RequestedBy: "planification:" + td.Request + "/" + td.Packet.Step,
		BlocksTask: true, About: td.Request, Expects: expects})
	if err != nil {
		t.Fatal(err)
	}
	td.Work["asked_in"] = q.Request.ID // the owner's packet, as askPerson wrote it
	delete(td.Work, "ask")
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: td.Todo}); err != nil {
		t.Fatal(err)
	}
	td.Packet.AskedIn, td.Packet.Ask = q.Request.ID, nil
	return td
}

// A review dismissed in the Inbox is no rejection — nothing says what to
// change: its to-do is blocked, saying the person dismissed it, and nothing is
// done again. The question it was asked in expects a verdict.
func TestADismissedReviewBlocksAndIsNotDoneAgain(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict)
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := inbox.Get(ctx, inboxapi.GetInput{ID: review.Packet.AskedIn})
	if err != nil || q.Entry.Expects != inboxapi.ExpectsVerdict {
		t.Fatalf("the review's question: %+v, %v; want it to expect a verdict", q.Entry, err)
	}
	if _, err := inbox.Dismiss(ctx, inboxapi.DismissInput{ID: review.Packet.AskedIn}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := stepOf(t, ctx, r.ID, "review")
		if got.State == schedapi.StateWaiting && strings.HasPrefix(got.Waits, schedapi.StatusBlocked) {
			if !strings.Contains(got.Waits, "dismissed") {
				t.Errorf("the dismissed review waits %q; want it to say the person dismissed it", got.Waits)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("5s after it was dismissed: the review is %s (%q), steps %v; want it blocked", got.State, got.Waits, steps(t, ctx, r.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if left := steps(t, ctx, r.ID); slices.ContainsFunc(left, func(s string) bool { return strings.HasPrefix(s, "implement") }) {
		t.Fatalf("a dismissed review: %v scheduled; want nothing done again", left)
	}
}

// AN ANSWER GIVEN WHILE NOTHING LISTENED IS NEVER LOST — a review whose
// question was answered with no event reaching planification (a hub
// restarting) is collected by the next settle, as the event would have.
func TestAnAnswerMissedWhileNothingListenedIsCollectedAtTheNextSettle(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	inbox := inboxOf(t, ctx)
	// The answer lands where nothing waits for it yet - its event wakes no
	// one - then the review turns out to have been asked there.
	q, err := inbox.File(ctx, inboxapi.FileInput{Title: "Validate the result", RequestedBy: "planification:" + r.ID + "/review", Expects: inboxapi.ExpectsVerdict})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: q.Request.ID, By: "jdoe", Note: "it sends them"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	review = stepOf(t, ctx, r.ID, "review")
	review.Work["asked_in"] = q.Request.ID // the owner's packet, as askPerson writes it
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: review.Todo}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, r.ID, "review"); st.State != schedapi.StateReady || st.Packet.AskedIn != q.Request.ID {
		t.Fatalf("before the next settle the review is %s, asked in %s; want it still ready, asked in %s (its answer missed)", st.State, st.Packet.AskedIn, q.Request.ID)
	}
	// Any settle: a dependency already ended settles the request again.
	d := file(t, ctx, req, api.FileInput{Title: "Done before", Module: "billing", Kind: api.KindFix})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: d.ID, Outcome: api.PlanDropped, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Depend(ctx, api.DependInput{ID: r.ID, On: d.ID}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for slices.Contains(steps(t, ctx, r.ID), "review") {
		if time.Now().After(deadline) {
			t.Fatalf("5s after a settle the review is still scheduled (%v); want its missed answer collected", steps(t, ctx, r.ID))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
