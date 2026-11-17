// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// NAMED GATES, planification's side: a move a gate refused plans its
// resolution — the request that does the work, the approve human task a person
// answers — and is tried again once they end.

// waitOf reads the gate-wait an object carries.
func waitOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) (api.GateWaitFields, bool) {
	t.Helper()
	module, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	w, ok, _ := cwmapi.DecodeCapability[api.GateWaitFields](got.Object, api.CapGateWait)
	return w, ok
}

// lifeOf is an object's lifecycle state.
func lifeOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) string {
	t.Helper()
	module, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := got.Object.Lifecycle()
	return l.State
}

// resolvers are the requests resolving an object (its inbound resolves edges).
func resolvers(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	t.Helper()
	out, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id, Kind: cwmapi.EdgeType(api.EdgeResolves), Direction: cwmapi.DirectionIn})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range out.Edges {
		ids = append(ids, e.From)
	}
	return ids
}

// asks are the open human tasks asking a person to confirm a gate's
// requirement on an object: their to-dos.
func asks(t *testing.T, ctx context.Context, _ cwmapi.CWMService, id string) []string {
	t.Helper()
	open, err := schedOf(t, ctx).List(ctx, schedapi.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, td := range open.Todos {
		ask, ok := inboxapi.AskOf(td)
		if !ok || ask.About != id || td.Result != nil {
			continue
		}
		for _, item := range ask.Satisfies {
			if c, ok := gatesapi.ParseConfirmationItem(item); ok && c.Object == id {
				ids = append(ids, td.ID)
				break
			}
		}
	}
	return ids
}

// askOf is what a gate's human task asks.
func askOf(t *testing.T, ctx context.Context, todo string) inboxapi.HumanTask {
	t.Helper()
	got, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: todo})
	if err != nil {
		t.Fatal(err)
	}
	ask, ok := inboxapi.AskOf(got.Todo)
	if !ok {
		t.Fatalf("%s is no human task: %+v", todo, got.Todo.Work)
	}
	return ask
}

// answerAsk is a person's answer to a gate's human task: a report of its to-do.
func answerAsk(t *testing.T, ctx context.Context, todo, verdict, note string) {
	t.Helper()
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: todo, Result: schedapi.Result{
		Status: schedapi.StatusSuccess, Verdict: verdict, Summary: verdict, Answer: note, By: person}}); err != nil {
		t.Fatal(err)
	}
}

// noGateEntry: no gate files an inbox approval-request any more.
func noGateEntry(t *testing.T, ctx context.Context) {
	t.Helper()
	tr, err := inboxOf(t, ctx).Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tr.Entries {
		if strings.HasPrefix(e.RequestedBy, "planification:gate:") {
			t.Fatalf("a gate filed an inbox entry: %+v; want its human task only", e)
		}
	}
}

func inboxOf(t *testing.T, ctx context.Context) inboxapi.InboxService {
	t.Helper()
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return inbox
}

