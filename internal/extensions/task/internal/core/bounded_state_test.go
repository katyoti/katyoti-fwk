// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strconv"
	"sync"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestBgManager_TerminalEntriesPrunedAfterTTL (UC-104) — terminal background
// task entries must not accumulate indefinitely. Once the retention TTL
// elapses, the next insert path (or any caller-driven prune) drops them while
// preserving in-flight readers.
//
// Approach: construct a bgManager directly, seed 100 terminal entries with a
// timestamp older than the test TTL, plus one in-flight entry (no terminalAt).
// Trigger pruneStaleLocked — assert only the in-flight survivor remains.
func TestBgManager_TerminalEntriesPrunedAfterTTL(t *testing.T) {
	m := &bgManager{
		entries:       make(map[string]*bgEntry),
		notifications: make(map[string][]taskapi.TaskNotification),
		retentionTTL:  1 * time.Millisecond,
	}

	past := time.Now().Add(-1 * time.Hour) // already way past the 1ms TTL
	for i := 0; i < 100; i++ {
		id := "t-" + strconv.Itoa(i)
		m.entries[id] = &bgEntry{
			record:     taskapi.TaskRecord{ID: id, Status: taskapi.TaskCompleted},
			done:       make(chan struct{}),
			terminalAt: past,
		}
		close(m.entries[id].done)
	}
	m.entries["in-flight"] = &bgEntry{
		record: taskapi.TaskRecord{ID: "in-flight", Status: taskapi.TaskRunning},
		done:   make(chan struct{}),
	}

	if got := len(m.entries); got != 101 {
		t.Fatalf("setup: expected 101 entries, got %d", got)
	}

	m.mu.Lock()
	m.pruneStaleLocked(time.Now())
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	if got := len(m.entries); got != 1 {
		t.Fatalf("UC-104: expected 1 in-flight entry after prune, got %d", got)
	}
	if _, ok := m.entries["in-flight"]; !ok {
		t.Fatal("UC-104: in-flight entry was pruned (terminalAt was zero — must not be evicted)")
	}
}

// TestBgManager_SetRetentionTTL_DisablesPrune (UC-104 boundary) — a
// non-positive TTL disables pruning entirely. Useful for diagnostics or
// long-lived audit modes that need to keep full history.
func TestBgManager_SetRetentionTTL_DisablesPrune(t *testing.T) {
	m := &bgManager{
		entries:       make(map[string]*bgEntry),
		notifications: make(map[string][]taskapi.TaskNotification),
		retentionTTL:  1 * time.Millisecond,
	}
	m.SetRetentionTTL(0)

	past := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 10; i++ {
		id := "t-" + strconv.Itoa(i)
		m.entries[id] = &bgEntry{
			record:     taskapi.TaskRecord{ID: id, Status: taskapi.TaskCompleted},
			done:       make(chan struct{}),
			terminalAt: past,
		}
	}

	m.mu.Lock()
	m.pruneStaleLocked(time.Now())
	m.mu.Unlock()

	if got := len(m.entries); got != 10 {
		t.Fatalf("UC-104: TTL=0 must disable pruning, got %d entries (expected 10)", got)
	}
}

// TestRunRegistry_TerminalRunsPrunedAfterTTL (UC-132) — runRegistry.runs
// must not accumulate indefinitely. The same TTL prune-on-insert policy
// applies (the Service composes this registry).
//
// Approach: construct a runRegistry directly. Seed 100 terminal entries +
// 1 in-flight, trigger prune, assert the in-flight entry survives.
func TestRunRegistry_TerminalRunsPrunedAfterTTL(t *testing.T) {
	r := &runRegistry{
		runs:         make(map[string]*runEntry),
		retentionTTL: 1 * time.Millisecond,
	}

	past := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 100; i++ {
		id := "run-" + strconv.Itoa(i)
		r.runs[id] = &runEntry{
			info:       taskapi.RunInfo{RunID: id, Status: taskapi.RunStatusCompleted},
			done:       make(chan struct{}),
			terminalAt: past,
		}
		close(r.runs[id].done)
	}
	r.runs["live"] = &runEntry{
		info: taskapi.RunInfo{RunID: "live", Status: taskapi.RunStatusRunning},
		done: make(chan struct{}),
	}

	if got := len(r.runs); got != 101 {
		t.Fatalf("setup: expected 101 runs, got %d", got)
	}

	r.mu.Lock()
	r.pruneLocked(time.Now())
	r.mu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()
	if got := len(r.runs); got != 1 {
		t.Fatalf("UC-132: expected 1 in-flight run after prune, got %d", got)
	}
	if _, ok := r.runs["live"]; !ok {
		t.Fatal("UC-132: in-flight run was pruned (terminalAt was zero — must not be evicted)")
	}
}

