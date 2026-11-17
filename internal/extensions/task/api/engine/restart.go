// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
)

// RestartedEvent is the event a run restarted after a crash steps with: it
// was working when its process died, and goes on from its last saved step —
// the step that was in flight then is done again, or (an agent's tool calls)
// answered "interrupted by a restart".
const RestartedEvent = "_restarted"

// Restartable is a machine whose steps may be done again after a crash: it
// resumes from its last saved step. A machine that is not (a script whose one
// step may have side effects) is reported interrupted instead.
type Restartable interface {
	RestartsAfterCrash() bool
}

// RestartsAfterCrash reports whether a machine resumes after a crash.
func RestartsAfterCrash(m Machine) bool {
	r, ok := m.(Restartable)
	return ok && r.RestartsAfterCrash()
}

// ReplaySafe marks a machine whose steps are safe to do again (pure, or
// idempotent): it restarts after a crash.
func ReplaySafe(m Machine) Machine { return replaySafe{m} }

type replaySafe struct{ Machine }

func (replaySafe) RestartsAfterCrash() bool { return true }

// ResumeWorking restarts a run that was working when its process died, from
// its last saved step: the machine steps with a RestartedEvent — or, when its
// last save is the run woken from a wait, with the event that woke it, marked
// Replayed (the step it woke may have done part of its work). The executor
// must be built around a freshly created machine of the same task, with the
// store holding the checkpoint.
func (e *Executor) ResumeWorking(ctx context.Context, runID string) (*RunResult, error) {
	cp, err := e.store.Load(runID)
	if err != nil {
		return nil, fmt.Errorf("load checkpoint %q: %w", runID, err)
	}
	e.channel.Register(runID, e.inbox)
	defer e.channel.Unregister(runID)

	state := cp.State
	if state.RunID == "" {
		state.RunID = runID
	}
	event := Event{Type: RestartedEvent}
	if cp.Event.Type != "" { // woken: the event it had taken
		event = cp.Event
		event.Replayed = true
	}
	return e.loop(ctx, &state, event, false)
}
