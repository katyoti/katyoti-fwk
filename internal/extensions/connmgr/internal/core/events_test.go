// SPDX-License-Identifier: Apache-2.0

package core

// FSM event-emission tests. ManagerImpl publishes
// `ConnStateChangedEvent` on every state transition when the bus is
// configured (NewManagerWithBus path). Stdio-only managers emit no
// events — verified separately.
//
// Uses the in-memory teststub instead of embedded NATS so the tests are
// fast and deterministic.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	teststub "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// eventRecorder buffers every ConnStateChangedEvent published on the
// universe-wide subject. Use it as a deterministic listener for the
// manager's emissions.
type eventRecorder struct {
	mu sync.Mutex
	ev []cmapi.ConnStateChangedEvent
}

func (r *eventRecorder) handler(_ context.Context, msg mbapi.Message) {
	var e cmapi.ConnStateChangedEvent
	if err := json.Unmarshal(msg.Data(), &e); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ev = append(r.ev, e)
}

func (r *eventRecorder) snapshot() []cmapi.ConnStateChangedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]cmapi.ConnStateChangedEvent, len(r.ev))
	copy(out, r.ev)
	return out
}

func (r *eventRecorder) waitFor(t *testing.T, count int, timeout time.Duration) []cmapi.ConnStateChangedEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := r.snapshot(); len(got) >= count {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	got := r.snapshot()
	t.Fatalf("waited %v for %d events, got %d: %+v", timeout, count, len(got), got)
	return nil
}

func subscribeRecorder(t *testing.T, bus mbapi.MessageBus, tenant, universe string) *eventRecorder {
	t.Helper()
	rec := &eventRecorder{}
	subject := subjects.ConnStateChangedSubject(tenant, universe)
	sub, err := bus.Subscribe(context.Background(), subject, rec.handler)
	if err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return rec
}

func newBusManager(t *testing.T, specs []cmapi.SpawnSpec) (*ManagerImpl, *eventRecorder) {
	t.Helper()
	bus := teststub.New()
	rec := subscribeRecorder(t, bus, "local", "test")
	m, err := NewManagerWithBus(specs, BusScope{Bus: bus, TenantID: "local", UniverseID: "test"})
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}
	return m, rec
}

func TestEvents_StdioOnlyManager_NoEvents(t *testing.T) {
	bus := teststub.New()
	rec := subscribeRecorder(t, bus, "local", "test")

	// stdio-only manager — no bus scope, so no events even though
	// someone is subscribed.
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "/path/to/missing-binary"},
	})
	_, _ = m.Acquire(context.Background(), "x") // expected to fail (missing binary)

	time.Sleep(20 * time.Millisecond)
	if got := rec.snapshot(); len(got) != 0 {
		t.Errorf("stdio-only: expected 0 events, got %d: %+v", len(got), got)
	}
}

func TestEvents_SpawnFailedEmitsTwo(t *testing.T) {
	// stdio with a missing binary → SPAWNING → FAILED. Bus-aware
	// manager should emit both transitions.
	//
	// Ordering note: the teststub dispatches each handler on its own
	// goroutine ("fully ordered per-subject is NOT guaranteed by the
	// stub"), so back-to-back publishes can be reordered at the
	// subscriber. We assert the *set* of transitions instead — both
	// expected (From, To) pairs must appear, and the FAILED event
	// must carry a reason.
	m, rec := newBusManager(t, []cmapi.SpawnSpec{
		{Name: "ghost", Transport: "stdio", Binary: "/path/that/does/not/exist"},
	})

	_, err := m.Acquire(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected spawn error, got nil")
	}

	got := rec.waitFor(t, 2, settleWait)

	var sawSpawning, sawFailed bool
	var failedReason string
	for _, e := range got {
		if e.Name != "ghost" {
			t.Errorf("Name = %q, want ghost", e.Name)
		}
		switch {
		case e.From == cmapi.ConnDeclared && e.To == cmapi.ConnSpawning:
			sawSpawning = true
		case e.From == cmapi.ConnSpawning && e.To == cmapi.ConnFailed:
			sawFailed = true
			failedReason = e.Reason
		}
	}
	if !sawSpawning {
		t.Errorf("missing DECLARED→SPAWNING transition in %+v", got)
	}
	if !sawFailed {
		t.Errorf("missing SPAWNING→FAILED transition in %+v", got)
	}
	if sawFailed && failedReason == "" {
		t.Error("FAILED event must include a reason")
	}
}

