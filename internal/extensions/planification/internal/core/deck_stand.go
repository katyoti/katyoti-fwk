// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// WHERE A REQUEST STANDS, ON THE DECK — read from its to-dos (the scheduler's
// queue), never from its plan's next step, which is written once when the
// request is filed. A request is:
//   - blocked: a to-do of it failed or is blocked, or its end was refused;
//   - waiting for a person: a person's step of it is ready (theirs, in the
//     Inbox), or its requester was asked a question;
//   - running: a to-do of it is pulled (or reported, being collected);
//   - lapsed: its pulled to-do's claim is past its until (its holder silent);
//   - ready: a step of it may be pulled now;
//   - waiting: on what comes before its next step — or nothing of it is
//     scheduled.
// The same precedence as the rollup (rollup.go): a person's step goes before
// work in progress.
//
// Its lane is the phase of its current step: the failed one, else the one a
// person must answer, else the one pulled, else the first ready, else the
// first that waits.

// stand is one open request on the deck.
type stand struct {
	plan  api.PlannedObject
	state string        // ready | running | waiting | blocked: the plan states' labels
	step  *api.WorkTodo // its current step; nil when it has none
	why   string        // why it waits or is blocked
	asked bool          // its requester was asked a question
	ended bool          // done, dropped or split: no step comes next
	// hold is what the one reader says holds its stopped step, when read;
	// onPerson whether the reader puts that stop on the person
	// : only then is the person asked to retry it.
	hold     *api.Blocker
	onPerson bool
}

// lane is the phase the request is in: its current step's, else requests.
func (s stand) lane() string {
	if s.step != nil && s.step.Packet.Phase != "" {
		return s.step.Packet.Phase
	}
	return api.PhaseRequests
}

// stepText is the current step in words: who does it and the step it is.
func (s stand) stepText() string {
	if s.step == nil {
		return "not planned"
	}
	p := s.step.Packet
	who := p.Task
	if p.Doer == api.DoerPerson {
		who = "a person to " + p.Interaction
	}
	if who == "" {
		who = p.Doer
	}
	return who + " (" + p.Step + ")"
}

// meta is the card's second line: its module, its step (none once it
// ended), and why it waits.
func (s stand) meta() string {
	parts := []string{s.plan.Module}
	if !s.ended {
		parts = append(parts, s.stepText())
	}
	if of := s.partOf(); of != "" {
		parts = append([]string{of}, parts...)
	}
	if s.why != "" {
		parts = append(parts, s.why)
	}
	return strings.Join(parts, " · ")
}

// heldText is a running request's line: its module, the step pulled, who
// holds it and since when.
func (s stand) heldText() string {
	parts := []string{s.plan.Module, s.stepText()}
	if s.step != nil && s.step.Claim != nil {
		held := "held by " + s.step.Claim.By.ID
		if at, err := time.Parse(time.RFC3339Nano, s.step.Claim.At); err == nil {
			held += " since " + at.Local().Format("15:04")
		}
		parts = append(parts, held)
	}
	return strings.Join(parts, " · ")
}

// partOf is what a request belongs to, in words: "part of <id>" or
// "follow-up of <id>"; "" for a demand of its own.
func (s stand) partOf() string {
	switch {
	case s.plan.SplitFrom != "":
		return "part of " + s.plan.SplitFrom
	case s.plan.FollowsUp != "":
		return "follow-up of " + s.plan.FollowsUp
	}
	return ""
}

// standsOf reads where each open plan stands from the to-dos of its request;
// asked names the requests whose requester was asked a question (their
// asked-in edges).
func standsOf(ctx context.Context, plans []api.PlannedObject, asked map[string]bool) ([]stand, error) {
	all, err := todosOf(ctx, "")
	if err != nil {
		return nil, err
	}
	by := map[string][]api.WorkTodo{}
	for _, t := range all {
		by[t.Request] = append(by[t.Request], t)
	}
	var out []stand
	for _, p := range plans {
		if planEnded(p.State) || p.State == api.PlanSplit {
			continue
		}
		out = append(out, standOf(p, by[p.ID], asked[p.ID], viewStepHold(ctx, p.ID, p.Title)))
	}
	return out, nil
}

