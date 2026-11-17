// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// oneReaderWorld is one stored state holding a piece of work of each kind
// K1-K6, a failed step and a step blocked on other work: the state every
// « the one reader » test reads (« À faire », the Inbox's list, the queue's
// wait lines), never a second copy of it.
type oneReaderWorld struct {
	// Answer (K1) waits on a person's provide step, Act (K2) on a person's
	// validate step.
	Answer, Act string
	// Ready (K3) is agent work ready, Held (K4) agent work held by another.
	Ready, Held string
	// Outside (K5) waits on an outside party (a GUI runner).
	Outside string
	// Incident (K6) is blocked with no wait named, past its deadline.
	Incident string
	// Failed is an agent's step whose failures went past the cap of agent
	// rounds; FailedOnce one that failed once - an agent retries it (K4,
	//
	Failed, FailedOnce string
	// OnWork is blocked on other work (Ready).
	OnWork string
}

// onThePerson is, per piece of work, whether the one reader puts it on the
// person (K1, K2, the incident and the step failed past the cap) or not.
func (w oneReaderWorld) onThePerson() map[string]bool {
	return map[string]bool{w.Answer: true, w.Act: true, w.Incident: true, w.Failed: true,
		w.Ready: false, w.Held: false, w.Outside: false, w.OnWork: false, w.FailedOnce: false}
}

// newOneReaderWorld files the K1-K6 state and answers its context.
func newOneReaderWorld(t *testing.T) (context.Context, oneReaderWorld) {
	t.Helper()
	ctx, _, req := world(t)
	return ctx, fillOneReaderWorld(t, ctx, req)
}

// fillOneReaderWorld files the K1-K6 state in a world.
func fillOneReaderWorld(t *testing.T, ctx context.Context, req api.RequestService) oneReaderWorld {
	t.Helper()
	return fillOneReaderWorldIn(t, ctx, req, "")
}

// fillOneReaderWorldIn files the K1-K6 state in a world, each request filed
// in increment inc ("" none).
func fillOneReaderWorldIn(t *testing.T, ctx context.Context, req api.RequestService, inc string) oneReaderWorld {
	t.Helper()
	var w oneReaderWorld
	w.Answer = file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing", Increment: inc}).ID
	personStep(t, ctx, w.Answer, "give-numbering", api.InteractionProvide)
	w.Act = file(t, ctx, req, api.FileInput{Title: "Round the totals", Module: "billing", Increment: inc}).ID
	personStep(t, ctx, w.Act, "validate-rounding", api.InteractionValidate)
	w.Ready = file(t, ctx, req, api.FileInput{Title: "Print the invoices", Module: "billing", Increment: inc}).ID
	w.Held = file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing", Increment: inc}).ID
	if got, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, w.Held, api.StepIntake).ID, By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull: %v %+v", err, got)
	}
	w.Outside = file(t, ctx, req, api.FileInput{Title: "Show the invoice screen", Module: "billing", Increment: inc}).ID
	reportBlocked(t, ctx, stepOf(t, ctx, w.Outside, api.StepIntake).ID, "its GUI check needs a runner",
		&schedapi.HeldOn{Kind: schedapi.HeldResource, Target: "gui-runner", Reason: "no GUI runner on the hub"})
	w.Incident = file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "billing", Increment: inc}).ID
	reportBlocked(t, ctx, stepOf(t, ctx, w.Incident, api.StepIntake).ID, "something is off", nil)
	// Past its deadline: an unnamed block is the person's only then
	//
	ageReport(t, ctx, stepOf(t, ctx, w.Incident, api.StepIntake).ID, 25*time.Hour)
	w.Failed = file(t, ctx, req, api.FileInput{Title: "Sign the invoices", Module: "billing", Increment: inc}).ID
	failPastTheCap(t, ctx, stepOf(t, ctx, w.Failed, api.StepIntake).ID, "the signing key is missing")
	w.FailedOnce = file(t, ctx, req, api.FileInput{Title: "Stamp the invoices", Module: "billing", Increment: inc}).ID
	pullAndReport(t, ctx, stepOf(t, ctx, w.FailedOnce, api.StepIntake).ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "the stamp is missing"})
	w.OnWork = file(t, ctx, req, api.FileInput{Title: "Send the invoice copies", Module: "billing", Increment: inc}).ID
	reportBlocked(t, ctx, stepOf(t, ctx, w.OnWork, api.StepIntake).ID, "it needs the printed invoices",
		&schedapi.HeldOn{Kind: schedapi.HeldObject, Target: w.Ready, Reason: "the invoices are printed first"})
	return w
}