// TestRunRegistry_AddGetMutateList pins the registry's lifecycle ops
// (the unit the Service composes): add inserts, get returns the pointer,
// mutate writes under the lock, list snapshots + filters by session.
func TestRunRegistry_AddGetMutateList(t *testing.T) {
	r := newRunRegistry()
	r.add(&runEntry{info: taskapi.RunInfo{RunID: "a", SessionID: "s1", Status: taskapi.RunStatusRunning}, done: make(chan struct{})})
	r.add(&runEntry{info: taskapi.RunInfo{RunID: "b", SessionID: "s2", Status: taskapi.RunStatusRunning}, done: make(chan struct{})})

	if r.len() != 2 {
		t.Fatalf("len = %d, want 2", r.len())
	}
	if _, ok := r.get("a"); !ok {
		t.Fatal("get(a) missing")
	}
	if sid := r.sessionIDOf("b"); sid != "s2" {
		t.Errorf("sessionIDOf(b) = %q", sid)
	}

	r.mutate("a", func(e *runEntry) { e.info.Status = taskapi.RunStatusCompleted })
	if e, _ := r.get("a"); e.info.Status != taskapi.RunStatusCompleted {
		t.Errorf("mutate didn't apply: %v", e.info.Status)
	}

	if got := r.list("s1"); len(got) != 1 || got[0].RunID != "a" {
		t.Errorf("list(s1) = %+v, want [a]", got)
	}
	if got := r.list(""); len(got) != 2 {
		t.Errorf("list(all) = %d, want 2", len(got))
	}
}

// TestBgManager_LRUCap_EvictsOldestTerminalWhenOverCap — once the map
// exceeds lruCap AFTER TTL eviction, the oldest-terminalAt entries are
// dropped until the map fits. In-flight entries (terminalAt zero) are
// never evicted by the LRU pass even if the cap is exceeded.
//
// Closes tier-2 R7 LRU-half: protects against burst workloads where lots
// of runs complete inside the TTL window. Per
// `decisions/tasks-performance-budget.md` reference scenario, 1000 is the
// production cap; tests use a small cap (cap=3 here) to exercise eviction
// without populating thousands of entries.
func TestBgManager_LRUCap_EvictsOldestTerminalWhenOverCap(t *testing.T) {
	m := &bgManager{
		entries:       make(map[string]*bgEntry),
		notifications: make(map[string][]taskapi.TaskNotification),
		retentionTTL:  1 * time.Hour, // long — keep TTL out of this test
		lruCap:        3,
	}

	// Seed 5 terminal entries with strictly increasing terminalAt timestamps
	// so the LRU oldest-first eviction order is unambiguous.
	base := time.Now().Add(-10 * time.Minute) // well within the 1h TTL
	for i := 0; i < 5; i++ {
		id := "t-" + strconv.Itoa(i)
		m.entries[id] = &bgEntry{
			record:     taskapi.TaskRecord{ID: id, Status: taskapi.TaskCompleted},
			done:       make(chan struct{}),
			terminalAt: base.Add(time.Duration(i) * time.Second),
		}
		close(m.entries[id].done)
	}
	// One in-flight entry — must survive eviction even though we're over cap.
	m.entries["alive"] = &bgEntry{
		record: taskapi.TaskRecord{ID: "alive", Status: taskapi.TaskRunning},
		done:   make(chan struct{}),
	}

	m.mu.Lock()
	m.pruneStaleLocked(time.Now())
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	if got := len(m.entries); got > 3 {
		t.Fatalf("LRU cap=3 not enforced: got %d entries", got)
	}
	if _, ok := m.entries["alive"]; !ok {
		t.Fatal("LRU evicted the in-flight entry (must never happen)")
	}
	// 6 entries (5 terminal + 1 in-flight) with cap=3 → evict 3 oldest
	// terminal entries (t-0, t-1, t-2). Survivors: t-3, t-4, alive.
	for _, id := range []string{"t-0", "t-1", "t-2"} {
		if _, ok := m.entries[id]; ok {
			t.Errorf("LRU should have evicted older entry %q (cap=3 with 6 total)", id)
		}
	}
	for _, id := range []string{"t-3", "t-4"} {
		if _, ok := m.entries[id]; !ok {
			t.Errorf("newer entry %q should survive LRU eviction", id)
		}
	}
}

