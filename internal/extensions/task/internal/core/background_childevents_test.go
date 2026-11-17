// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sync"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestBgManager_EmitsChildTerminal pins that the background manager publishes
// `child.<taskID>.done` / `.failed` on the shared Channel at terminal commits,
// so an agent_as_human parent blocked on await-child wakes instead of polling.
func TestBgManager_EmitsChildTerminal(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	m := NewBackgroundTaskManager(channel).(*bgManager)

	var mu sync.Mutex
	var doneFired, failedFired bool
	var failErr string
	bus.Subscribe(engine.ChildDoneTopic("t1"), func(_ string, _ any) {
		mu.Lock()
		doneFired = true
		mu.Unlock()
	})
	bus.Subscribe(engine.ChildFailedTopic("t2"), func(_ string, payload any) {
		mu.Lock()
		failedFired = true
		if e, ok := payload.(*engine.ChildFailedEvent); ok {
			failErr = e.Error
		}
		mu.Unlock()
	})

	m.emitChildTerminal("t1", "alpha", "parent", "")    // success
	m.emitChildTerminal("t2", "beta", "parent", "boom") // failure

	mu.Lock()
	defer mu.Unlock()
	if !doneFired {
		t.Error("child.t1.done was not published on completion")
	}
	if !failedFired {
		t.Error("child.t2.failed was not published on failure")
	}
	if failErr != "boom" {
		t.Errorf("failed event Error = %q, want boom", failErr)
	}
}

// TestBgManager_EmitChildTerminal_NilChannel pins the best-effort guard: a nil
// Channel (unit/test contexts) must be a silent no-op, never a panic.
func TestBgManager_EmitChildTerminal_NilChannel(t *testing.T) {
	m := NewBackgroundTaskManager(nil).(*bgManager)
	m.emitChildTerminal("t1", "alpha", "parent", "") // must not panic
}
