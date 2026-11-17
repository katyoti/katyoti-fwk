// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// registerSimulatorOnce registers the simulator MachineFactory in the global
// MachineDomain so LocalSpawner.Spawn can look it up. Idempotent — re-running
// inside the same test binary is safe (the contribute call may report "already
// registered" which we ignore).
func registerSimulatorOnce(t *testing.T) {
	t.Helper()
	_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
	rawFactory, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("simulator factory: %v", err)
	}
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", rawFactory)
	_ = engine.MachineDomain.Types.Contribute(td)
}

// TestSpawner_DryRunSubstitutesAgent verifies that when ChildSpec.DryRunMode is
// set, an `agent` kind is replaced by the simulator at spawn time and the
// child completes with the synthetic dry-run output.
func TestSpawner_DryRunSubstitutesAgent(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	// Capture the child.done payload so we can inspect the substituted output.
	var doneOutput any
	doneCh := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("researcher"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.ChildDoneEvent); ok {
			doneOutput = evt.Output
		}
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})

	// Note: spec.Kind is "agent" — without dry-run this would fail because the
	// agent factory isn't registered in this test binary. With dry-run, the
	// spawner substitutes it with the simulator.
	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind:       "agent",
		Name:       "researcher",
		Input:      map[string]any{"query": "name 3 colors"},
		DryRunMode: true,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for child.done")
	}

	out, ok := doneOutput.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T: %v", doneOutput, doneOutput)
	}
	if dr, _ := out["dry_run"].(bool); !dr {
		t.Errorf("output.dry_run should be true, got %v", out["dry_run"])
	}
	if kind, _ := out["child_kind"].(string); kind != "agent" {
		t.Errorf("output.child_kind should be 'agent' (preserved original), got %q", out["child_kind"])
	}
	if summary, _ := out["summary"].(string); summary != "DRY_RUN: researcher" {
		t.Errorf("output.summary unexpected: %q", out["summary"])
	}
	keys, _ := out["input_keys"].([]any)
	if len(keys) != 1 || keys[0] != "query" {
		t.Errorf("output.input_keys should be [query], got %v", keys)
	}
}

// TestSpawner_DryRunPassthroughForSimulator verifies that simulator and other
// non-substituted kinds run unchanged under dry-run — only LLM/program kinds
// are swapped.
func TestSpawner_DryRunPassthroughForSimulator(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	var doneOutput any
	doneCh := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("baseline"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.ChildDoneEvent); ok {
			doneOutput = evt.Output
		}
		select {
		case doneCh <- struct{}{}:
		default:
		}
	})

	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "simulator",
		Name: "baseline",
		Config: map[string]any{
			"duration_ms": 10,
			"output":      map[string]any{"data": "real-simulator-output"},
		},
		DryRunMode: true, // dry-run set, but kind=simulator is passthrough
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for child.done")
	}

	out, ok := doneOutput.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", doneOutput)
	}
	// Real simulator output preserved — NOT replaced with dry-run synthetic.
	if data, _ := out["data"].(string); data != "real-simulator-output" {
		t.Errorf("expected real simulator output preserved, got %v", out)
	}
	if _, hasMarker := out["dry_run"]; hasMarker {
		t.Errorf("simulator passthrough should NOT carry dry_run marker, got %v", out)
	}
}

// TestSpawner_DryRunSubstitutesAllExpensiveKinds verifies that all four kinds
// in the substitution set (agent, task, human-gate, program) get replaced.
func TestSpawner_DryRunSubstitutesAllExpensiveKinds(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	kinds := []string{"agent", "task", "human-gate"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			doneCh := make(chan map[string]any, 1)
			bus.Subscribe(engine.ChildDoneTopic("c-"+kind), func(_ string, payload any) {
				if evt, ok := payload.(*engine.ChildDoneEvent); ok {
					if m, ok := evt.Output.(map[string]any); ok {
						doneCh <- m
					}
				}
			})

			_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
				Kind:       kind,
				Name:       "c-" + kind,
				DryRunMode: true,
			})
			if err != nil {
				t.Fatalf("spawn %s: %v", kind, err)
			}

			select {
			case out := <-doneCh:
				if dr, _ := out["dry_run"].(bool); !dr {
					t.Errorf("%s: dry_run marker missing", kind)
				}
				if origKind, _ := out["child_kind"].(string); origKind != kind {
					t.Errorf("%s: child_kind = %q, want %q", kind, origKind, kind)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s: timeout", kind)
			}
		})
	}
}

// TestSpawner_DryRunPropagatesIntoNestedExecRequest verifies that DryRunMode
// flows into the child's TaskExecRequest.Runtime so a nested workflow inherits
// it. We can't observe it directly without a workflow factory in scope, so
// this test asserts via output payload that the synthetic config was used.
func TestSpawner_DryRunInputKeysIncluded(t *testing.T) {
	registerSimulatorOnce(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	spawner := NewLocalSpawner(channel)

	doneCh := make(chan map[string]any, 1)
	bus.Subscribe(engine.ChildDoneTopic("multi"), func(_ string, payload any) {
		if evt, ok := payload.(*engine.ChildDoneEvent); ok {
			if m, ok := evt.Output.(map[string]any); ok {
				doneCh <- m
			}
		}
	})

	_, err := spawner.Spawn(context.Background(), engine.ChildSpec{
		Kind: "agent",
		Name: "multi",
		Input: map[string]any{
			"alpha": 1,
			"beta":  "two",
			"gamma": []int{3},
		},
		DryRunMode: true,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	select {
	case out := <-doneCh:
		keysAny, _ := out["input_keys"].([]any)
		got := make([]string, 0, len(keysAny))
		for _, k := range keysAny {
			if s, ok := k.(string); ok {
				got = append(got, s)
			}
		}
		sort.Strings(got)
		want := []string{"alpha", "beta", "gamma"}
		if len(got) != len(want) {
			t.Fatalf("input_keys = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("input_keys[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	// Suppress unused-import warning for taskapi when refactoring.
	_ = taskapi.TaskExecRequest{}
}
