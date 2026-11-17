// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// Event is the universal input to a state machine.
// Everything is an event: a call, a notification, a resume trigger, an internal tick.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
	ID      string `json:"id,omitempty"`    // an event's own id, when its producer gives one
	Label   string `json:"label,omitempty"` // identifies which WaitCondition matched
	// Replayed: the event is stepped with again after a restart — the step it
	// woke may have done part of its work before the process died.
	Replayed bool `json:"replayed,omitempty"`
}

// TickEvent is the internal event type used to continue the loop
// when the previous step produced neither Output nor Wait.
const TickEvent = "_tick"

// AllEvent wakes a machine whose WaitAll wait is met: its Payload is every
// event that met it ([]Event), in condition order.
const AllEvent = "_all"

// State is a serializable execution snapshot. The checkpoint.
// Data is typed (struct pointer or map) — Machine.Step handles conversion.
type State struct {
	Name    string `json:"name"`
	Data    any    `json:"data,omitempty"`
	RunID   string `json:"runId"`
	Version int    `json:"version"`
}

// StepResult is what an ActionFunc produces.
// Data is full replacement — the action owns the state completely.
// NextAction is derived from fields: Output set → Done, Wait set → Suspended, neither → Continue.
type StepResult struct {
	NextState string    `json:"nextState"`
	Data      any       `json:"data,omitempty"`
	Output    any       `json:"output,omitempty"`
	Emit      []Event   `json:"emit,omitempty"`
	Wait      *WaitSpec `json:"wait,omitempty"` // non-nil → suspend, wait for events
	// Panels are the display panel events the step sends
	// numbered by the machine's panel.Track kept in its state; the executor
	// publishes them after the checkpoint, as it dispatches Emit.
	Panels []panel.Event `json:"panels,omitempty"`
	// KeepWaiting: the step goes on waiting on the wait it was woken from
	// (parallel / race awaiting more children) — the same subscriptions, the
	// events queued meanwhile kept. Without it, a step that returns no Wait
	// ends its wait: its subscriptions and the events still queued for it go.
	KeepWaiting bool `json:"keepWaiting,omitempty"`

	// Failure is the typed companion to the legacy `Output.(map)["error"]`
	// failure shape, per `decisions/node-handler-interface.md` quirk #2.
	// When NextState=="failed", machines SHOULD populate this alongside the
	// `Output` map (the map shape stays for backward compat with existing
	// callers). Consumers that prefer structured failure handling read
	// `result.Failure.Reason` instead of parsing the error string out of the
	// output map. Lifecycle event emitters (e.g. `wrapLifecycleEvents`)
	// prefer this field when present and fall back to the map shape.
	Failure *Failure `json:"failure,omitempty"`
}

// Failure is the structured form of a machine-level failure. NOT a Go error
// (those live in `RunResult.Error`) — this carries the *kind* of failure so
// callers can switch on it: a child crashed, a retry budget was exhausted,
// a validation rule rejected the input. The legacy map-based form
// (`Output.(map)["error"]`) coexists for backward compatibility.
//
// All fields except Reason are optional. Producers populate whatever's
// available; consumers should tolerate missing context fields.
type Failure struct {
	// Reason is the short human-readable summary. Always populated.
	Reason string `json:"reason"`

	// Child is the role name of the child that triggered the failure
	// (parallel branch failed, race loser, single spawn failed). Empty if
	// the failure is workflow-internal (validation, missing node, etc.).
	Child string `json:"child,omitempty"`

	// Retries is the count of attempts before this failure was final.
	// Zero unless the OnErrorRetry strategy ran and exhausted its budget.
	Retries int `json:"retries,omitempty"`
}

// ActionFunc is a closure with everything baked in by Machine.Step.
// Only ctx comes from the executor — allows timeout/cancel wrapping.
// Step already bound: typed data, event, input parameters.
type ActionFunc func(ctx context.Context) (*StepResult, error)

// Machine is the strategy interface for state machine execution.
// It is NOT a transition table — the table is one implementation.
// Step receives state + event, handles type conversion, and returns
// a ready-to-call closure. The executor just calls it.
type Machine interface {
	// Name identifies this machine type (for registry lookup).
	Name() string

	// InitialState creates the starting state from a start event.
	InitialState(event Event) State

	// Step is the core dispatch: given state + event, decide what to call.
	// Handles type conversion (map → struct after checkpoint restore),
	// parameter binding, TTL checks, and returns a ready-to-call closure.
	// On checkpoint restore, calling Step with the same (state, event)
	// produces the same ActionFunc — no separate RestoreState needed.
	Step(ctx context.Context, state *State, event Event) (ActionFunc, error)
}

// NextAction indicates what the executor should do after a step.
type NextAction int

const (
	Continue  NextAction = iota // feed next event (internal tick)
	Done                        // terminal — Output is the final result
	Suspended                   // waiting for external event(s) (WaitSpec)
	Failed                      // error
)

func (n NextAction) String() string {
	switch n {
	case Continue:
		return "continue"
	case Done:
		return "done"
	case Suspended:
		return "suspended"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// EventStamper is an optional interface for event payloads.
// If a payload implements it, the executor calls StampEvent before publishing.
// This lets event structs with embedded metadata (type, timestamp, emitter)
// get auto-populated without the machine code filling them manually.
type EventStamper interface {
	StampEvent(eventType string, emitter string)
}

// RunResult is the outcome of an executor run (or resume).
//
// Failure carries the typed structured-failure shape per D1 quirk #2 — it
// is the companion to the legacy `Output.(map)["error"]` map idiom. When
// the workflow reaches a Failed terminal via the `NextState=="failed"`
// path, the executor copies the terminal StepResult's Failure into the
// RunResult so external callers (CLI, tests, dispatch glue) can inspect
// the structured form without parsing the output map.
type RunResult struct {
	State   State      `json:"state"`
	Next    NextAction `json:"next"`
	Output  any        `json:"output,omitempty"`
	Error   error      `json:"error,omitempty"`
	Wait    *WaitSpec  `json:"wait,omitempty"`
	Failure *Failure   `json:"failure,omitempty"`
}
