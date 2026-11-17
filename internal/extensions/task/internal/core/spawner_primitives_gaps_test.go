// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	core "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
)

// D3 follow-on: spawner-dependent engine-primitive UC tests landed here
// with the LocalSpawner relocation. The non-spawner tests stay in
// api/engine/engine_primitives_gaps_test.go.

// registerSimulatorOnceForCore is the package-local copy of the simulator
// registration helper. core_test can't reuse the one in api/engine since
// it's a different test package; both rely on idempotent
// engine.MachineDomain.Types.Contribute so registering from two test
// packages in the same binary is safe.
var registerSimulatorOnceForCore sync.Once

func registerSimulatorForGapTests(t *testing.T) {
	t.Helper()
	registerSimulatorOnceForCore.Do(func() {
		_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
		rawFactory, err := createFn(context.Background())
		if err != nil {
			t.Fatalf("simulator factory: %v", err)
		}
		td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", rawFactory)
		_ = engine.MachineDomain.Types.Contribute(td)
	})
}

// TestCancel_EmitsCancelEvent (UC-015)
func TestCancel_EmitsCancelEvent(t *testing.T) {
	registerSimulatorForGapTests(t)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := core.NewLocalSpawner(channel)

	var (
		mu        sync.Mutex
		reason    string
		gotCancel bool
	)
	bus.Subscribe(engine.ChildCancelTopic("cancel-target"), func(_ string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		gotCancel = true
		if s, ok := payload.(string); ok {
			reason = s
		}
	})

	handle, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "cancel-target",
		Config: map[string]any{
			"duration_ms": 200,
			"output":      "should-not-reach",
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if err := spawner.Cancel(handle.RunID, "uc-015-test-reason"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !gotCancel {
		t.Error("UC-015: cancel event not received on bus")
	}
	if !strings.Contains(reason, "uc-015") {
		t.Errorf("cancel payload = %q, want to contain the supplied reason", reason)
	}
}

// TestResult_AfterCompletion (UC-016)
func TestResult_AfterCompletion(t *testing.T) {
	registerSimulatorForGapTests(t)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := core.NewLocalSpawner(channel)

	done := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("uc-016-child"), func(_ string, _ any) {
		select {
		case done <- struct{}{}:
		default:
		}
	})

	handle, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "uc-016-child",
		Config: map[string]any{
			"duration_ms": 0,
			"output":      map[string]any{"uc": "uc-016", "value": 42},
		},
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	if _, ok := spawner.Result("run-doesnotexist"); ok {
		t.Error("Result on unknown runID should return ok=false")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("child didn't complete within 2s")
	}

	got, ok := spawner.Result(handle.RunID)
	if !ok {
		t.Fatal("Result(completed runID) returned ok=false")
	}
	m, _ := got.(map[string]any)
	if m == nil || m["uc"] != "uc-016" {
		t.Errorf("Result = %+v, want map with uc=uc-016", got)
	}
}

// TestChildDone_Emit (UC-017)
func TestChildDone_Emit(t *testing.T) {
	registerSimulatorForGapTests(t)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := core.NewLocalSpawner(channel)

	doneCh := make(chan any, 1)
	bus.Subscribe(engine.ChildDoneTopic("uc-017-child"), func(_ string, payload any) {
		select {
		case doneCh <- payload:
		default:
		}
	})

	if _, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "uc-017-child",
		Config: map[string]any{
			"duration_ms": 0,
			"output":      "uc-017-output",
		},
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case payload := <-doneCh:
		evt, ok := payload.(*engine.ChildDoneEvent)
		if !ok {
			t.Fatalf("UC-017: child.done payload type = %T, want *ChildDoneEvent", payload)
		}
		if evt.ChildName != "uc-017-child" {
			t.Errorf("ChildName = %q", evt.ChildName)
		}
		if evt.Output != "uc-017-output" {
			t.Errorf("Output = %v, want uc-017-output", evt.Output)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UC-017: child.done not emitted within 2s")
	}
}

// TestChildFailed_Emit (UC-018)
func TestChildFailed_Emit(t *testing.T) {
	registerSimulatorForGapTests(t)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := core.NewLocalSpawner(channel)

	failCh := make(chan any, 1)
	bus.Subscribe(engine.ChildFailedTopic("uc-018-child"), func(_ string, payload any) {
		select {
		case failCh <- payload:
		default:
		}
	})

	if _, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "uc-018-child",
		Config: map[string]any{
			"duration_ms":      0,
			"fail_probability": 1.0,
			"fail_message":     "intentional uc-018 failure",
		},
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case payload := <-failCh:
		evt, ok := payload.(*engine.ChildFailedEvent)
		if !ok {
			t.Fatalf("UC-018: child.failed payload type = %T, want *ChildFailedEvent", payload)
		}
		if !strings.Contains(evt.Error, "uc-018") {
			t.Errorf("Error = %q, want to contain uc-018", evt.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UC-018: child.failed not emitted within 2s")
	}
}
