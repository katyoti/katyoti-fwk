// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strconv"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestSpawner_TerminalChildrenPrunedAfterTTL (UC-133) — terminal child
// entries in LocalSpawner.children must not accumulate indefinitely. After
// the retention TTL elapses, pruneStaleChildrenLocked drops them while
// preserving in-flight children.
//
// Approach: construct a LocalSpawner directly (no factory wiring needed for
// the prune-logic isolation test), seed 100 terminal entries with a past
// timestamp, plus one in-flight entry (no terminalAt). Trigger the prune
// and assert only the in-flight survivor remains.
func TestSpawner_TerminalChildrenPrunedAfterTTL(t *testing.T) {
	s := &LocalSpawner{
		children:     make(map[string]*childEntry),
		retentionTTL: 1 * time.Millisecond,
	}

	past := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 100; i++ {
		id := "child-" + strconv.Itoa(i)
		_, cancel := context.WithCancel(context.Background())
		s.children[id] = &childEntry{
			handle:     engine.ChildHandle{RunID: id, Name: id, Status: engine.ChildDone},
			cancel:     cancel,
			done:       make(chan struct{}),
			terminalAt: past,
		}
		close(s.children[id].done)
		cancel()
	}
	_, liveCancel := context.WithCancel(context.Background())
	defer liveCancel()
	s.children["alive"] = &childEntry{
		handle: engine.ChildHandle{RunID: "alive", Name: "alive", Status: engine.ChildRunning},
		cancel: liveCancel,
		done:   make(chan struct{}),
	}

	if got := len(s.children); got != 101 {
		t.Fatalf("setup: expected 101 children, got %d", got)
	}

	s.mu.Lock()
	s.pruneStaleChildrenLocked(time.Now())
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()
	if got := len(s.children); got != 1 {
		t.Fatalf("UC-133: expected 1 in-flight child after prune, got %d", got)
	}
	if _, ok := s.children["alive"]; !ok {
		t.Fatal("UC-133: in-flight child was pruned (terminalAt was zero — must not be evicted)")
	}
}

// TestSpawner_SetRetentionTTL_DisablesPrune (UC-133 boundary) — a non-positive
// TTL disables pruning entirely.
func TestSpawner_SetRetentionTTL_DisablesPrune(t *testing.T) {
	s := &LocalSpawner{
		children:     make(map[string]*childEntry),
		retentionTTL: 1 * time.Millisecond,
	}
	s.SetRetentionTTL(0)

	past := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 10; i++ {
		id := "child-" + strconv.Itoa(i)
		_, cancel := context.WithCancel(context.Background())
		s.children[id] = &childEntry{
			handle:     engine.ChildHandle{RunID: id, Status: engine.ChildDone},
			cancel:     cancel,
			done:       make(chan struct{}),
			terminalAt: past,
		}
		cancel()
	}

	s.mu.Lock()
	s.pruneStaleChildrenLocked(time.Now())
	s.mu.Unlock()

	if got := len(s.children); got != 10 {
		t.Fatalf("UC-133: TTL=0 must disable pruning, got %d entries (expected 10)", got)
	}
}
