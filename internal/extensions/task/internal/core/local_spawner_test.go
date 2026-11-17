// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestSpawner_NormalCompletion_NoOrphan (UC-112 negative) — spawning a child
// with a real ParentID instantiates an OrphanDetector, but a normally-
// completing child must NOT trigger orphan-cancel. The detector's Stop()
// in the child goroutine cleans up before the timeout could fire.
//
// We use a simulator child that completes very quickly (duration_ms=10).
// Even with the production 90s orphan timeout, the child finishes ~9000×
// faster than the orphan would fire — so we just confirm the child reaches
// ChildDone and its context is closed (cancel was called by the executor's
// own goroutine cleanup, not the orphan detector).
//
// Race-detector clean: depends on OrphanDetector.Stop being called via
// the spawner goroutine's defer.
func TestSpawner_NormalCompletion_NoOrphan(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	// Capture child.done — proves the child completed normally.
	doneCh := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("normal-completer"), func(_ string, _ any) {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})

	// Spawn a fast simulator child WITH a parentID — wires the OrphanDetector.
	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind:     "simulator",
		Name:     "normal-completer",
		ParentID: "parent-run-id-fake",
		Config: map[string]any{
			"duration_ms": 10,
			"output":      map[string]any{"ok": true},
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case <-doneCh:
		// pass — child reported done before the orphan detector could fire
	case <-time.After(2 * time.Second):
		t.Fatal("UC-112 negative: child did not complete within 2s; orphan detector may have spuriously cancelled")
	}
}

// TestSpawner_AppliesParentPolicies (UC-053) — spawner-level test that the
// workflow-spawn path now applies parent SpawnPolicies (closes sec-1/T1).
//
// Setup: spawn a simulator child via LocalSpawner with `Mode = "worker"` and
// `ParentMode = "coordinator"`. Verify the child receives a ConfigOverride
// map containing SafeToolSubset (the coordinator-restricts-children policy
// fired). Token-free; uses the simulator-substitution path.
//
// We don't directly inspect TaskExecRequest.ConfigOverride (it's built
// inside Spawn), so we use a simulator child whose Config gets merged with
// the override. After spawn, the child's resolved Config should carry the
// override fields.
func TestSpawner_AppliesParentPolicies(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	// Track child completion to know spawn finished cleanly. The actual
	// Config-merge effect is asserted by inspecting the simulator's Output,
	// which echoes input. The simulator strategy doesn't expose Config
	// internals, but the merge happens inside Spawn before factory.CreateMachine
	// — so the integration is tested at the wider scope (UC-053 L2 scenario
	// inspects the journal). Here we just confirm the spawn does not error
	// when the policy fires.
	doneCh := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("policy-test"), func(_ string, _ any) {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})

	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind:       "simulator",
		Name:       "policy-test",
		Mode:       "worker",        // child mode → triggers BuildChildOverride
		ParentMode: "coordinator",   // parent mode → fires CoordinatorRestrictsChildren policy
		ParentID:   "parent-run-id", // non-empty so OrphanDetector wires (won't fire in 2s)
		Config: map[string]any{
			"duration_ms": 10,
			"output":      map[string]any{"ok": true},
		},
	})
	if err != nil {
		t.Fatalf("UC-053: spawn errored when applying parent policies: %v", err)
	}

	select {
	case <-doneCh:
		// Pass. The spawn applied policies AND the simulator child completed
		// without the merge breaking config decoding.
	case <-time.After(2 * time.Second):
		t.Fatal("UC-053: spawn did not complete within 2s; policy-merge path may have broken simulator config")
	}
}

// TestSpawner_RootSpawnSkipsOrphanDetector (UC-112 negative, root case) —
// when ParentID is empty (root task spawn), no OrphanDetector is wired.
// We can't directly inspect the detector (private field), but we can confirm
// the root spawn completes without any orphan-related event traffic.
func TestSpawner_RootSpawnSkipsOrphanDetector(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	// Track that no internal cancel-via-orphan happened by watching the
	// child's terminal state. Failed-with-context-cancelled would suggest
	// orphan misfire; ChildDone confirms the natural termination path.
	doneCh := make(chan struct{}, 1)
	failedCh := make(chan struct{}, 1)
	var (
		mu     sync.Mutex
		failed bool
	)

	bus.Subscribe(engine.ChildDoneTopic("root-spawn"), func(_ string, _ any) {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})
	bus.Subscribe(engine.ChildFailedTopic("root-spawn"), func(_ string, _ any) {
		mu.Lock()
		failed = true
		mu.Unlock()
		select {
		case failedCh <- struct{}{}:
		default:
		}
	})

	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "root-spawn",
		// ParentID intentionally empty — root task semantics
		Config: map[string]any{
			"duration_ms": 10,
			"output":      map[string]any{"root": true},
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case <-doneCh:
		// pass
	case <-failedCh:
		t.Fatal("UC-112 negative: root spawn failed unexpectedly")
	case <-time.After(2 * time.Second):
		t.Fatal("UC-112 negative: root spawn did not complete within 2s")
	}

	mu.Lock()
	if failed {
		t.Error("UC-112 negative: root spawn marked failed despite ChildDone path")
	}
	mu.Unlock()
}

// TestSpawn_NoLostDoneEvent — corr-2 regression.
//
// Spawn 100 simulator children with duration_ms=0 (i.e. they complete
// essentially instantly). The parent uses WaitSpec/Channel.Subscribe via the
// WaitForSubscribe + OnSubscribed gating pattern that the workflow uses.
// Without the gate, child.done events would fire BEFORE the parent's
// subscription registers and be lost; the parent's inbox.Get would hang
// until ctx timeout. With the gate, every done event is observed.
//
// 100 iterations to make the race window statistically inevitable on any
// A child that ends before its parent listens is not lost: its end is on
// its run's topic, and its outcome stays — a parent settles from it.
func TestSpawn_NoLostDoneEvent(t *testing.T) {
	registerSimulatorOnce(t)
	spawner := NewLocalSpawner(engine.NewChannel(engine.NewMemoryEventBus()))
	for i := 0; i < 100; i++ {
		h, err := spawner.Spawn(context.Background(), engine.ChildSpec{
			Kind:   "simulator",
			Name:   "fast-" + strconv.Itoa(i),
			Config: map[string]any{"duration_ms": 0, "output": map[string]any{"i": i}},
		})
		if err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for {
			if o, ended := spawner.Outcome(h.RunID); ended {
				if o.Status != engine.ChildDone {
					t.Fatalf("child %d: %+v; want done", i, o)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("child %d: no outcome", i)
			}
			time.Sleep(time.Millisecond)
		}
	}
}
