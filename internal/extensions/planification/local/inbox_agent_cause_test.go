// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN AGENT'S SESSION NEVER ANSWERS A PERSON'S INBOX ENTRY — a person's review
// asked in the inbox, answered or dismissed through the inbox from an agent's
// session (its cause), is refused whatever by says: the entry stays pending,
// the person's to-do is not reported, the request does not move. The person
// answering it, under their own cause, ends it as before — recorded as them.
func TestAnAgentSessionNeverAnswersAPersonsInboxEntry(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := file(t, ctx, req, api.FileInput{Title: "Rename the total", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson, Estimate: 1})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Depth: api.DepthQuick, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "renamed"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	review := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict)
	asked := review.Packet.AskedIn

	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	_, err = inbox.Answer(agent, inboxapi.AnswerInput{ID: asked, Decision: inboxapi.StatusApproved, Note: "good", By: person.ID})
	if err == nil {
		// The gap: say what followed the accepted answer.
		time.Sleep(2 * time.Second)
		got, _ := req.Get(ctx, api.GetInput{ID: r.ID})
		q, _ := inbox.Get(ctx, inboxapi.GetInput{ID: asked})
		t.Fatalf("an agent's session answered the person's review %s by %q: accepted (entry %s, resolved by %q); the request is now %s",
			asked, person.ID, q.Entry.Status, q.Entry.ResolvedBy, got.Request.State)
	}
	if _, err := inbox.Dismiss(agent, inboxapi.DismissInput{ID: asked}); err == nil {
		t.Fatalf("an agent's session dismissed the person's review %s: accepted", asked)
	}
	q, err := inbox.Get(ctx, inboxapi.GetInput{ID: asked})
	if err != nil {
		t.Fatal(err)
	}
	if q.Entry.Status != inboxapi.StatusPending || q.Entry.ResolvedBy != "" {
		t.Fatalf("after the refused answers the entry is %q, resolved by %q; want it pending", q.Entry.Status, q.Entry.ResolvedBy)
	}
	if again := stepOf(t, ctx, r.ID, "review"); again.ID != review.ID || again.State != schedapi.StateReady {
		t.Fatalf("after the refused answers the review is %s %s; want %s still ready", again.ID, again.State, review.ID)
	}

	// The person answers, under their own cause: recorded as them.
	own := cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: person.ID})
	out, err := inbox.Answer(own, inboxapi.AnswerInput{ID: asked, Note: "good"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Request.ResolvedBy != person.ID {
		t.Errorf("the person's answer is resolved by %q; want %q (the call's cause)", out.Request.ResolvedBy, person.ID)
	}
	slowly(t, "the request done from the person's approval", func() (string, bool) {
		got, err := req.Get(ctx, api.GetInput{ID: r.ID})
		return fmt.Sprint(got.Request.State, err), err == nil && got.Request.State == api.StateDone
	})
}
