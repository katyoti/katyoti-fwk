// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
)

// TestPlanificationsQuestionsAreInTheOnePlace: for what was
// asked before its questions asked through human tasks (an entry a step
// filed), and what no human task carries, planification contributes them to the
// inbox's « À faire » - each of its items as the kind of question it is, the
// entry a step filed carried by its item, the inbox's own entries left to the
// inbox (a gate's confirmation and a notice are not planification's to show),
// and each item's buttons as its « À faire » card has them.
func TestPlanificationsQuestionsAreInTheOnePlace(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	review := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, review.ID)
	askedBefore(t, ctx, stepOf(t, ctx, review.ID, "review"), "Validate the result", inboxapi.ExpectsVerdict)
	proposed := openDecision(t, ctx, cwm, "Keep the old format?", true)
	question, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "May I deploy?", RequestedBy: "run:r1", Expects: inboxapi.ExpectsText})
	if err != nil {
		t.Fatal(err)
	}

	src := plancore.WaitingSource()
	items, err := src.Items(ctx)
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	kinds := map[string]string{}
	for _, it := range items {
		if !strings.HasPrefix(it.Key, "planification:") || it.Kind == "" || it.Title == "" {
			t.Errorf("an item without its key, kind or title: %+v", it)
		}
		if it.About == question.Request.ID || it.Entry == question.Request.ID {
			t.Errorf("the inbox's own entry is planification's item: %+v", it)
		}
		kinds[it.About] = it.Kind
	}
	want := map[string]string{review.ID: inboxapi.KindApprove, proposed: inboxapi.KindDecide}
	for id, kind := range want {
		if kinds[id] != kind {
			t.Errorf("%s is asked as %q; want %q", id, kinds[id], kind)
		}
	}
	var rev inboxapi.WaitingItem
	for _, it := range items {
		if it.About == review.ID {
			rev = it
		}
	}
	if rev.Entry == "" || rev.Todo == "" {
		t.Fatalf("the review = %+v; want the entry its step filed and its to-do", rev)
	}
	if ctl := src.Controls(ctx, rev, "en"); len(ctl) == 0 {
		t.Error("the review has no controls; want its card's buttons")
	}
	if !src.Answers(inboxapi.Entry{ID: rev.Entry, RequestedBy: "planification:" + review.ID, Expects: inboxapi.ExpectsVerdict}) {
		t.Error("a step's entry is not planification's: it would show twice")
	}
	for _, e := range []inboxapi.Entry{
		{RequestedBy: "planification:gate:ready-to-release/notes@x", Expects: inboxapi.ExpectsVerdict},
		{RequestedBy: "planification:" + review.ID, Expects: inboxapi.ExpectsNotice},
		{RequestedBy: "run:r1", Expects: inboxapi.ExpectsText},
	} {
		if src.Answers(e) {
			t.Errorf("%+v is taken as planification's; a gate's confirmation, a notice and another's question are the inbox's", e)
		}
	}
	if k := plancore.KindOfItem(api.PersonItem{Action: api.ActionValidate, Wants: []api.Target{{ID: "x"}}}); k != inboxapi.KindAccept {
		t.Errorf("wants to accept are asked as %q; want accept", k)
	}
	if k := plancore.KindOfItem(api.PersonItem{Action: api.ActionTest}); k != inboxapi.KindTestByHand {
		t.Errorf("a hand test is asked as %q; want test-by-hand", k)
	}
}
