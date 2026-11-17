// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// Executor drives a Machine to completion (or suspension).
// Implements the Joram A3 transactional react loop:
//
//	wait → react (buffered emit) → commit (pop + save + dispatch) → subscribe → loop
//
// Key guarantees:
//   - No events delivered during Step execution (inbox paused)
//   - No outgoing events visible until commit (emit buffer)
//   - Checkpoint AFTER action (not before) — crash = replay from last committed state
//   - Pop-on-commit: consumed events removed only when state is safely persisted
//   - On suspension: executor blocks on inbox.Get() until WaitSpec is satisfied
type Executor struct {
	machine        Machine
	channel        *Channel
	store          StateStore
	inbox          *MachineInbox
	preferredRunID string         // pre-assigned run id (set via SetRunID); used if state.RunID is empty
	hooks          []ExecutorHook // D4: observers + policy injectors; see executor_hooks.go
	panels         PanelPublisher // sends a step's panel events; nil drops them
}

// PanelPublisher sends a step's display panel events, in order (the task
// service publishes them on their panel subjects).
type PanelPublisher func(ctx context.Context, events []panel.Event)

// SetPanelPublisher sets who sends the panel events a step returns.
func (e *Executor) SetPanelPublisher(p PanelPublisher) { e.panels = p }

// NewExecutor creates an executor for the given machine.
// channel and store may be nil — defaults are used.
func NewExecutor(machine Machine, channel *Channel, store StateStore) *Executor {
	if channel == nil {
		channel = NewChannel(nil)
	}
	if store == nil {
		store = NopStateStore{}
	}
	return &Executor{
		machine: machine,
		channel: channel,
		store:   store,
		inbox:   NewMachineInbox(),
	}
}

// Inbox returns the executor's inbox for external event delivery.
func (e *Executor) Inbox() *MachineInbox {
	return e.inbox
}

// SetRunID pre-assigns the run id used by this executor. If set, it takes
// precedence over the auto-generated id. The TaskService uses this so it can
// wire the recorder filter to the same run id that will be stamped on events.
func (e *Executor) SetRunID(id string) {
	e.preferredRunID = id
}

// Run starts the machine from its initial state with the given event.
func (e *Executor) Run(ctx context.Context, startEvent Event) (*RunResult, error) {
	state := e.machine.InitialState(startEvent)
	if state.RunID == "" {
		state.RunID = e.preferredRunID
	}
	if state.RunID == "" {
		state.RunID = GenerateRunID()
	}
	e.channel.Register(state.RunID, e.inbox)
	defer e.channel.Unregister(state.RunID)

	return e.loop(ctx, &state, startEvent, false)
}

// ResumeWaiting restores a run checkpointed while it waited — after a
// restart — and waits again where it waited: the saved WaitSpec is subscribed
// again, then the run continues from the saved state with the event that met
// it. A wait marked RecheckOnResume is rechecked at once (see await).
// The executor must be built around a freshly created machine of the same
// task, with the store holding the checkpoint.
func (e *Executor) ResumeWaiting(ctx context.Context, runID string) (*RunResult, error) {
	cp, err := e.store.Load(runID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint %q: %w", runID, err)
	}
	if cp.Wait == nil {
		return nil, fmt.Errorf("run %q was not waiting", runID)
	}
	e.channel.Register(runID, e.inbox)
	defer e.channel.Unregister(runID)

	if cp.State.RunID == "" {
		cp.State.RunID = runID
	}
	event, rechecking, err := e.await(ctx, &cp.State, cp.Wait, true)
	if err != nil {
		return &RunResult{State: cp.State, Next: Failed, Error: err}, nil
	}
	return e.loop(ctx, &cp.State, event, rechecking)
}