// A release refused for its change log gets the request that writes it; once
// that is done, a person validates it in the inbox; approved, the confirmation
// is recorded and the release ships.
func TestARefusedReleasePlansItsChangeLog(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err == nil {
		t.Fatal("released with no change log confirmed")
	}
	work := eventually(t, "change-log request", func() (string, bool) {
		rs := resolvers(t, ctx, cwm, r.ID)
		if _, ok := waitOf(t, ctx, cwm, r.ID); !ok || len(rs) != 1 {
			return "", false
		}
		return rs[0], true
	})
	// Filed, then its intake planned: read it once planned.
	var got api.GetOutput
	eventually(t, "the change-log request planned", func() (string, bool) {
		g, err := req.Get(ctx, api.GetInput{ID: work})
		got = g
		return "", err == nil && g.Request.State == api.PlanReady
	})
	if q := got.Request; q.Resolves != r.ID || q.GateRequirement != "ready-to-release/changelog-updated" || q.Kind != "maintain" || !strings.Contains(q.Title, "change log") || q.State != api.PlanReady {
		t.Fatalf("the request filed = %+v; want a maintain request resolving %s, its intake planned", q, r.ID)
	}
	if s := got.Request.Source; s == nil || s.Kind != api.SourceGate || s.Name != "ready-to-release/changelog-updated" || s.Object != r.ID || !strings.Contains(s.Change, "refused") {
		t.Errorf("its source = %+v; want the gate's refusal, on the release", s)
	}
	if w, _ := waitOf(t, ctx, cwm, r.ID); w.Gate != api.GateReadyToRelease || w.To != api.ReleaseReleased || w.By != person {
		t.Fatalf("the wait = %+v; want ready-to-release, to released, by %v", w, person)
	}
	// Refused again while the work is open: nothing more is filed.
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err == nil {
		t.Fatal("released while its change-log work is open")
	}
	planlocal.Settle(ctx, 5*time.Second)
	// The release plan shows the checklist and what the release waits on.
	out, err := views(t, ctx).ReleasePlan(ctx, api.ViewInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	contains(t, text(t, out, "plan/"+r.ID), "## Gates\n\n**Ready to release:**\n\n- ⬜ The change log lists its user-visible changes — waits for a person's confirmation",
		"**Waiting on a gate:**\n\n- 1.4.0 → released, ready-to-release: changelog-updated `"+work+"` _(ready)_")

	walk(t, ctx, cwm, work, api.PlanReady, api.StateDone)
	ask := eventually(t, "change-log validation", func() (string, bool) {
		if a := asks(t, ctx, cwm, r.ID); len(a) == 1 {
			return a[0], true
		}
		return "", false
	})
	if rs := resolvers(t, ctx, cwm, r.ID); len(rs) != 1 {
		t.Fatalf("requests resolving %s = %v; want the one (filed once)", r.ID, rs)
	}
	// The confirmation is one approve human task about the release, satisfying
	// the requirement for this wait, a person's to answer - no inbox entry.
	q := askOf(t, ctx, ask)
	w, _ := waitOf(t, ctx, cwm, r.ID)
	if q.Kind != inboxapi.KindApprove || q.About != r.ID || !strings.Contains(q.Question, r.ID) || q.AnswerableBy != inboxapi.AnswerablePerson ||
		len(q.Satisfies) != 1 || q.Satisfies[0] != gatesapi.ConfirmationItem(r.ID, api.GateReadyToRelease, "changelog-updated", w.Since) {
		t.Fatalf("the confirmation asked = %+v; want an approve human task about %s satisfying ready-to-release/changelog-updated for this wait", q, r.ID)
	}
	noGateEntry(t, ctx)
	answerAsk(t, ctx, ask, "approved", "listed on the 01/10 page")
	eventually(t, "the release shipped", func() (string, bool) {
		return "", lifeOf(t, ctx, cwm, r.ID) == api.ReleaseReleased
	})
	planlocal.Settle(ctx, 5*time.Second) // the planner takes the wait off once the move passed
	if _, waiting := waitOf(t, ctx, cwm, r.ID); waiting {
		t.Error("the release still waits once released")
	}
	chk, err := gatesOf(t, ctx).Check(ctx, gatesapi.CheckInput{ID: r.ID, Gate: api.GateReadyToRelease})
	if err != nil || !chk.Checklist.Passes || chk.Checklist.Items[0].ConfirmedBy == nil || chk.Checklist.Items[0].ConfirmedBy.By.ID != "jdoe" ||
		chk.Checklist.Items[0].ConfirmedBy.Note != "listed on the 01/10 page" {
		t.Fatalf("checklist = %+v, %v; want changelog-updated confirmed by jdoe, with his note", chk.Checklist, err)
	}
}

// specified files a feature that wants a use case and moves it to specified.
func specified(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) string {
	t.Helper()
	uc := usecase(t, ctx, cwm, "A mail is sent for each invoice")
	f := want(t, ctx, cwm, "feature", "Send by mail", cwmapi.ObjectEdge{Kind: "wants", To: uc})
	accepted(t, ctx, cwm, f)
	if err := move(ctx, cwm, f, "specified", intake); err != nil {
		t.Fatal(err)
	}
	return f
}

// A person's requirement with no work is asked in the inbox at once; the
// approval moves the feature on, as the actor who asked.
func TestAChallengeIsAskedAndTheFeatureMovesOn(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	f := specified(t, ctx, cwm)
	if err := move(ctx, cwm, f, "ready", intake); err == nil {
		t.Fatal("ready before a person challenged it")
	}
	ask := eventually(t, "challenge validation", func() (string, bool) {
		if a := asks(t, ctx, cwm, f); len(a) == 1 {
			return a[0], true
		}
		return "", false
	})
	if q := askOf(t, ctx, ask); !strings.HasPrefix(q.Title, "Challenge: ") {
		t.Errorf("question = %q; want the requirement's interaction (Challenge)", q.Title)
	}
	answerAsk(t, ctx, ask, "approved", "")
	eventually(t, "the feature ready", func() (string, bool) { return "", lifeOf(t, ctx, cwm, f) == "ready" })
}

// A person who refuses to confirm ends the wait: the move is not tried again.
func TestARefusedConfirmationEndsTheWait(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	f := specified(t, ctx, cwm)
	if err := move(ctx, cwm, f, "ready", intake); err == nil {
		t.Fatal("ready before a person challenged it")
	}
	ask := eventually(t, "challenge validation", func() (string, bool) {
		if a := asks(t, ctx, cwm, f); len(a) == 1 {
			return a[0], true
		}
		return "", false
	})
	answerAsk(t, ctx, ask, "rejected", "split it first")
	// The answer reaches the wait through the kept answer's event: wait for
	// the end, never a pause.
	eventually(t, "the wait ended after a person refused to confirm", func() (string, bool) {
		_, waiting := waitOf(t, ctx, cwm, f)
		return "", !waiting
	})
	if st := lifeOf(t, ctx, cwm, f); st != "specified" {
		t.Errorf("state = %s; want specified (the person refused)", st)
	}
	// Moved again: a new wait, a new question — the refused one belongs to the
	// earlier wait and does not end this one.
	if err := move(ctx, cwm, f, "ready", intake); err == nil {
		t.Fatal("ready before a person challenged it")
	}
	again := eventually(t, "a second challenge validation", func() (string, bool) {
		if a := asks(t, ctx, cwm, f); len(a) == 1 {
			return a[0], a[0] != ask
		}
		return "", false
	})
	answerAsk(t, ctx, again, "approved", "")
	eventually(t, "the feature ready", func() (string, bool) { return "", lifeOf(t, ctx, cwm, f) == "ready" })
}

// A move a person forces through a gate is shown on the release plan, with
// the requirements it passed unmet and the person's note.
func TestAForcedMoveIsShownOnTheReleasePlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	f := specified(t, ctx, cwm)
	planlocal.Settle(ctx, 5*time.Second) // the planner writes on the object after a refusal: CWM writes are not atomic (object-update-loses-concurrent-writes)
	if _, err := gatesOf(t, ctx).Confirm(ctx, gatesapi.ConfirmInput{ID: f, Gate: api.GateReadyToBuild, Requirement: "feature-challenged", By: person}); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"ready", "active"} {
		if err := move(ctx, cwm, f, to, intake); err != nil {
			t.Fatal(err)
		}
	}
	// A release holds it through an increment.
	in := increment(t, ctx, "Invoices reach their customers by mail")
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := increments(t, ctx).ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	// Only a person forces a gate, with a note.
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", By: intake, Force: true, Note: "the agent insists"}); err == nil {
		t.Fatal("an agent forced the ready-to-test gate")
	}
	// The planner's answer to the refusal lands first (a queued event Settle
	// cannot see): the wait and its requests, then the person forces.
	eventually(t, "the refusal planned", func() (string, bool) {
		_, waiting := waitOf(t, ctx, cwm, f)
		return "", waiting && len(resolvers(t, ctx, cwm, f)) >= 2
	})
	planlocal.Settle(ctx, 5*time.Second) // CWM writes are not atomic (object-update-loses-concurrent-writes)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", By: person, Force: true, Note: "verified by hand"}); err != nil {
		t.Fatal(err)
	}
	// The journal records the move as it hears of it: wait for the page to
	// show it, never a pause.
	eventually(t, "the forced move on the release plan", func() (string, bool) {
		out, err := views(t, ctx).ReleasePlan(ctx, api.ViewInput{ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		page := text(t, out, "plan/"+r.ID)
		return page, strings.Contains(page, "**Forced through a gate:**\n\n- Send by mail: active → done, jdoe ") &&
			strings.Contains(page, "verification-declared, verification-green — verified by hand")
	})
	// A planner's write behind the person's never undoes the move.
	if st := lifeOf(t, ctx, cwm, f); st != "done" {
		t.Errorf("state = %s; want done (forced by a person)", st)
	}
	// The move passed: the wait ends, and the requests filed to resolve it are
	// dropped — they have no purpose left.
	rs := resolvers(t, ctx, cwm, f)
	if len(rs) == 0 {
		t.Fatal("no request was filed for the agent's refused move")
	}
	eventually(t, "the wait ended and its requests dropped", func() (string, bool) {
		if _, waiting := waitOf(t, ctx, cwm, f); waiting {
			return "", false
		}
		for _, id := range rs {
			if got, err := req.Get(ctx, api.GetInput{ID: id}); err != nil || got.Request.State != api.StateDropped {
				return "", false
			}
		}
		return "", true
	})
}

// entriesAsked are the inbox validations an object is asked-in.
func entriesAsked(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	t.Helper()
	module, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range got.Object.Edges {
		if e.Kind == api.EdgeAskedIn {
			ids = append(ids, e.To)
		}
	}
	return ids
}
