// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"sync"
)

// Checkpoint holds the state + wait spec for durable execution.
// On restore, the executor re-subscribes from WaitSpec and continues waiting.
type Checkpoint struct {
	State State     `json:"state"`
	Event Event     `json:"event"`          // the event that woke the run, when this save is its wake (a restart steps with it)
	Wait  *WaitSpec `json:"wait,omitempty"` // what we're waiting for (if suspended)
	// Layout is the layout of State.Data, as its machine declares it
	// (StateLayouter); empty when the machine declares none.
	Layout string `json:"layout,omitempty"`
}

// StateLayouter is a machine that declares the layout of the state it saves —
// a name and a version ("agent-loop/1") its author bumps when the saved
// shape changes. A checkpoint saved under another layout is not resumed: it
// would be read half.
type StateLayouter interface {
	StateLayout() string
}

// StateLayoutOf is the layout a machine declares, "" when it declares none.
func StateLayoutOf(m Machine) string {
	if l, ok := m.(StateLayouter); ok {
		return l.StateLayout()
	}
	return ""
}

// StateStore persists checkpoints for long-running state machines.
// Phase 1: MemoryStateStore (no durability — process death = lost).
// Phase 2: SQLite. Phase 3: S3+Postgres.
type StateStore interface {
	Save(runID string, cp Checkpoint) error
	Load(runID string) (Checkpoint, error)
	Delete(runID string) error
}

// MemoryStateStore is an in-process store with no durability.
type MemoryStateStore struct {
	mu    sync.RWMutex
	store map[string]Checkpoint
}

// NewMemoryStateStore creates an in-process state store.
func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		store: make(map[string]Checkpoint),
	}
}

func (s *MemoryStateStore) Save(runID string, cp Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[runID] = cp
	return nil
}

func (s *MemoryStateStore) Load(runID string) (Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp, ok := s.store[runID]
	if !ok {
		return Checkpoint{}, fmt.Errorf("checkpoint %q not found", runID)
	}
	return cp, nil
}

func (s *MemoryStateStore) Delete(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, runID)
	return nil
}

// NopStateStore discards all checkpoints. Used when durability is not needed.
type NopStateStore struct{}

func (NopStateStore) Save(string, Checkpoint) error   { return nil }
func (NopStateStore) Load(string) (Checkpoint, error) { return Checkpoint{}, fmt.Errorf("nop store") }
func (NopStateStore) Delete(string) error             { return nil }
