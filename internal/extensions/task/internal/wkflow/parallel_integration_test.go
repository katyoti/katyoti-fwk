// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

var _ = strategies.StrategyDef[engine.MachineFactory]{} // ensure strategies import is used

// stepCounter counts a run's steps.
type stepCounter struct {
	engine.NoopHook
	n atomic.Int64
}

func (c *stepCounter) OnPostStep(context.Context, *engine.State, *engine.StepResult, error) {
	c.n.Add(1)
}

// TestWorkflowParallel_AllChildrenDoneViaSimulator runs an actual workflow
// with a sequence â†’ parallel â†’ 3 simulator children + return, end-to-end via
// the executor + spawner + bus. Reproduces the hang reported when running
// `demo-parallel-fallback` from the CLI: parallel handler returns Wait==nil
// after each child.done event, expecting the executor to deliver the next
// queued one. Before the executor TryGet fix, this test would deadlock.
func TestWorkflowParallel_AllChildrenDoneViaSimulator(t *testing.T) {
	// Register the simulator type in the global MachineDomain so the
	// LocalSpawner.Spawn can look it up by name "simulator". The Strategy's
	// CreateTypeDef wraps the factory the strategy creates.
	_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
	rawFactory, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("simulator factory: %v", err)
	}
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", rawFactory)
	// Idempotent: another test in this binary may have already registered it.
	_ = engine.MachineDomain.Types.Contribute(td)

	// Build a workflow config with: sequence â†’ parallel(3 simulator branches) â†’ return.
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "fanout",
				"kind": "parallel",
				"branches": []any{
					map[string]any{
						"name": "a",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 50,
							"output":      map[string]any{"data": "a-ok"},
						},
					},
					map[string]any{
						"name": "b",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 30,
							"output":      map[string]any{"data": "b-ok"},
						},
					},
					map[string]any{
						"name": "c",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 20,
							"output":      map[string]any{"data": "c-ok"},
						},
					},
				},
			},
			map[string]any{
				"kind":  "return",
				"value": "${results}", // Mode B: evaluate, not the literal "results"
			},
		},
	}

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: cfg})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	exec := engine.NewExecutor(m, channel, nil)
	steps := &stepCounter{}
	exec.RegisterHook(steps)

	doneCh := make(chan struct {
		result *engine.RunResult
		err    error
	}, 1)
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		r, e := exec.Run(runCtx, engine.Event{Type: "start"})
		doneCh <- struct {
			result *engine.RunResult
			err    error
		}{r, e}
	}()

	select {
	case d := <-doneCh:
		if d.err != nil {
			t.Fatalf("workflow run error: %v", d.err)
		}
		if d.result == nil {
			t.Fatal("nil result")
		}
		if d.result.Next != engine.Done {
			t.Fatalf("expected Done, got %v", d.result.Next)
		}
		// The return must yield the actual results map (proves ${results}
		// evaluated AND all three branches landed) — not the literal string.
		out, ok := d.result.Output.(map[string]any)
		if !ok {
			t.Fatalf("output = %#v, want the results map (did ${results} evaluate?)", d.result.Output)
		}
		for _, name := range []string{"a", "b", "c"} {
			if _, present := out[name]; !present {
				t.Errorf("results missing branch %q: %#v", name, out)
			}
		}
		// Awaiting its children, the parallel waits — it does not spin (a step,
		// and a saved state, per turn of a busy loop).
		if n := steps.n.Load(); n > 20 {
			t.Errorf("%d steps for a parallel of 3; it spun while awaiting", n)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("workflow hung — parallel awaiting children never completed")
	}
}

// TestWorkflowParallel_OnErrorFallback runs the on_error.fallback path:
// branch-c's primary always fails, fallback simulator succeeds. Mirrors the
// demo-parallel-fallback embedded task. Verifies the fallback child is
// spawned + its output flows into results under the FALLBACK child's name.
//
// HISTORY: this test originally only asserted Next==Done and t.Logf'd the
// output — and branch-level on_error was in fact DEAD inside parallel (the
// failed: path went straight to collect/fail_fast accounting). The recovery
// is real now; this test pins it.
func TestWorkflowParallel_OnErrorFallback(t *testing.T) {
	_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
	rawFactory, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("simulator factory: %v", err)
	}
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", rawFactory)
	_ = engine.MachineDomain.Types.Contribute(td) // idempotent — may already be registered

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name":            "fanout",
				"kind":            "parallel",
				"on_branch_error": "collect",
				"branches": []any{
					map[string]any{
						"name": "a",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 30,
							"output":      map[string]any{"data": "a-ok"},
						},
					},
					map[string]any{
						"name": "c",
						"task": map[string]any{
							"type":             "simulator",
							"duration_ms":      30,
							"fail_probability": 1.0,
							"fail_message":     "always fails",
						},
						"on_error": map[string]any{
							"fallback": map[string]any{
								"name": "c-fallback",
								"task": map[string]any{
									"type":        "simulator",
									"duration_ms": 20,
									"output":      map[string]any{"data": "c-recovered"},
								},
							},
						},
					},
				},
			},
			map[string]any{"kind": "return", "value": "${results}"},
		},
	}

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: cfg})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	exec := engine.NewExecutor(m, channel, nil)

	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := exec.Run(runCtx, engine.Event{Type: "start"})
	if err != nil {
		t.Fatalf("workflow run error: %v", err)
	}
	if r == nil || r.Next != engine.Done {
		t.Fatalf("expected Done, got %+v", r)
	}
	out, ok := r.Output.(map[string]any)
	if !ok {
		t.Fatalf("output = %#v, want results map", r.Output)
	}
	if a, _ := out["a"].(map[string]any); a == nil || a["data"] != "a-ok" {
		t.Errorf("results.a = %#v, want a-ok", out["a"])
	}
	fb, _ := out["c-fallback"].(map[string]any)
	if fb == nil || fb["data"] != "c-recovered" {
		t.Errorf("results.c-fallback = %#v, want the fallback recovery output (branch on_error.fallback must RUN inside parallel)", out["c-fallback"])
	}
}
