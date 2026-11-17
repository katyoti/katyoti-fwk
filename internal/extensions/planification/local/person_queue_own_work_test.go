// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « À FAIRE » ASKS THE PERSON ONLY FOR WHAT ONLY THE PERSON CAN MOVE
// : on one state holding
//   - a request the person blocked with their reason: their own hold, listed
//     under On hold, out of the count, taken back by resuming its plan;
//   - a failed hand test with agent work to redo: that work done again at
//     once, the plan resumed - no card until the person tests again;
//   - a check failing at the end with agent work to redo: the same;
//   - an end refused while work it depends on is open: a wait on that work;
//
// none of them counts. Past the cap of rounds, a failed hand test and a
// failed check are the person's again.
// setBlockKind gives a plan blocked through the block verb the kind its
// blocker would have given it: an end refused, a
// move refused - what the verb, a block by hand, never writes.
func setBlockKind(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, kind string) {
	t.Helper()
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	pf, _, err := cwmapi.DecodeCapability[api.PlanFields](got.Object, api.CapPlan)
	if err != nil {
		t.Fatal(err)
	}
	pf.Block = kind
	if err := got.Object.SetCapability(api.CapPlan, pf); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: got.Object}); err != nil {
		t.Fatal(err)
	}
}

func TestÀFaireAsksOnlyWhatOnlyThePersonMoves(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	plans := plansOf(t, ctx)
	approve := func(id string) {
		t.Helper()
		toReview(t, ctx, id)
		if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, id, "review").ID,
			Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
			t.Fatal(err)
		}
	}
	// reworkLands reports the agent steps of the round planned, one round.
	reworkLands := func(id string) {
		t.Helper()
		round := map[string]bool{}
		for _, td := range openTodos(t, ctx, id) {
			round[td.ID] = true
		}
		for range 20 {
			ready := ""
			for _, td := range openTodos(t, ctx, id) {
				if round[td.ID] && td.State == schedapi.StateReady && td.For != schedapi.ForPerson {
					ready = td.ID
					break
				}
			}
			if ready == "" {
				return
			}
			pullAndReport(t, ctx, ready, session, schedapi.Result{Summary: "done again"})
		}
		t.Fatalf("the rework of %s never ends: %v", id, steps(t, ctx, id))
	}

	// The person's own block.
	held := file(t, ctx, req, api.FileInput{Title: "Move invoices to the database", Module: "billing", Kind: api.KindBuild})
	const reason = "On hold by the storage decision: the database comes first"
	if _, err := plans.Block(ctx, api.BlockInput{ID: held.ID, Note: reason, By: person}); err != nil {
		t.Fatal(err)
	}
	// A failed hand test, its review approved before.
	hand := file(t, ctx, req, api.FileInput{Title: "Archive invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen in the archive", ByHand: "Open the archive; the invoice is there."}}})
	approve(hand.ID)
	if items := itemsOf(personQueue(t, ctx, ""), hand.ID); actionsOf(items) != api.ActionTest {
		t.Fatalf("before its test « À faire » carries %q for %s; want its hand test", actionsOf(items), hand.ID)
	}
	fail := func() {
		t.Helper()
		if _, err := req.Test(ctx, api.TestInput{ID: hand.ID, Deliverable: "seen in the archive", Result: api.TestFails, Note: "not there", By: person}); err != nil {
			t.Fatal(err)
		}
	}
	fail()
	// A check failing at the end.
	checked := file(t, ctx, req, api.FileInput{Title: "Total invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "the totals add up", Check: "exit 3"}}})
	approve(checked.ID)
	// An end refused while the work it depends on is open.
	first := file(t, ctx, req, api.FileInput{Title: "Store the totals", Module: "billing", Kind: api.KindBuild})
	waiting := file(t, ctx, req, api.FileInput{Title: "Report the totals", Module: "billing", Kind: api.KindBuild, DependsOn: []string{first.ID}})
	if _, err := plans.Block(ctx, api.BlockInput{ID: waiting.ID, By: person,
		Note: "its to-dos are done, but deliverable \"the report\": exit 3 failed"}); err != nil {
		t.Fatal(err)
	}
	setBlockKind(t, ctx, cwm, waiting.ID, api.BlockEndRefused)

	q := personQueue(t, ctx, "")
	items := itemsOf(q, held.ID)
	if len(items) != 1 || items[0].Held == nil || !items[0].HoldIsBlock || items[0].Held.Reason != reason || items[0].Held.By.ID != person.ID {
		t.Fatalf("the person's own block: %+v; want one item on hold, by them, with their reason, taken back by resuming", items)
	}
	for _, id := range []string{hand.ID, checked.ID, waiting.ID} {
		if got := itemsOf(q, id); len(got) != 0 {
			t.Errorf("%s is agent work or waits on work, yet « À faire » carries %s: %+v", id, actionsOf(got), got)
		}
	}
	if q.Count != 0 || q.Held != 1 {
		t.Errorf("« À faire » counts %d items, %d on hold; want none counted, the person's block on hold", q.Count, q.Held)
	}
	redo := stepOf(t, ctx, hand.ID, "implement (rework)")
	if redo.State != schedapi.StateReady || !strings.Contains(redo.Packet.Instructions, "not there") || planState(t, ctx, hand.ID) != api.PlanReady {
		t.Errorf("after the hand test failed: %s %s (%q), plan %s; want its implementation done again, ready, the failure as feedback, the plan resumed",
			redo.Packet.Step, redo.State, redo.Packet.Instructions, planState(t, ctx, hand.ID))
	}
	if redo := stepOf(t, ctx, checked.ID, "implement (rework)"); redo.State != schedapi.StateReady || !strings.Contains(redo.Packet.Instructions, "its check failed") {
		t.Errorf("after its check failed at its end: %s %s (%q); want its implementation done again with the check's output", redo.Packet.Step, redo.State, redo.Packet.Instructions)
	}

	// The held card's take-back resumes the plan: no hold entry to remove.
	var tree map[string]any
	if err := json.Unmarshal([]byte(queuePageOf(t, ctx, "held")), &tree); err != nil {
		t.Fatal(err)
	}
	back := findNode(tree, func(n map[string]any) bool { return n["id"] == "unhold" })
	if back == nil || back["uri"] != "ktw-agents:plans/plan" {
		t.Errorf("the held block's take-back: %v; want the plan resumed (ktw-agents:plans/plan)", back)
	}

	// Each lands and is refused again: the hand test is the person's to run
	// again; past the cap of rounds, a failure is the person's.
	for round := 1; round <= plancore.MaxAgentRounds; round++ {
		reworkLands(hand.ID)
		reworkLands(checked.ID)
		if got := actionsOf(itemsOf(personQueue(t, ctx, ""), hand.ID)); got != api.ActionTest {
			t.Fatalf("round %d: once the rework landed « À faire » carries %q for %s; want its hand test again", round, got, hand.ID)
		}
		if round < plancore.MaxAgentRounds {
			if got := itemsOf(personQueue(t, ctx, ""), checked.ID); len(got) != 0 {
				t.Errorf("round %d: under the cap « À faire » carries %s for the failed check", round, actionsOf(got))
			}
		}
		fail()
	}
	q = personQueue(t, ctx, "")
	if got := actionsOf(itemsOf(q, hand.ID)); got != api.ActionUnblock || planState(t, ctx, hand.ID) != api.PlanBlocked {
		t.Errorf("past the cap the failed hand test is %q, plan %s; want one unblock, the plan blocked", got, planState(t, ctx, hand.ID))
	}
	if got := itemsOf(q, checked.ID); len(got) != 1 || !got[0].Checks {
		t.Errorf("past the cap the failed check is %+v; want its check card", got)
	}
}