// checkPutsOnThePerson asserts the check agrees with the world's expectation
// and answers, per piece of work, whether it puts it on the person.
func checkPutsOnThePerson(t *testing.T, ctx context.Context, w oneReaderWorld) map[string]api.Blocker {
	t.Helper()
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	blockers := map[string]api.Blocker{}
	for work, want := range w.onThePerson() {
		b := blockerOf(t, out, work)
		blockers[work] = b
		if onPerson := len(b.Chain) == 0 && b.Due == "" && (b.ActorKind == api.ActorPerson || b.Kind == api.BlockerIncident); onPerson != want {
			t.Errorf("%s: the check says %s %s by %q (chain %v); the test expects it on the person: %v", work, b.Class, b.Kind, b.Actor, b.Chain, want)
		}
	}
	return blockers
}

// « À faire » READS WHAT HOLDS WORK FROM THE ONE READER:
// on one stored state holding a piece of work of each kind K1-K6, a failed
// step and a step blocked on other work, « À faire » carries a card for a
// piece of work exactly when `planning check` puts it on the person (K1, K2)
// or calls it an incident (K6) at the root of its own wait - never for agent
// work ready or held (K3, K4), an outside party's hold (K5) or a wait on other
// work, which is moved where that work's root is.
func TestToDoNamesThePersonExactlyWhereTheCheckDoes(t *testing.T) {
	t.Parallel()
	ctx, w := newOneReaderWorld(t)
	blockers := checkPutsOnThePerson(t, ctx, w)
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := views.PersonQueue(ctx, api.PersonQueueInput{})
	if err != nil {
		t.Fatal(err)
	}
	carded := map[string]bool{}
	for _, it := range q.Items {
		carded[it.Request] = true
	}
	for work, onPerson := range w.onThePerson() {
		if b := blockers[work]; carded[work] != onPerson {
			t.Errorf("%s: « À faire » carries a card: %v; the check puts it on the person: %v (%s %s by %q, chain %v)",
				work, carded[work], onPerson, b.Class, b.Kind, b.Actor, b.Chain)
		}
	}
}

// THE INBOX READS WHAT HOLDS WORK FROM THE ONE READER (
// ): on the same K1-K6 state, the Inbox's waiting list -
// what the rail's badge and the phone glance read - carries an item for a
// piece of work exactly when `planning check` puts it on the person: the
// person's own step for K1 and K2, an unblock item for the incident and the
// failed step; none for agent work ready or held, an outside party's hold or
// a wait on other work. The badge counts that same list.
func TestTheInboxNamesThePersonExactlyWhereTheCheckDoes(t *testing.T) {
	t.Parallel()
	ctx, w := newOneReaderWorld(t)
	blockers := checkPutsOnThePerson(t, ctx, w)
	waiting, err := application.UseService[inboxapi.WaitingService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := waiting.List(ctx, inboxapi.WaitingInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string][]string{}
	n := 0
	for _, g := range out.Groups {
		for _, it := range g.Items {
			n++
			if it.Request != "" {
				kinds[it.Request] = append(kinds[it.Request], it.Kind)
			}
		}
	}
	for work, onPerson := range w.onThePerson() {
		if b := blockers[work]; (len(kinds[work]) > 0) != onPerson {
			t.Errorf("%s: the Inbox lists %v; the check puts it on the person: %v (%s %s by %q, chain %v)",
				work, kinds[work], onPerson, b.Class, b.Kind, b.Actor, b.Chain)
		}
	}
	for _, work := range []string{w.Incident, w.Failed} {
		unblock := false
		for _, k := range kinds[work] {
			unblock = unblock || k == inboxapi.KindUnblock
		}
		if !unblock {
			t.Errorf("%s: the Inbox lists %v; want an %s item for its stopped step", work, kinds[work], inboxapi.KindUnblock)
		}
	}
	for _, work := range []string{w.Answer, w.Act} {
		for _, k := range kinds[work] {
			if k == inboxapi.KindUnblock {
				t.Errorf("%s: the Inbox lists an %s item; its person step waits as its own kind", work, k)
			}
		}
	}
	if out.Count != n {
		t.Errorf("the list counts %d questions but holds %d", out.Count, n)
	}
	badge, err := waiting.Count(ctx, inboxapi.WaitingInput{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if badge.Count != n {
		t.Errorf("the badge says %d; the list holds %d", badge.Count, n)
	}
}
