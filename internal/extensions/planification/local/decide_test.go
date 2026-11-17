// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// DECIDING — a decide request on a decision is prepared, challenged, then
// chosen by the person on the decision itself: the decision's move to
// `applies` ends the person's step, the plan, and the step's question.

// decision opens a decision in billing and files a decide request on it.
func decision(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, req api.RequestService, title string) string {
	t.Helper()
	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: title}
	_ = o.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"}})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create the decision: %v", err)
	}
	if _, err := req.File(ctx, api.FileInput{On: out.Object.ID, Title: title, Kind: api.KindDecide, By: person}); err != nil {
		t.Fatalf("file the decide request: %v", err)
	}
	return out.Object.ID
}

// prepared works a decide request up to its person's step: intake, prepare,
// challenge (approved).
func prepared(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	for _, step := range []string{api.StepIntake, "prepare"} {
		pullAndReport(t, ctx, stepOf(t, ctx, id, step).ID, session, schedapi.Result{})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, id, "challenge").ID, session, schedapi.Result{Verdict: api.VerdictApproved, Summary: "the proposal holds"})
}

// The decide recipe's challenge is an agent's review that answers a verdict:
// rejected, the decision is prepared again with what to change and challenged
// again before the person's decide step is asked; approved, it goes on to
// that step.
func TestADecideChallengeRejectedPreparesTheDecisionAgain(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	od := decision(t, ctx, cwm, req, "Keep the old totals?")
	if c := stepOf(t, ctx, od, "challenge"); c.Packet.Interaction != api.InteractionChallenge || c.Packet.Doer == api.DoerPerson {
		t.Fatalf("the challenge step: doer %q, interaction %q; want an agent's challenge", c.Packet.Doer, c.Packet.Interaction)
	}
	for _, step := range []string{api.StepIntake, "prepare"} {
		pullAndReport(t, ctx, stepOf(t, ctx, od, step).ID, session, schedapi.Result{})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, od, "challenge").ID, session, schedapi.Result{Verdict: api.VerdictRejected, Summary: "the proposal ignores the ledger's rounding"})
	redo := stepOf(t, ctx, od, "prepare (rework)")
	if redo.State != schedapi.StateReady || !strings.Contains(fmt.Sprint(redo.Packet.Params["feedback"]), "ledger") {
		t.Errorf("prepare done again: state %s, params %v; want ready with the challenge's feedback", redo.State, redo.Packet.Params)
	}
	again := stepOf(t, ctx, od, "challenge")
	if again.State != schedapi.StateWaiting || !slices.Contains(again.After, redo.ID) {
		t.Errorf("the challenge asked again: state %s after %v; want it waiting on %s", again.State, again.After, redo.ID)
	}
	if d := stepOf(t, ctx, od, "decide"); d.State == schedapi.StateReady {
		t.Fatalf("the person's decide step is asked on a rejected proposal")
	}
	pullAndReport(t, ctx, redo.ID, session, schedapi.Result{Summary: "options re-prepared with the ledger's rounding"})
	pullAndReport(t, ctx, stepOf(t, ctx, od, "challenge").ID, session, schedapi.Result{Verdict: api.VerdictApproved, Summary: "the proposal holds"})
	eventually(t, "the decide step asked after the approved challenge", func() (string, bool) {
		d := stepOf(t, ctx, od, "decide")
		return d.State, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
}

// choose records the option a person chooses, as the decisions service does
// before its move.
func choose(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, option string, by cwmapi.Actor) {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	d := o.Capabilities["decision"]
	d["selected"] = []string{option}
	_ = o.SetCapability("decision", d)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, id, "applies", by); err != nil {
		t.Fatalf("decide %s: %v", id, err)
	}
}

// A decide request plans prepare, challenge, decide; the person's step ends
// on the decision's move, which ends the plan and closes its question.
func TestADecisionResolvedOnItsPanelEndsItsDecideStep(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	od := decision(t, ctx, cwm, req, "Keep the old totals?")
	if got := strings.Join(steps(t, ctx, od), ","); got != "intake,prepare,challenge,decide" {
		t.Fatalf("steps %s; want intake, prepare, challenge, decide", got)
	}
	if d := stepOf(t, ctx, od, "decide"); d.Packet.EndsOn != "applies" || d.Packet.Moves != "applies" {
		t.Fatalf("decide ends on %q, moves %q; want both applies", d.Packet.EndsOn, d.Packet.Moves)
	}
	prepared(t, ctx, od)
	eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, od, "decide")
		return d.ID, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	choose(t, ctx, cwm, od, "keep", person)
	eventually(t, "the decide request ended", func() (string, bool) {
		left := steps(t, ctx, od)
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: od})
		if err != nil {
			return err.Error(), false
		}
		f, _, _ := cwmapi.DecodeCapability[api.PlanFields](got.Object, api.CapPlan)
		return strings.Join(left, ",") + " / " + f.State, len(left) == 0 && f.State == api.PlanDone
	})
	done, err := req.Get(ctx, api.GetInput{ID: od})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(done.Request.Plan.Steps, func(s api.StepRecord) bool { return strings.HasSuffix(s.Step, "decide") })
	if i < 0 || done.Request.Plan.Steps[i].By == nil || *done.Request.Plan.Steps[i].By != person || !strings.Contains(done.Request.Plan.Steps[i].Result, "keep") {
		t.Errorf("the decide step's record %+v; want it done by the person, with the option", done.Request.Plan.Steps)
	}
}

