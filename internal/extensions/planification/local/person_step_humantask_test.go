// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// TestAPersonStepIsOneRecordOnceIN039IsOn (decision
// ): with IN-039's switch on, a recipe's person step is asked as a
// human task on its to-do - kind, question, about the request, settling the
// step - and no inbox entry is filed; « À faire » answers it by the to-do's
// report, which ends the step. With the switch off it is asked in the inbox
// as before.
func TestAPersonStepIsOneRecordOnceIN039IsOn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)

	review := stepOf(t, ctx, r.ID, "review")
	ask := review.Packet.Ask
	if ask == nil || ask.Kind != inboxapi.KindApprove || ask.About != r.ID || len(ask.Satisfies) != 1 || ask.Satisfies[0] != r.ID+"/review" ||
		ask.AnswerableBy != inboxapi.AnswerablePerson || review.Packet.AskedIn != "" {
		t.Fatalf("the review = %+v ask %+v; want an approve human task about the request, settling its step, and no inbox entry", review.Packet, ask)
	}
	tri, err := inboxOf(t, ctx).Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tri.Entries {
		if strings.Contains(e.RequestedBy, r.ID) {
			t.Fatalf("an inbox entry was filed for the step: %+v", e)
		}
	}
	items := itemsOf(personQueue(t, ctx, ""), r.ID)
	if len(items) != 1 || items[0].Todo != review.ID || items[0].Ask != "" {
		t.Fatalf("« À faire » = %+v; want the review, answered by its to-do", items)
	}
	page, err := views(t, ctx).PersonQueuePage(ctx, api.PersonQueueInput{Target: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if !strings.Contains(string(raw), "ktw-agents:todos/report") || !strings.Contains(string(raw), review.ID) {
		t.Error("the card does not answer the review by its to-do's report")
	}

	pullAndReport(t, ctx, review.ID, person, schedapi.Result{Verdict: api.VerdictApproved, Summary: "fine"})
	if items := itemsOf(personQueue(t, ctx, ""), r.ID); len(items) != 0 {
		t.Errorf("after the person's report « À faire » still asks: %+v", items)
	}
}

// TestAPersonStepTakesItsKindsLabelFromTheContributedKinds
// : a person step asked as a human task takes its
// kind's label and defaults from the contributed kinds of questions
// , never from a list in planification's code.
func TestAPersonStepTakesItsKindsLabelFromTheContributedKinds(t *testing.T) {
	// Not in parallel: it replaces the process's reader of the kinds.
	was := inboxapi.QueryAskKinds
	inboxapi.QueryAskKinds = func(context.Context) ([]inboxapi.AskKind, error) {
		return []inboxapi.AskKind{{Name: inboxapi.KindApprove, Label: "Sign it off", Requires: "request:approve"}}, nil
	}
	defer func() { inboxapi.QueryAskKinds = was }()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, r.ID)

	review := stepOf(t, ctx, r.ID, "review")
	if review.Packet.Label != "Sign it off" || review.Packet.Ask == nil || review.Packet.Ask.Label != "Sign it off" || review.Packet.Ask.Requires != "request:approve" {
		t.Fatalf("the review = label %q ask %+v; want the contributed kind's label and its default permission code", review.Packet.Label, review.Packet.Ask)
	}
}
