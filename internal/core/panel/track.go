// SPDX-License-Identifier: Apache-2.0

package panel

import "fmt"

// Track is where a panel stands for a machine that drives it from its steps
// : its id, the sequence of its last event and its state. It is
// plain data, kept in the run's checkpointed state, so the numbering goes on
// after a restart; Next makes each event the step returns (StepResult.Panels),
// which the engine publishes after the checkpoint.
type Track struct {
	ID    string `json:"id"`
	Seq   int64  `json:"seq"`
	State State  `json:"state,omitempty"`
}

// NewTrack tracks the panel id (a fresh one when empty), not opened yet.
func NewTrack(id string) Track {
	if id == "" {
		id = NewID()
	}
	return Track{ID: id}
}

// Next numbers e after the last event, validates it and moves the panel's
// state: open first, asking after an ask, open again after an input taken
// (Answered), complete, closed; nothing but a close after complete.
func (t *Track) Next(e Event) (Event, error) {
	if t.State == StateClosed || (t.State == StateComplete && e.Kind != KindClose) {
		return Event{}, fmt.Errorf("panel %s: %s after the panel is %s", t.ID, e.Kind, t.State)
	}
	if e.Kind == KindOpen && e.Open != nil {
		o := *e.Open
		withGenericTree(&o)
		e.Open = &o
	}
	e.Panel = t.ID
	e.Seq = t.Seq + 1
	for i := range e.Ops {
		e.Ops[i].Seq = e.Seq
	}
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	t.Seq = e.Seq
	switch e.Kind {
	case KindOpen:
		t.State = StateOpen
	case KindAsk:
		t.State = StateAsking
	case KindComplete:
		t.State = StateComplete
	case KindClose:
		t.State = StateClosed
	}
	return e, nil
}

// Answered says the person's input was taken: an asking panel is open again.
func (t *Track) Answered() {
	if t.State == StateAsking {
		t.State = StateOpen
	}
}
