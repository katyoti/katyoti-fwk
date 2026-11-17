// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// Publish sends one payload on a subject - the bus's Publish (MessageBus,
// rule) as a function, so this package needs nothing of the bus.
type Publish func(ctx context.Context, subject string, payload []byte) error

// Scope is where a panel's subjects live: its tenant and universe.
type Scope struct {
	Tenant   string
	Universe string
}

// Handle is a running task's hold on the panel it opened: each call sends
// one event, numbered after the last, and moves the panel's state. It is
// safe for concurrent use; an event after the panel completed or closed is
// refused.
type Handle struct {
	mu      sync.Mutex
	id      string
	scope   Scope
	publish Publish
	seq     int64
	state   State
	lent    bool // resumed from a Ref: reports, never completes (lend.go)
}

// OpenPanel opens a new panel - its id fresh - and sends its open event.
func OpenPanel(ctx context.Context, scope Scope, publish Publish, open Open) (*Handle, error) {
	return OpenPanelWithID(ctx, scope, publish, NewID(), open)
}

// OpenPanelWithID opens the panel id (an id made by NewID earlier, given
// back by a long method before its run starts) and sends its open event.
func OpenPanelWithID(ctx context.Context, scope Scope, publish Publish, id string, open Open) (*Handle, error) {
	if publish == nil {
		return nil, fmt.Errorf("panel: open: no publish")
	}
	h := &Handle{id: id, scope: scope, publish: publish}
	withGenericTree(&open)
	if err := h.send(ctx, Event{Kind: KindOpen, Open: &open}, StateOpen); err != nil {
		return nil, err
	}
	return h, nil
}

// ID is the panel's id, what any screen shows it by.
func (h *Handle) ID() string { return h.id }

// State is where the panel is.
func (h *Handle) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Seq is the sequence number of the last event sent.
func (h *Handle) Seq() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seq
}

// InputSubject is the subject the person's input to this panel comes back
// on: what the owning run waits on.
func (h *Handle) InputSubject() string {
	return subjects.PanelInSubject(h.scope.Tenant, h.scope.Universe, h.id)
}

// Patch changes what the panel shows by node id (render.Op); each op takes
// the event's sequence number.
func (h *Handle) Patch(ctx context.Context, ops ...render.Op) error {
	return h.send(ctx, Event{Kind: KindOps, Ops: ops}, "")
}

// Progress says how far the work is.
func (h *Handle) Progress(ctx context.Context, label string, current, total int64) error {
	return h.send(ctx, Event{Kind: KindProgress, Progress: &Progress{Label: label, Current: current, Total: total}}, "")
}

// Ask puts a question to the person; the panel is asking until an input.
func (h *Handle) Ask(ctx context.Context, ask Ask) error {
	return h.send(ctx, Event{Kind: KindAsk, Ask: &ask}, StateAsking)
}

// Answered says the person's input was taken: the panel runs again. It
// sends nothing - the input itself is in the panel's log.
func (h *Handle) Answered() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == StateAsking {
		h.state = StateOpen
	}
}

// Complete ends the work with its status and result.
func (h *Handle) Complete(ctx context.Context, status string, result any) error {
	return h.send(ctx, Event{Kind: KindComplete, Complete: &Complete{Status: status, Result: result}}, StateComplete)
}

// Close closes the panel.
func (h *Handle) Close(ctx context.Context) error {
	return h.send(ctx, Event{Kind: KindClose}, StateClosed)
}

// send numbers, validates and publishes one event; next is the state it
// moves the panel to ("" keeps it).
func (h *Handle) send(ctx context.Context, e Event, next State) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == StateClosed || (h.state == StateComplete && e.Kind != KindClose) {
		return fmt.Errorf("panel %s: %s after the panel is %s", h.id, e.Kind, h.state)
	}
	if h.lent && (e.Kind == KindComplete || e.Kind == KindClose) {
		return fmt.Errorf("panel %s: a lent handle sends no %s - the run that owns the panel does", h.id, e.Kind)
	}
	e.Panel = h.id
	e.Seq = h.seq + 1
	for i := range e.Ops {
		e.Ops[i].Seq = e.Seq
	}
	if err := e.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("panel %s: %w", h.id, err)
	}
	if err := h.publish(ctx, subjects.PanelOutSubject(h.scope.Tenant, h.scope.Universe, h.id), payload); err != nil {
		return fmt.Errorf("panel %s: send %s: %w", h.id, e.Kind, err)
	}
	h.seq = e.Seq
	if next != "" {
		h.state = next
	}
	return nil
}