// await waits on w: it subscribes the wait FIRST, so nothing published from
// then on is lost, then lets the observers see it (the slot given back, the
// external events bridged), then blocks until the wait is met.
//
// A wait marked RecheckOnResume may have been met before it was subscribed —
// an answer given while the step that asked was still committing, or while no
// process listened (a restart). When recheck is true such a wait is not
// blocked on: the machine steps once at once with a TickEvent and checks for
// itself (rechecking is then true). If that recheck waits again, the loop
// arms it with recheck false, so a wait is rechecked once, never in a loop.
func (e *Executor) await(ctx context.Context, state *State, w *WaitSpec, recheck bool) (event Event, rechecking bool, err error) {
	if !w.Mode.Valid() {
		// No mode chosen: refused, never taken for a wait-for-all.
		return Event{}, false, ErrWaitWithoutMode
	}
	runID := state.RunID
	e.channel.Wait(runID, w)
	e.callSuspend(ctx, runID, w)
	if recheck && w.RecheckOnResume {
		event = Event{Type: TickEvent}
		e.callResume(ctx, runID, event)
		return event, true, nil
	}
	events, err := e.inbox.Get(ctx, w)
	if err != nil {
		return Event{}, false, err
	}
	event = Event{Type: TickEvent}
	switch {
	case w.Mode == WaitAll && len(events) > 1:
		event = Event{Type: AllEvent, Label: "all", Payload: events} // every event that met it
	case len(events) > 0:
		event = events[0]
	}
	// The wait is met: the store keeps no longer the wait — a crash from here
	// must not wait again for an event already taken — but the run woken by
	// this event. A kind restarted after a crash steps with it again; another
	// keeps no checkpoint and is reported interrupted.
	if err := e.store.Save(runID, Checkpoint{State: *state, Event: event, Layout: StateLayoutOf(e.machine)}); err != nil {
		slog.Error("executor: checkpoint save failed", "run_id", runID, "err", err)
	}
	// D4: observers see the resume just before the next Step.
	e.callResume(ctx, runID, event)
	return event, false, nil
}

