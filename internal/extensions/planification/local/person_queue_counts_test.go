// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « À FAIRE », ITS RAIL COUNT AND PLANIFICATION'S « NEEDS YOU » COUNT THE SAME
// ITEMS - ONLY WHAT A PERSON ALONE MOVES: on one state
// holding an item of each kind - a person's answer and validation, an
// incident, a step failed past the cap of agent rounds, a hand test, a
// decision, an Inbox approval, an item the person held - and work that is not
// the person's - an agent's step failed once (an agent retries it), a
// deliverable whose GUI walk waits in an open GUI batch, agent work ready or
// held -, the page's items, the rail's count (PersonQueueCount), the deck's
// Needs you tile and its Needs tab's badge are one number, and neither the
// failed-once step nor the batched walk is in it.
func TestThePageTheRailAndTheDeckCountOnlyWhatThePersonMoves(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	// Before the K1-K6 world, whose held work holds billing. A hand test:
	// its review approved, its by-hand deliverable left.
	hand := file(t, ctx, req, api.FileInput{Title: "Archive invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen in the archive", ByHand: "Open the archive; the invoice is there."}}})
	approve := func(id string) {
		t.Helper()
		toReview(t, ctx, id)
		if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, id, "review").ID,
			Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
			t.Fatal(err)
		}
	}
	approve(hand.ID)
	// A walk not yet run: its end refused on a deliverable whose live GUI
	// walk an open batch request lists.
	const walk = "bash app/ktw-gui/frontend/tests/live/invoice-screen.sh"
	file(t, ctx, req, api.FileInput{Title: "Run the GUI batch", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "the invoice screen walk passes", Check: walk}}})
	batched := file(t, ctx, req, api.FileInput{Title: "Show the invoice screen", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "the invoice screen walk passes", Check: walk}}})
	approve(batched.ID)
	if st := planState(t, ctx, batched.ID); st != api.PlanBlocked {
		t.Fatalf("the batched request's plan is %s; want blocked on its walk not run", st)
	}
	w := fillOneReaderWorld(t, ctx, req)
	decision := openDecision(t, ctx, cwm, "Keep the old format?", true)
	held := openDecision(t, ctx, cwm, "Keep the old paper?", true)
	if _, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "Confirm the release", RequestedBy: "gate:ready-to-release", Expects: inboxapi.ExpectsVerdict}); err != nil {
		t.Fatal(err)
	}
	q := personQueue(t, ctx, "")
	for _, it := range q.Items {
		if it.Object == held && it.Held == nil {
			if _, err := req.Hold(ctx, api.HoldInput{Key: it.Key, Reason: "after the audit", By: person}); err != nil {
				t.Fatal(err)
			}
		}
	}

	q = personQueue(t, ctx, "")
	page := 0
	for _, it := range q.Items {
		if it.Held == nil && it.Action != api.ActionNotice && it.Action != api.ActionConversation {
			page++
		}
	}
	for _, id := range []string{w.Answer, w.Act, w.Incident, w.Failed, hand.ID, decision} {
		if len(itemsOf(q, id)) == 0 {
			t.Errorf("%s waits on the person but « À faire » carries nothing for it", id)
		}
	}
	for _, id := range []string{w.FailedOnce, batched.ID, w.Ready, w.Held} {
		if items := itemsOf(q, id); len(items) != 0 {
			t.Errorf("%s is not the person's to move, yet « À faire » carries %s: %+v", id, actionsOf(items), items)
		}
	}
	if q.Held != 1 {
		t.Errorf("« À faire » holds %d items on hold; want the one the person held", q.Held)
	}

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	rail, err := views.PersonQueueCount(ctx, api.PersonQueueInput{})
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(ctx, api.DeckInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	tile, badge := -1, -1
	for _, tab := range tree["tabs"].([]any) {
		tab := tab.(map[string]any)
		switch tab["id"] {
		case "overview":
			if n := findNode(tab, func(n map[string]any) bool { return n["data-view"] == "deck-needs-tile" }); n != nil {
				v, _ := n["value"].(float64)
				tile = int(v)
			}
		case "needs":
			v, _ := tab["badge"].(float64)
			badge = int(v)
		}
	}
	if page != q.Count || rail.Count != page || tile != page || badge != page {
		t.Errorf("the page lists %d items (its count %d), the rail counts %d, the Needs you tile %d, the Needs tab's badge %d; want one number",
			page, q.Count, rail.Count, tile, badge)
	}
}
