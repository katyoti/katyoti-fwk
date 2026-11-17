// SPDX-License-Identifier: Apache-2.0

package engine_test

import (
	"context"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// Engine primitives — gap closures (tier-1.3, batch 2).
// One test function per UC; small, focused, no shared state.

// TestSubscribe_UnsubStops (UC-005) — Subscribe returns an unsub closure; once
// called, the handler no longer fires. Asymmetric to TestEventBus_PubSub
// (UC-004) which exercises fan-out: this isolates the unsub guarantee so a
// regression flips this UC red specifically.
func TestSubscribe_UnsubStops(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	var hits int
	unsub := bus.Subscribe("uc-005", func(_ string, _ any) { hits++ })
	bus.Publish("uc-005", "first")
	if hits != 1 {
		t.Fatalf("hits = %d before unsub, want 1", hits)
	}
	unsub()
	bus.Publish("uc-005", "second")
	if hits != 1 {
		t.Errorf("hits = %d after unsub, want 1 (unsub didn't stop delivery)", hits)
	}
}

// TestGet_Blocks_UntilMatch (UC-007) — MachineInbox.Get blocks until a
// matching event arrives, then returns it.
func TestGet_Blocks_UntilMatch(t *testing.T) {
	inbox := engine.NewMachineInbox()
	spec := &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: "uc-007", Label: "go"}}}

	go func() {
		time.Sleep(10 * time.Millisecond)
		inbox.Push(engine.Event{Type: "uc-007", Payload: "hello"})
	}()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got, err := inbox.Get(ctx, spec)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 || got[0].Payload != "hello" {
		t.Errorf("got = %+v, want one event with payload hello", got)
	}
	if dur < 5*time.Millisecond {
		t.Errorf("Get returned in %s — should have blocked at least until the push", dur)
	}
	if dur > 200*time.Millisecond {
		t.Errorf("Get took %s — should have returned promptly after the push", dur)
	}
}

// TestGet_Predicate (UC-008) — conditional matching honors topic AND
// payload Filter. Events whose payload-map doesn't match the filter stay in
// the queue; the first satisfying one wins.
func TestGet_Predicate(t *testing.T) {
	inbox := engine.NewMachineInbox()
	spec := &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{
		{Topic: "uc-008", Label: "match", Filter: map[string]string{"phase": "want"}},
	}}

	inbox.Push(engine.Event{Type: "uc-008", Payload: map[string]any{"phase": "skip"}})
	inbox.Push(engine.Event{Type: "uc-008", Payload: map[string]any{"phase": "want"}})
	inbox.Push(engine.Event{Type: "uc-008", Payload: map[string]any{"phase": "skip"}})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	got, err := inbox.Get(ctx, spec)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("matches = %d, want exactly 1 (Filter selects)", len(got))
	}
	m, _ := got[0].Payload.(map[string]any)
	if m == nil || m["phase"] != "want" {
		t.Errorf("matched payload = %+v, want phase=want", got[0].Payload)
	}
}

// TestLoad_ByRunID (UC-010) — Load by known runID returns a checkpoint
// equal to the saved one; load by unknown runID returns an error.
func TestLoad_ByRunID(t *testing.T) {
	store := engine.NewMemoryStateStore()
	original := engine.Checkpoint{
		State: engine.State{Name: "phase-a", RunID: "run-uc010", Version: 7},
		Event: engine.Event{Type: "tick", Payload: map[string]any{"k": "v"}},
	}
	if err := store.Save("run-uc010", original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load("run-uc010")
	if err != nil {
		t.Fatalf("Load known: %v", err)
	}
	if got.State.Name != original.State.Name || got.State.Version != original.State.Version {
		t.Errorf("loaded state = %+v, want %+v", got.State, original.State)
	}
	if _, err := store.Load("run-doesnotexist"); err == nil {
		t.Error("expected error when loading unknown runID")
	}
}

// TestNopStateStore_NoOps (UC-011) — NopStateStore Save is a silent no-op;
// Load surfaces a not-found error so the executor knows to start fresh.
func TestNopStateStore_NoOps(t *testing.T) {
	store := &engine.NopStateStore{}
	if err := store.Save("any-run", engine.Checkpoint{State: engine.State{Name: "x"}}); err != nil {
		t.Errorf("Nop.Save returned %v, want nil", err)
	}
	if _, err := store.Load("any-run"); err == nil {
		t.Error("Nop.Load expected not-found error, got nil")
	}
}