// Decided before its step is asked, the decision ends that step when it
// becomes ready; an agent's move ends nothing a person owes.
func TestADecisionDecidedEarlyEndsItsStepAndAnAgentsMoveDoesNot(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	early := decision(t, ctx, cwm, req, "Round the totals?")
	choose(t, ctx, cwm, early, "drop", person)
	prepared(t, ctx, early)
	eventually(t, "the early decision's step ended", func() (string, bool) {
		left := steps(t, ctx, early)
		return strings.Join(left, ","), len(left) == 0
	})

	byAgent := decision(t, ctx, cwm, req, "Print the totals?")
	prepared(t, ctx, byAgent)
	eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, byAgent, "decide")
		return d.State, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	choose(t, ctx, cwm, byAgent, "keep", builder)
	planlocal.Settle(ctx, 5*time.Second)
	if d := stepOf(t, ctx, byAgent, "decide"); d.State != schedapi.StateReady {
		t.Errorf("after an agent's move the decide step is %s; want it still the person's, ready", d.State)
	}
}

// On a decision, the Request panel says where its plan stands and, for the
// person's step, links to choosing on the decision instead of a typed answer.
func TestTheRequestPanelOnADecisionLinksItsChoiceToTheDecision(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	od := decision(t, ctx, cwm, req, "Keep the old totals?")
	prepared(t, ctx, od)
	eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, od, "decide")
		return d.State, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Request(ctx, api.RequestViewInput{ID: od})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	if !strings.Contains(page, `"navigate":{"object":"`+od+`"`) || !strings.Contains(page, "Choose on "+od) {
		t.Errorf("the panel does not link the choice to %s: %s", od, page)
	}
	if strings.Contains(page, "AnswerTheQuestion") {
		t.Error("the panel offers a typed answer for a step the decision's move ends")
	}
}

// A request that depends on a decision goes on once the decision applies; a
// decided decision's plan ends without being asked to become done.
func TestARequestDependingOnADecisionGoesOnOnceItApplies(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	od := decision(t, ctx, cwm, req, "Round the totals?") // planned, as was
	r := file(t, ctx, req, api.FileInput{Title: "Print the totals", Module: "billing", Kind: api.KindBuild, DependsOn: []string{od}, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{})
	if a := stepOf(t, ctx, r.ID, "analyse"); a.State != schedapi.StateWaiting || !strings.Contains(a.Waits, od) {
		t.Fatalf("analyse is %s (%q); want it waiting on %s", a.State, a.Waits, od)
	}
	choose(t, ctx, cwm, od, "keep", person)
	eventually(t, "analyse released", func() (string, bool) {
		a := stepOf(t, ctx, r.ID, "analyse")
		return a.State + " " + a.Waits, a.State == schedapi.StateReady
	})

	// A review planned on a decision ends with a move to done its workflow
	// does not have: decided, it is skipped and the plan ends.
	o := cwmapi.Object{Type: "decision", Parent: "billing", Name: "Print the totals in cents?"}
	_ = o.SetCapability("decision", map[string]any{"options": []string{"keep", "drop"}})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatal(err)
	}
	od2 := made.Object.ID
	if _, err := req.File(ctx, api.FileInput{On: od2, Title: "Review the cents", Kind: api.KindReview, By: person}); err != nil {
		t.Fatal(err)
	}
	choose(t, ctx, cwm, od2, "keep", person)
	for _, step := range []string{api.StepIntake, "review"} {
		pullAndReport(t, ctx, stepOf(t, ctx, od2, step).ID, session, schedapi.Result{})
	}
	eventually(t, "the decided decision's plan ended", func() (string, bool) {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: od2})
		if err != nil {
			return err.Error(), false
		}
		f, _, _ := cwmapi.DecodeCapability[api.PlanFields](got.Object, api.CapPlan)
		return f.State + " " + f.Note, f.State == api.PlanDone
	})
}

// A person's decide step whose decision a person already took, the move's
// event missed (taken in another process, or while nothing listened): the
// queue no longer asks it, and the next settle ends it.
func TestADecideStepWhoseDecisionWasTakenEndsAtTheNextSettle(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	od := decision(t, ctx, cwm, req, "Keep the old totals?")
	prepared(t, ctx, od)
	asked := eventually(t, "the decide step asked", func() (string, bool) {
		d := stepOf(t, ctx, od, "decide")
		return d.ID, d.State == schedapi.StateReady && d.Packet.Ask != nil
	})
	stale := stepOf(t, ctx, od, "decide")
	choose(t, ctx, cwm, od, "keep", person)
	eventually(t, "the decide request ended", func() (string, bool) {
		return "", len(steps(t, ctx, od)) == 0
	})
	// Its step left behind on an open request, as the missed event leaves it.
	r := file(t, ctx, req, api.FileInput{Title: "Apply the totals", Module: "billing", Kind: api.KindBuild, By: person})
	other := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	left := stale.Todo
	left.ID, left.Request, left.Target, left.State, left.Waits, left.After, left.Claim = "", r.ID, r.ID, schedapi.StateReady, "", nil, nil
	left.Work["on"] = od
	if _, err := schedOf(t, ctx).Put(ctx, schedapi.PutInput{Todo: left}); err != nil {
		t.Fatal(err)
	}
	if got := itemsOf(personQueue(t, ctx, ""), r.ID); slices.ContainsFunc(got, func(it api.PersonItem) bool { return it.Action == api.ActionDecide }) {
		t.Errorf("the queue asks a decision already taken: %+v", got)
	}
	// Any collect settles the queue: the other request's intake.
	pullAndReport(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, session, schedapi.Result{})
	eventually(t, "the stale decide step ended", func() (string, bool) {
		for _, s := range steps(t, ctx, r.ID) {
			if s == "decide" {
				return "still there", false
			}
		}
		return "", true
	})
	_ = asked
}
