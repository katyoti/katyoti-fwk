// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A STEP THAT ENDS ON ITS TARGET'S STATE — a person's step whose packet names
// EndsOn (the decide step: `applies`) is done when its target reaches that
// state, however it got there: a decision resolved on its own panel, from the
// command line, or before the step was even asked. Who moved it reports it —
// a person only: a move made by an agent ends nothing a person owes. The
// report comes first, then the step's question in the inbox is closed with
// the same answer, so the closing never reads as an answer of its own.

// Reached ends the person's steps of id that wait for it to reach `to`: what
// the observer calls on the target's own move.
func Reached(ctx context.Context, id, to string, by cwmapi.Actor) error {
	if by.Kind != "person" {
		return nil
	}
	todos, err := stepsEndingOn(ctx, id)
	if errors.Is(err, cwmapi.ErrInvalidInput) {
		return nil // no planning space (a system module's object: an inbox entry's own move)
	}
	if err != nil {
		return err
	}
	for _, t := range todos {
		if t.Packet.EndsOn != to || t.For != schedapi.ForPerson || t.State != schedapi.StateReady {
			continue // not yet asked: the settle that makes it ready ends it
		}
		if err := endOnTarget(ctx, t, by); err != nil {
			return err
		}
	}
	return nil
}

// stepsEndingOn is the to-dos that may end on id's move: its own plan's
// (a decide request on a decision) and the steps of its module's requests
// that name it (WorkPacket.On: the decision a choice opened).
func stepsEndingOn(ctx context.Context, id string) ([]api.WorkTodo, error) {
	// id in its project: its module's to-dos are that project's (qualified
	// when several projects are served); the rows keep ids bare.
	ref := requestRef(ctx, id)
	module, err := requestModule(ref)
	if err != nil {
		return todosOf(ctx, id)
	}
	s, err := sched(ctx)
	if err != nil {
		return nil, err
	}
	got, err := s.List(ctx, schedapi.ListInput{Module: module})
	if err != nil {
		return nil, err
	}
	bare := bareID(ref)
	var out []api.WorkTodo
	for _, t := range got.Todos {
		if t.Owner != api.Owner {
			continue
		}
		if w := decode(t); (w.Request == bare && w.Packet.On == "") || bareID(endsOnObject(w)) == bare {
			out = append(out, w)
		}
	}
	return out, nil
}

// endReachedAlready ends a step that became ready while its target already
// was in the state it ends on, by the person who moved it there; moved by no
// person, the person is asked as usual.
func endReachedAlready(ctx context.Context, t api.WorkTodo) {
	settling.Lock() // after the settle that made it ready
	settling.Unlock()
	by, ok := movedBy(ctx, endsOnObject(t), t.Packet.EndsOn)
	if !ok {
		if err := askPerson(ctx, t); err != nil {
			slog.Warn("planification: a person's step was not asked", "todo", t.ID, "error", err)
		}
		return
	}
	if err := endOnTarget(ctx, t, by); err != nil {
		slog.Warn("planification: a step whose target was already there did not end", "todo", t.ID, "error", err)
	}
}

// movedBy is the person whose move brought id to `to`, read from its journal.
func movedBy(ctx context.Context, id, to string) (cwmapi.Actor, bool) {
	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return cwmapi.Actor{}, false
	}
	got, err := journal.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil {
		return cwmapi.Actor{}, false
	}
	for i := len(got.Entries) - 1; i >= 0; i-- {
		if e := got.Entries[i]; e.To == to {
			return e.By, e.By.Kind == "person"
		}
	}
	return cwmapi.Actor{}, false
}

// endOnTarget reports a person's step done by the person who moved its
// target, with what was chosen, then closes its question in the inbox.
func endOnTarget(ctx context.Context, t api.WorkTodo, by cwmapi.Actor) error {
	s, err := sched(ctx)
	if err != nil {
		return err
	}
	on := endsOnObject(t)
	answer := chosenOn(ctx, on, t.Packet.EndsOn)
	if on != t.Request {
		answer += " on " + on // the steps after it read where the choice is recorded
	}
	r := schedapi.Result{Status: schedapi.StatusSuccess, Summary: answer, Answer: answer, By: by}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: t.ID, Result: r}); err != nil {
		return fmt.Errorf("end %s on %s: %w", t.ID, on, err)
	}
	if t.Packet.AskedIn == "" {
		return nil
	}
	closing := answer
	if on == t.Request {
		closing += " (on " + on + ")"
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return nil
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: t.Packet.AskedIn, Note: closing, By: by.ID}); err != nil {
		return fmt.Errorf("close the question %s of %s: %w", t.Packet.AskedIn, t.ID, err)
	}
	return nil
}

// chosenOn says what the target's move came to: the options a decision
// selected, else the state it reached.
func chosenOn(ctx context.Context, id, state string) string {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "it became " + state
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return "it became " + state
	}
	if d, ok := o.Capabilities["decision"]; ok {
		if sel, ok := d["selected"].([]any); ok && len(sel) > 0 {
			var names []string
			for _, x := range sel {
				names = append(names, fmt.Sprint(x))
			}
			return "chose " + strings.Join(names, ", ")
		}
	}
	return "it became " + state
}

// closeAsked closes the Inbox question of a person's step that leaves the
// queue without an answer — its request ended or was split: the question is
// dismissed as planification's own act, never left pending for an answer
// nothing would collect (why is the step's reason to leave).
func closeAsked(ctx context.Context, t api.WorkTodo, why string) {
	if t.Packet.AskedIn == "" {
		return
	}
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return
	}
	withdraw(ctx, inbox, t.Packet.AskedIn) // planification's own act, whoever's session ended the request
}
