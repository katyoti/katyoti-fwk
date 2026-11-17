// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// suspension holds the in-memory state of one suspended task: the resume
// channel callers block on + bookkeeping for Suspensions().
type suspension struct {
	taskID    string
	waitURI   string
	startedAt time.Time
	resume    chan json.RawMessage // buffered=1 so Resume never blocks
	delivered bool                 // idempotency flag for repeat Resume calls
	// out is what the waiter receives when a group scheduler is installed:
	// the payload is passed on once the run has its slot again.
	out chan json.RawMessage
}

// suspensionManager owns the ask-user suspension inbox (dimension 5,
// user-comms): a per-task resume channel + bookkeeping so the inbox
// ask-user path (and any future async-wait tool) can block on a Resume
// call from an EventService handler. This is the agent-suspension lifecycle
// owner the user-comms runtime builds on; it is intentionally separate from
// the background scheduler. v1 is in-memory only; cross-restart persistence
// is a future follow-up.
type suspensionManager struct {
	mu          sync.RWMutex
	suspensions map[string]*suspension // taskID → suspension record + channel
	// slots, when set, frees the waiting run's group slot during the wait.
	slots *groupScheduler
	// tell, when set, tells the run's lifecycle it waits (on what) in a tool
	// call, and is woken.
	tell func(runID string, t engine.RunTransition, what string)
}

func (m *suspensionManager) told(runID string, t engine.RunTransition, what string) {
	if m.tell != nil {
		m.tell(runID, t, what)
	}
}

func newSuspensionManager() *suspensionManager {
	return &suspensionManager{suspensions: make(map[string]*suspension)}
}

// Suspend registers a per-task wakeup channel. The caller blocks on the
// returned receive-only channel until Resume delivers a payload (typically
// from an EventService handler that fired on the configured wait URI).
// Repeated Suspend calls for the same taskID return the SAME channel so
// concurrent callers within one task share the wakeup; in practice the
// ask-user in-flight guard makes this one-caller anyway.
//
// taskID may be empty in legacy programmatic paths that don't have a proper
// task identity (chat keepalive, ad-hoc tool invocations); we generate a
// synthetic ID + log a warning so the operator can spot the leak — a Suspend
// without a matching Resume strands the channel in the map until process exit.
func (m *suspensionManager) Suspend(taskID string, waitURI string) (<-chan json.RawMessage, error) {
	if waitURI == "" {
		return nil, fmt.Errorf("Suspend: waitURI must be non-empty")
	}
	if taskID == "" {
		// Its answer could never reach it (Resume names the task): refused,
		// not left waiting for ever.
		return nil, fmt.Errorf("Suspend: no task id: the answer to %s could never reach it", waitURI)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Repeated Suspend for the same taskID:
	//   - existing.delivered=false → return the same channel
	//     (shared wakeup for concurrent waiters in the same task)
	//   - existing.delivered=true  → start a fresh suspension
	//     (previous wait cycle is complete; this is a new wait)
	if existing, ok := m.suspensions[taskID]; ok && !existing.delivered {
		if existing.out != nil {
			return existing.out, nil
		}
		return existing.resume, nil
	}

	s := &suspension{
		taskID:    taskID,
		waitURI:   waitURI,
		startedAt: time.Now(),
		resume:    make(chan json.RawMessage, 1), // buffered so Resume never blocks
	}
	m.suspensions[taskID] = s
	if m.slots == nil {
		m.told(taskID, engine.RunWaiting, waitURI)
		return s.resume, nil
	}
	// The run waits inside a tool call: its slot goes back now and is taken
	// again (queueing) before the waiter continues.
	m.slots.release(taskID)
	m.told(taskID, engine.RunWaiting, waitURI)
	s.out = make(chan json.RawMessage, 1)
	ended := m.slots.ended(taskID)
	go func(id string, in <-chan json.RawMessage, out chan<- json.RawMessage) {
		select {
		case payload := <-in:
			_ = m.slots.acquire(context.Background(), id) // ends when the run does
			out <- payload
		case <-ended: // the run ended unanswered: nothing to wait for
		}
	}(taskID, s.resume, s.out)
	return s.out, nil
}

// Resume delivers payload on the resume channel for taskID. Returns an error
// if no suspension exists for taskID. Idempotent for the "already delivered"
// case (no-op + nil).
func (m *suspensionManager) Resume(taskID string, payload json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.suspensions[taskID]
	if !ok {
		return fmt.Errorf("Resume: taskID %q is not suspended", taskID)
	}
	if s.delivered {
		// Already resumed — no-op. Idempotent path matters because
		// EventService handlers may fire twice on subject overlap or NATS
		// redelivery; the second call shouldn't surface as an error in
		// operator logs.
		return nil
	}
	// Non-blocking send: channel is buffered=1, never blocks the caller. If
	// the receiver has already exited (ctx cancelled), the value sits in the
	// buffer until the suspension record is removed and the channel is GC'd.
	s.resume <- payload
	s.delivered = true
	m.told(taskID, engine.RunWoken, "its answer")
	// Keep the record in the map so a second Resume call lands on the
	// `s.delivered` short-circuit above instead of returning "not suspended".
	// Suspensions() excludes delivered records so the GUI / sweeper never see
	// them; the entry sits idle until the next Suspend reuses the taskID OR
	// the run completes (a post-completion cleanup is a future concern when
	// the persistence story lands).
	return nil
}

// forget removes a run's suspension once the run has ended: an answer after
// is to a run that is gone.
func (m *suspensionManager) forget(taskID string) {
	m.mu.Lock()
	delete(m.suspensions, taskID)
	m.mu.Unlock()
}

// Suspensions returns a snapshot of currently-waiting tasks. Used by the
// timeout sweeper + by GUI consumers rendering the inbox panel.
func (m *suspensionManager) Suspensions() []taskapi.SuspendedRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]taskapi.SuspendedRecord, 0, len(m.suspensions))
	for _, s := range m.suspensions {
		if s.delivered {
			continue
		}
		out = append(out, taskapi.SuspendedRecord{
			TaskID:    s.taskID,
			WaitURI:   s.waitURI,
			StartedAt: s.startedAt,
		})
	}
	return out
}

// WhileWaiting: see taskapi.SuspensionManager.
func (m *suspensionManager) WhileWaiting(ctx context.Context, taskID string, wait func()) error {
	if m.slots == nil || taskID == "" {
		wait()
		return nil
	}
	m.slots.release(taskID)
	m.told(taskID, engine.RunWaiting, "a person's answer")
	wait()
	m.told(taskID, engine.RunWoken, "the person's answer")
	return m.slots.acquire(ctx, taskID)
}

var _ taskapi.SuspensionManager = (*suspensionManager)(nil)