func TestEvents_NATSWithoutBusScope(t *testing.T) {
	// Stdio-only manager (no bus) but a `transport: nats` spec.
	// Acquire fails fast — no events emitted because the manager has
	// no bus to publish on.
	bus := teststub.New()
	rec := subscribeRecorder(t, bus, "local", "test")
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "nats", Binary: "x"},
	})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	time.Sleep(20 * time.Millisecond)
	if got := rec.snapshot(); len(got) != 0 {
		t.Errorf("expected 0 events from stdio-only manager, got %d", len(got))
	}
}

func TestEvents_IdleDisconnectEmitsBothTransitions(t *testing.T) {
	// Synthesise a READY connection (skip the actual spawn) and trigger
	// the idle path. Events expected (any delivery order — the teststub
	// doesn't guarantee strict ordering when wall-clock timestamps
	// collide on back-to-back emissions):
	//   READY → IDLE_DRAINING (reason: idle)
	//   IDLE_DRAINING → DECLARED (reason: drained)
	m, rec := newBusManager(t, []cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: 10 * time.Millisecond},
	})
	c := m.conns["x"]
	c.mu.Lock()
	c.state = cmapi.ConnReady
	c.refCount = 1
	c.mu.Unlock()

	m.Release("x") // refCount → 0 → arms idle timer

	got := rec.waitFor(t, 2, settleWait)
	have := map[string]bool{}
	for _, e := range got {
		if e.Name != "x" {
			t.Errorf("unexpected Name %q in event %+v", e.Name, e)
		}
		have[string(e.To)+":"+e.Reason] = true
	}
	if !have["IDLE_DRAINING:idle"] {
		t.Errorf("missing READY→IDLE_DRAINING/idle event; got %+v", got)
	}
	if !have["DECLARED:drained"] {
		t.Errorf("missing IDLE_DRAINING→DECLARED/drained event; got %+v", got)
	}
}

func TestEvents_CloseEmitsForLiveEntries(t *testing.T) {
	m, rec := newBusManager(t, []cmapi.SpawnSpec{
		{Name: "live", Transport: "stdio", Binary: "x"},
		{Name: "dormant", Transport: "stdio", Binary: "y"},
	})
	// Synthesise "live" as READY, leave "dormant" in DECLARED.
	live := m.conns["live"]
	live.mu.Lock()
	live.state = cmapi.ConnReady
	live.mu.Unlock()

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// One event expected — only the live entry transitions.
	got := rec.waitFor(t, 1, settleWait)
	if got[0].Name != "live" {
		t.Errorf("expected 'live' event, got %+v", got[0])
	}
	if got[0].To != cmapi.ConnDeclared || got[0].Reason != cmapi.ReasonClose {
		t.Errorf("expected close transition, got %+v", got[0])
	}
}

func TestEvents_PayloadShape(t *testing.T) {
	m, rec := newBusManager(t, []cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "/path/that/does/not/exist"},
	})
	_, _ = m.Acquire(context.Background(), "x")

	got := rec.waitFor(t, 2, settleWait)
	for i, e := range got {
		if e.Name != "x" {
			t.Errorf("[%d] Name = %q, want x", i, e.Name)
		}
		if e.At.IsZero() {
			t.Errorf("[%d] At is zero — must be a real timestamp", i)
		}
		if !e.At.Before(time.Now().Add(time.Second)) {
			t.Errorf("[%d] At in the future: %v", i, e.At)
		}
	}
}
