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

// A PERSON'S STEP ANSWERED ANOTHER WAY CLOSES ITS INBOX ENTRY — a review asked
// in the Inbox and answered in « À faire » or the request view (its to-do
// reported) leaves no validation pending: the entry is answered as the step
// was, approved or rejected with its note.
func TestAStepAnsweredAnotherWayClosesItsInboxEntry(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		title, verdict, want string
	}{
		{"Send invoices", api.VerdictApproved, inboxapi.StatusApproved},
		{"Print invoices", api.VerdictRejected, inboxapi.StatusRejected},
	} {
		r := file(t, ctx, req, api.FileInput{Title: c.title, Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
		for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
			pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
		}
		review := askedBefore(t, ctx, stepOf(t, ctx, r.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict)
		pullAndReport(t, ctx, review.ID, person, schedapi.Result{Verdict: c.verdict, Summary: "judged in the request view"})
		deadline := time.Now().Add(5 * time.Second)
		for {
			q, err := inbox.Get(ctx, inboxapi.GetInput{ID: review.Packet.AskedIn})
			if err == nil && q.Entry.Status == c.want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: 5s after the review was reported %s, its Inbox entry is %+v (%v); want it %s", c.title, c.verdict, q.Entry, err, c.want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
