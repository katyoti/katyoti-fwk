// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN AGENT TESTS FIRST — a request whose end is refused only by deliverables
// no check verifies gives them to an agent's auto-test step instead of
// blocking; the check the agent records then ends it. Once per deliverable: a
// second refusal blocks, and `requests autotest` asks an agent again. A
// deliverable a person tests by hand (by_hand) blocks at once, for the person.

// workFix works a fix request's steps up to its end.
func workFix(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, id, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	autoTested(t, ctx, id)
	// The reviewer verifies it (a person where asked: their step waits).
	if v := stepOf(t, ctx, id, "verify"); v.For == schedapi.ForAgent {
		pullAndReport(t, ctx, v.ID, session, schedapi.Result{Summary: "verify done", Verdict: api.VerdictApproved})
	}
}

func TestAnUntestedDeliverableGoesToAnAgentFirst(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the totals", Module: "billing", Kind: api.KindFix,
		Deliverables: []api.Deliverable{{Title: "the totals are right"}}, By: person})
	workFix(t, ctx, r.ID)
	if st := planState(t, ctx, r.ID); st == api.PlanBlocked {
		t.Fatalf("its end was blocked on an untested deliverable; want an agent's auto-test first")
	}
	at := stepOf(t, ctx, r.ID, api.StepAutoTest)
	if at.For != schedapi.ForAgent || at.Packet.Task != api.AutoTestTask || at.Packet.Phase != api.PhaseVerify {
		t.Fatalf("the auto-test step: %+v; want an agent's auto-test in verify", at)
	}
	// The agent records a check that passes, then reports: the request ends.
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: session, Note: "a check that proves it, falsified",
		Deliverables: []api.Deliverable{{Title: "the totals are right", Check: "go version"}}}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, at.ID, session, schedapi.Result{Summary: "checked"})
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after the auto-test recorded a passing check: %s; want done", st)
	}
}

func TestASecondRefusalBlocksAndAutoTestAsksAgain(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the dates", Module: "billing", Kind: api.KindFix,
		Deliverables: []api.Deliverable{{Title: "the dates are right"}}, By: person})
	workFix(t, ctx, r.ID)
	// The agent gives no check: the second refusal blocks, as before.
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepAutoTest).ID, session, schedapi.Result{Summary: "found no way"})
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("after an auto-test that gave no check: %s; want blocked", st)
	}
	if page := requestPanel(t, ctx, r.ID); !strings.Contains(page, "ask an agent to test it") || !strings.Contains(page, `"uri":"ktw-agents:requests/auto-test"`) {
		t.Error("the blocked request does not offer to ask an agent to test it")
	}
	if _, err := req.AutoTest(ctx, api.AutoTestInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st == api.PlanBlocked {
		t.Fatal("asked to auto-test, it stays blocked")
	}
	if at := stepOf(t, ctx, r.ID, api.StepAutoTest); at.State != schedapi.StateReady {
		t.Errorf("the auto-test asked again is %s; want ready", at.State)
	}
	if _, err := req.AutoTest(ctx, api.AutoTestInput{ID: r.ID, By: person}); err == nil {
		t.Error("a second auto-test while one waits was accepted")
	}
}

func TestADeliverableAPersonTestsWaitsForThePerson(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the layout", Module: "billing", Kind: api.KindFix,
		Deliverables: []api.Deliverable{{Title: "it reads well", ByHand: "open the invoice and read it; only a person judges readability"}}, By: person})
	workFix(t, ctx, r.ID)
	// An agent tried it first; kept by hand, the verify is the person's.
	if v := stepOf(t, ctx, r.ID, "verify"); v.For != schedapi.ForPerson {
		t.Fatalf("a by-hand deliverable: its verify %s; want the person's", v.For)
	}
	if _, err := req.AutoTest(ctx, api.AutoTestInput{ID: r.ID, By: person}); err == nil {
		t.Error("an auto-test of a by-hand deliverable was accepted")
	}
}

// TestARevisedDeliverableKeepsHowAPersonTestsIt: the auto-test marks a
// deliverable no agent can test with by_hand (the steps and why) through
// Revise - under its own title, or a new one - and the request keeps it.
func TestARevisedDeliverableKeepsHowAPersonTestsIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the layout", Module: "billing", Kind: api.KindFix,
		Deliverables: []api.Deliverable{{Title: "it reads well"}}, By: person})
	steps := "open the invoice and read it; no agent renders the page"
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: session, Note: "only a person can judge it",
		Deliverables: []api.Deliverable{{Title: "it reads well", ByHand: steps}, {Title: "it prints well", ByHand: "print it"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Deliverables) != 2 || got.Request.Deliverables[0].ByHand != steps || got.Request.Deliverables[1].ByHand != "print it" {
		t.Fatalf("the by-hand steps were not kept: %+v", got.Request.Deliverables)
	}
}

// TestARevisedDeliverableKeepsHowLongItsCheckMayRun: a slow check (a
// clean-machine proof) declares its own bound through Revise - on a new check,
// and on a check that is unchanged - and the request keeps it.
func TestARevisedDeliverableKeepsHowLongItsCheckMayRun(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix the install", Module: "billing", Kind: api.KindFix,
		Deliverables: []api.Deliverable{{Title: "the proof passes", Check: "bash proof.sh"}}, By: person})
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: session, Note: "a proof runs about 15 min",
		Deliverables: []api.Deliverable{{Title: "the proof passes", Check: "bash proof.sh", Timeout: "45m"}, {Title: "it is fast", Check: "bash fast.sh", Timeout: "20m"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d := got.Request.Deliverables; len(d) != 2 || d[0].Timeout != "45m" || d[1].Timeout != "20m" {
		t.Fatalf("the checks' bounds were not kept: %+v", d)
	}
}