// TestBgManager_LRUCap_NoOpUnderCap — when the map is at-or-below cap,
// the LRU pass is a no-op (no entries evicted).
func TestBgManager_LRUCap_NoOpUnderCap(t *testing.T) {
	m := &bgManager{
		entries:       make(map[string]*bgEntry),
		notifications: make(map[string][]taskapi.TaskNotification),
		retentionTTL:  1 * time.Hour,
		lruCap:        10,
	}
	base := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 3; i++ {
		id := "t-" + strconv.Itoa(i)
		m.entries[id] = &bgEntry{
			record:     taskapi.TaskRecord{ID: id, Status: taskapi.TaskCompleted},
			done:       make(chan struct{}),
			terminalAt: base.Add(time.Duration(i) * time.Second),
		}
	}
	m.mu.Lock()
	m.pruneStaleLocked(time.Now())
	m.mu.Unlock()
	if got := len(m.entries); got != 3 {
		t.Errorf("3 entries under cap=10 should all survive; got %d", got)
	}
}

// TestLocalSpawner_LRUCap_EvictsOldestTerminal — mirror of the bgManager
// LRU test for the spawner's children map. Confirms the shared
// pruneAndCap helper enforces the cap consistently across all 3 maps.
func TestLocalSpawner_LRUCap_EvictsOldestTerminal(t *testing.T) {
	s := &LocalSpawner{
		children:     make(map[string]*childEntry),
		retentionTTL: 1 * time.Hour,
		lruCap:       2,
	}
	base := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 4; i++ {
		id := "c-" + strconv.Itoa(i)
		s.children[id] = &childEntry{
			done:       make(chan struct{}),
			terminalAt: base.Add(time.Duration(i) * time.Second),
		}
	}
	s.mu.Lock()
	s.pruneStaleChildrenLocked(time.Now())
	s.mu.Unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if got := len(s.children); got > 2 {
		t.Fatalf("LRU cap=2 not enforced on spawner: got %d entries", got)
	}
	if _, ok := s.children["c-0"]; ok {
		t.Errorf("spawner LRU should have evicted oldest c-0")
	}
	if _, ok := s.children["c-3"]; !ok {
		t.Errorf("newest entry c-3 should survive LRU eviction")
	}
}

// TestService_EmitQueueDepth_PublishesEvent verifies the periodic
// queue-depth metric: subscribe to `engine.RunQueueDepthTopic()`, call
// emitQueueDepth, expect a `RunQueueDepthEvent` with payload counts
// matching the actual map sizes.
//
// Doesn't exercise the goroutine itself (too flaky — sleeps would slow
// the test down and timing assertions are noisy). The goroutine just
// calls emitQueueDepth on a ticker; this test pins the per-tick behavior.
func TestService_EmitQueueDepth_PublishesEvent(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	bgMgr := NewBackgroundTaskManager(channel)
	svc := &Service{
		channel:        channel,
		bgMgr:          bgMgr,
		reg:            newRunRegistry(),
		metricInterval: 0, // suppress the loop — this test pulls directly
		metricStop:     make(chan struct{}),
	}

	// Populate 3 runs + 2 background entries.
	for i := 0; i < 3; i++ {
		id := "run-" + strconv.Itoa(i)
		svc.reg.runs[id] = &runEntry{done: make(chan struct{})}
	}
	bg := bgMgr.(*bgManager)
	for i := 0; i < 2; i++ {
		id := "bg-" + strconv.Itoa(i)
		bg.entries[id] = &bgEntry{
			record: taskapi.TaskRecord{ID: id, Status: taskapi.TaskRunning},
			done:   make(chan struct{}),
		}
	}

	// Subscribe BEFORE emit. MemoryEventBus is synchronous so the callback
	// fires inline during Publish — no sleeps needed.
	var (
		mu      sync.Mutex
		got     *engine.RunQueueDepthEvent
		gotType string
	)
	unsub := bus.Subscribe(engine.RunQueueDepthTopic(), func(topic string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		gotType = topic
		if evt, ok := payload.(*engine.RunQueueDepthEvent); ok {
			got = evt
		}
	})
	defer unsub()

	svc.emitQueueDepth()

	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatalf("expected RunQueueDepthEvent payload, got type=%q payload=nil", gotType)
	}
	if got.Runs != 3 {
		t.Errorf("Runs = %d, want 3", got.Runs)
	}
	if got.BackgroundEntries != 2 {
		t.Errorf("BackgroundEntries = %d, want 2", got.BackgroundEntries)
	}
	// SpawnerChildren is sampled per-workflow, not service-level — emit
	// reports 0 from the service-level path.
	if got.SpawnerChildren != 0 {
		t.Errorf("SpawnerChildren = %d, want 0 (service-level sample)", got.SpawnerChildren)
	}
}