// standOf decides where one request stands from its open to-dos (in position
// order); why a step of it waits or stopped is what hold, the one reader,
// says holds it - nil reads nothing.
func standOf(p api.PlannedObject, todos []api.WorkTodo, asked bool, hold stepHold) stand {
	sort.SliceStable(todos, func(i, j int) bool { return todos[i].Position < todos[j].Position })
	s := stand{plan: p, asked: asked}
	pick := func(match func(api.WorkTodo) bool) *api.WorkTodo {
		for i := range todos {
			if match(todos[i]) {
				return &todos[i]
			}
		}
		return nil
	}
	anyStep := func(api.WorkTodo) bool { return true }
	failed := func(t api.WorkTodo) bool {
		return stopped(t)
	}
	personReady := func(t api.WorkTodo) bool { return t.State == schedapi.StateReady && t.For == schedapi.ForPerson }
	pulled := func(t api.WorkTodo) bool {
		return t.State == schedapi.StateClaimed || t.State == schedapi.StateReported
	}
	ready := func(t api.WorkTodo) bool { return t.State == schedapi.StateReady }
	// held words what holds t, as the one reader says.
	held := func(t api.WorkTodo) (api.Blocker, bool, bool) {
		if hold == nil {
			return api.Blocker{}, false, false
		}
		return hold(t)
	}
	switch {
	case pick(failed) != nil:
		t := pick(failed)
		verb, summary := stopVerb(*t)
		s.state, s.step, s.why = stateLabels[api.PlanBlocked], t, verb+": "+summary
		if b, onPerson, ok := held(*t); ok {
			s.why, s.hold, s.onPerson = holdWords(b), &b, onPerson
		}
	case p.State == api.PlanBlocked:
		s.state, s.step, s.why = stateLabels[api.PlanBlocked], pick(anyStep), p.Note
	case pick(personReady) != nil:
		s.state, s.step, s.why = stateLabels[api.PlanIdle], pick(personReady), "a person's answer (in the Inbox)"
	case asked:
		s.state, s.step, s.why = stateLabels[api.PlanIdle], pick(anyStep), "its requester's answer"
		if p.Note != "" {
			s.why = p.Note
		}
	case pick(pulled) != nil:
		s.state, s.step = stateLabels[standRunning], pick(pulled)
		if at, ok := claimLapsed(*s.step, deckNow()); ok {
			s.state, s.why = stateLabels[standLapsed], s.step.Claim.By.ID+" silent since "+at.Local().Format("15:04")
		}
	case pick(ready) != nil:
		s.state, s.step = stateLabels[api.PlanReady], pick(ready)
	case pick(anyStep) != nil:
		t := pick(anyStep)
		s.state, s.step, s.why = stateLabels[api.PlanIdle], t, "it waits"
		if b, _, ok := held(*t); ok {
			s.why = holdWords(b)
		}
	default:
		// Nothing of it is scheduled: never planned (work plan), or its steps
		// are all collected and its end is still to come.
		s.state, s.why = stateLabels[api.PlanIdle], "no step scheduled"
		if p.Note != "" && p.State == api.PlanIdle {
			s.why = p.Note // waits on its follow-ups, its requester...
		}
	}
	return s
}

// deckNow is the deck's clock (a test sets it).
var deckNow = time.Now

// claimLapsed answers when a pulled to-do's claim lapsed, if it is past its
// until at now; a reported to-do (being collected) never lapses.
func claimLapsed(t api.WorkTodo, now time.Time) (time.Time, bool) {
	if t.State != schedapi.StateClaimed || t.Claim == nil {
		return time.Time{}, false
	}
	until, err := time.Parse(time.RFC3339Nano, t.Claim.Until)
	if err != nil || !now.After(until) {
		return time.Time{}, false
	}
	return until, true
}

// awaitsPerson reports a request waiting for a person: a person's step is
// theirs, or its requester was asked a question.
func (s stand) awaitsPerson() bool {
	return s.state == stateLabels[api.PlanIdle] &&
		((s.step != nil && s.step.For == schedapi.ForPerson && s.step.State == schedapi.StateReady) || s.asked)
}

// failedStep is the to-do to retry for a blocked request, if one failed.
func (s stand) failedStep() string {
	if s.state == stateLabels[api.PlanBlocked] && s.step != nil &&
		stopped(*s.step) {
		return s.step.ID
	}
	return ""
}
