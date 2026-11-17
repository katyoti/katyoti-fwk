// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// REMOVING A PERSON'S STEP CLOSES ITS INBOX QUESTION:
// a person's go asked in the Inbox and then taken out of the plan (work remove)
// leaves no pending entry for the person to answer: it is withdrawn with the
// step.
func TestRemovingAPersonsStepClosesItsInboxEntry(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	intake := stepOf(t, ctx, r.ID, api.StepIntake)
	// A person's go, right after its intake: asked in the Inbox once ready.
	added, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: r.ID, Step: "publish", After: []string{intake.ID}, Doer: api.DoerPerson,
		Interaction: api.InteractionValidate, Instructions: "Publish the release once it is built", By: person})
	if err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, intake.ID, session, schedapi.Result{Summary: "assessed"})
	var review api.WorkTodo
	for i := 0; i < 100; i++ {
		if review = stepOf(t, ctx, r.ID, "publish"); review.Packet.Ask != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if review.ID != added.Todo.ID || review.Packet.Ask == nil {
		t.Fatalf("the case needs the person's step asked: %+v", review)
	}
	review = askedBefore(t, ctx, review, "Publish the release", inboxapi.ExpectsVerdict)
	if _, err := workOf(t, ctx).Remove(ctx, api.RemoveWorkInput{ID: review.ID, Note: "the person reviews it with the other GUI walks", By: person}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		q, err := inbox.Get(ctx, inboxapi.GetInput{ID: review.Packet.AskedIn})
		if err == nil && q.Entry.Status != "" && q.Entry.Status != inboxapi.StatusPending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("5s after its step was removed, the review's Inbox entry is %+v (%v); want it no longer pending", q.Entry, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
