// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A HAND TEST — « Tester à la main »: a person tests one deliverable no check
// verifies. It passes: the deliverable is verified by them, what they saw as
// its evidence (Verify). It fails, saying how: the work goes back — the
// validation waiting on the test is answered rejected with that note, so the
// work before it is done again with it as its feedback; with no validation
// waiting, the request is blocked saying why. Either way the failure lasts on
// the request (its step's result, or its plan's note).

// Test records a hand test of one deliverable, by a person.
func (s *Service) Test(ctx context.Context, in api.TestInput) (api.RequestOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.RequestOutput{}, err
	}
	if in.By.Kind != "person" {
		return api.RequestOutput{}, invalid("Test: a person tests by hand, not %s %s — give it a check instead (Revise)", in.By.Kind, in.By.ID)
	}
	relayed, err := personsAct(ctx, in.By, "test:"+in.ID+"#"+in.Deliverable, "Test: a person tests by hand themselves (« À faire », the request view, or their own command)")
	if err != nil {
		return api.RequestOutput{}, err
	}
	note := strings.TrimSpace(in.Note)
	switch in.Result {
	case api.TestPasses:
		if note == "" {
			note = "passes"
		}
		return s.Verify(person.ForAct(ctx, "verify:"+in.ID+"#"+in.Deliverable), api.VerifyInput{ID: in.ID, Deliverable: in.Deliverable, Evidence: "tested by hand: " + note, By: in.By})
	case api.TestFails:
	default:
		return api.RequestOutput{}, invalid("Test: the result is passes or fails, got %q", in.Result)
	}
	if note == "" {
		return api.RequestOutput{}, invalid("Test: say how “%s” fails (note)", in.Deliverable)
	}
	o, f, err := openRequest(ctx, "Test", in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	found := false
	for _, d := range f.Deliverables {
		found = found || d.Title == in.Deliverable
	}
	if !found {
		return api.RequestOutput{}, invalid("Test: %s has no deliverable %q", in.ID, in.Deliverable)
	}
	why := fmt.Sprintf("tested by hand by %s — “%s” fails: %s", in.By.ID, in.Deliverable, note)
	if relayed != nil {
		why += fmt.Sprintf(" (relayed by %s, confirmed by %s)", relayed.By, relayed.Confirmed)
	}
	todos, err := todosOf(ctx, in.ID)
	if err != nil {
		return api.RequestOutput{}, err
	}
	for _, t := range todos {
		if t.For != schedapi.ForPerson || t.State != schedapi.StateReady || !askedOf(t) ||
			(t.Packet.Interaction != api.InteractionValidate && t.Packet.Interaction != api.InteractionChallenge) {
			continue
		}
		if t.Packet.AskedIn == "" {
			// Asked as a human task: its report is the
			// person's verdict - sent back.
			sc, err := sched(ctx)
			if err != nil {
				return api.RequestOutput{}, err
			}
			if _, err := sc.Report(ctx, schedapi.ReportInput{ID: t.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
				Verdict: api.VerdictRejected, Summary: "Sent back: " + why, By: in.By}}); err != nil {
				return api.RequestOutput{}, fmt.Errorf("Test: send back %s: %w", t.Packet.Step, err)
			}
			awaitCollected(ctx, t)
			return api.RequestOutput{Request: toRequest(o)}, nil
		}
		inbox, err := application.UseService[inboxapi.InboxService](ctx)
		if err != nil {
			return api.RequestOutput{}, err
		}
		if _, err := inbox.Answer(person.ForAct(ctx, "answer:"+t.Packet.AskedIn), inboxapi.AnswerInput{ID: t.Packet.AskedIn, Decision: inboxapi.StatusRejected,
			Note: "Sent back: " + why, By: in.By.ID}); err != nil {
			return api.RequestOutput{}, fmt.Errorf("Test: send back %s: %w", t.Packet.Step, err)
		}
		// Followed at once (the Inbox's event follows it too, finding it done):
		// the page re-read after this shows the work sent back.
		if _, err := s.Answered(ctx, t.Packet.AskedIn); err != nil && stillReady(ctx, t) {
			return api.RequestOutput{}, fmt.Errorf("Test: send back %s: %w", t.Packet.Step, err)
		}
		awaitCollected(ctx, t)
		return api.RequestOutput{Request: toRequest(o)}, nil
	}
	blocked := handTestFailed + why
	// No validation waiting: the request's last round of agent work is done
	// again at once, the failure as its feedback, and its plan resumes - the
	// person tests again once it has landed. Past
	// the cap of rounds, or nothing to redo: blocked for the person.
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	planned, err := reworkAtEnd(ctx, cwm, in.ID, "hand test failed: "+in.Deliverable, blocked, in.By)
	if err != nil {
		return api.RequestOutput{}, err
	}
	if planned {
		if planState(o) == api.PlanBlocked {
			if _, err := NewPlans().plan(ctx, api.PlanNextInput{ID: in.ID, By: in.By, Note: blocked + " — its work is done again by an agent"}); err != nil {
				return api.RequestOutput{}, err
			}
		}
		if err := settleRequest(ctx, in.ID, PlanningActor); err != nil {
			return api.RequestOutput{}, err
		}
	} else if planState(o) == api.PlanBlocked {
		// Already blocked (its end refused for this very deliverable): the
		// failure becomes why.
		pf, _ := planOf(o)
		pf.Note, pf.Block = blocked, api.BlockHandTestFailed
		if _, err := writePlan(ctx, cwm, o, pf, true); err != nil {
			return api.RequestOutput{}, err
		}
	} else if _, err := NewPlans().blockAs(ctx, api.BlockInput{ID: in.ID, Note: blocked, By: in.By}, api.BlockHandTestFailed); err != nil {
		return api.RequestOutput{}, err
	}
	got, err := s.Get(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		return api.RequestOutput{}, err
	}
	return api.RequestOutput{Request: got.Request}, nil
}

// awaitCollected waits, a few seconds at most, for a to-do answered just now
// to be collected — whichever of this call and the Inbox's event reported it —
// so what is read next shows the work sent back.
func awaitCollected(ctx context.Context, t api.WorkTodo) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		todos, err := listTodos(ctx, t.Request)
		if err != nil || !slices.ContainsFunc(todos, func(x api.WorkTodo) bool { return x.ID == t.ID }) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// stillReady reports a to-do still waiting to be reported: not taken another
// way meanwhile (the Inbox's event following the same answer).
func stillReady(ctx context.Context, t api.WorkTodo) bool {
	todos, err := listTodos(ctx, t.Request)
	if err != nil {
		return true
	}
	for _, x := range todos {
		if x.ID == t.ID {
			return x.State == schedapi.StateReady
		}
	}
	return false
}