// loop is the core execution loop (Joram A3 transactional model). rechecked
// is true when event is the recheck of a wait (see await).
func (e *Executor) loop(ctx context.Context, state *State, event Event, rechecked bool) (*RunResult, error) {
	// Make the run identity + shared Channel reachable from every machine
	// action's ctx. The agent reads its id from its loop; a OneShot strategy
	// (simulator, program) has none — and the comms service needs both the run
	// id (to suspend/route a child's question) and the Channel (to emit the
	// wake events a parent's Await reacts to).
	ctx = WithRunID(ctx, state.RunID)
	ctx = WithChannel(ctx, e.channel)

	// lastWait tracks the most recent non-nil WaitSpec a step returned.
	// When a subsequent step returns Wait==nil, we first try to drain any
	// queued event that matches lastWait before falling back to TickEvent —
	// otherwise events arriving between steps (e.g., the 2nd/3rd child.done
	// during a parallel.awaiting cycle) would sit in the inbox forever.
	var lastWait *WaitSpec
	for {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return &RunResult{State: *state, Next: Failed, Error: err}, nil
		}

		// ── 1. REACT (inbox paused, emit buffered) ──────────────────────
		e.inbox.Pause()

		// D4: hooks fire before Step. An OnPreStep error short-circuits
		// the step with the same Failed-result shape as a Step error;
		// OnPostStep still fires below so observers see the abort.
		if err := e.callPreStep(ctx, state, event); err != nil {
			e.callPostStep(ctx, state, nil, err)
			e.inbox.Resume()
			return &RunResult{State: *state, Next: Failed, Error: err}, nil
		}

		// Machine decides WHAT to call
		action, err := e.machine.Step(ctx, state, event)
		if err != nil {
			e.callPostStep(ctx, state, nil, err)
			e.inbox.Resume()
			return &RunResult{State: *state, Next: Failed, Error: err}, nil
		}

		// Execute action
		result, err := action(ctx)
		if err != nil {
			e.callPostStep(ctx, state, result, err)
			e.inbox.Resume()
			return &RunResult{State: *state, Next: Failed, Error: err}, nil
		}

		// D4: OnPostStep fires on every successful action too.
		e.callPostStep(ctx, state, result, nil)

		// A step that keeps waiting waits again on the wait it was woken from.
		if result.Wait == nil && result.KeepWaiting && lastWait != nil {
			result.Wait = lastWait
		}

		// ── 2. Apply state update ───────────────────────────────────────
		state.Name = result.NextState
		state.Data = result.Data
		state.Version++

		// ── 3. COMMIT (atomic: pop + save + dispatch) ───────────────────
		// Pop consumed events from inbox
		e.inbox.Pop()

		for i := range result.Emit {
			stampEvent(&result.Emit[i], state.RunID)
		}

		// Checkpoint AFTER action (Joram model).
		if err := e.store.Save(state.RunID, Checkpoint{
			State:  *state,
			Wait:   result.Wait,
			Layout: StateLayoutOf(e.machine),
		}); err != nil {
			// A lost checkpoint breaks crash-replay. The in-memory run keeps
			// going — its run is told checkpoint-failed (the task service's
			// store).
			slog.Error("executor: checkpoint save failed", "run_id", state.RunID, "err", err)
		}

		// Dispatch (now visible to others)
		e.channel.Dispatch(result.Emit)
		// The step's panel events go out after its checkpoint too.
		if len(result.Panels) > 0 && e.panels != nil {
			e.panels(ctx, result.Panels)
		}

		// ── 4. Terminal? ────────────────────────────────────────────────
		if result.Output != nil {
			if err := e.store.Delete(state.RunID); err != nil {
				slog.Warn("executor: checkpoint delete failed", "run_id", state.RunID, "err", err)
			}
			e.inbox.Resume()
			// D1 quirk #2: propagate the typed Failure companion when the
			// machine populated it (workflow-level structured failure). The
			// Output map shape remains the primary signal for "is this run
			// done?" — Failure is supplementary metadata.
			return &RunResult{State: *state, Next: Done, Output: result.Output, Failure: result.Failure}, nil
		}

		// ── 5. Suspended — subscribe and block on inbox ─────────────────
		if result.Wait != nil {
			e.inbox.Resume()
			lastWait = result.Wait
			// A wait that comes out of a recheck is not rechecked again.
			event, rechecked, err = e.await(ctx, state, result.Wait, !rechecked)
			if err != nil {
				return &RunResult{State: *state, Next: Failed, Error: err}, nil
			}
			continue
		}
		rechecked = false

		// Wait==nil: the step left its wait. Its subscriptions end and what was
		// still queued for it goes — a race loser's late event is never fed to
		// the next step.
		e.inbox.Resume()
		if lastWait != nil {
			e.channel.EndWait(state.RunID)
			lastWait = nil
		}
		event = Event{Type: TickEvent}
	}
}

// stampEvent populates EventBase metadata on payloads that support it.
// It also fills the correlation envelope's RunID (layered stamping — the
// executor knows the run; it never overwrites a non-empty field).
func stampEvent(evt *Event, emitter string) {
	if evt.Payload == nil {
		return
	}
	FillCorrelation(evt.Payload, Correlation{RunID: emitter})
	// Try direct (payload is already a pointer)
	if stamper, ok := evt.Payload.(EventStamper); ok {
		stamper.StampEvent(evt.Type, emitter)
		return
	}
	// Try via reflect — get addressable copy for pointer-receiver methods
	v := reflect.ValueOf(&evt.Payload).Elem()
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.CanAddr() {
		if stamper, ok := v.Addr().Interface().(EventStamper); ok {
			stamper.StampEvent(evt.Type, emitter)
			return
		}
	}
	// Value not addressable — make a copy, stamp, store back
	cp := reflect.New(v.Type())
	cp.Elem().Set(v)
	if stamper, ok := cp.Interface().(EventStamper); ok {
		stamper.StampEvent(evt.Type, emitter)
		evt.Payload = cp.Elem().Interface()
	}
}

// GenerateRunID creates a short random run ID.
func GenerateRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("run-%x", b)
}
