// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// registerSimulator registers the simulator factory in MachineDomain.
// Idempotent — re-running inside the same test binary is safe.
func registerSimulator(t *testing.T) {
	t.Helper()
	_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
	rawFactory, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("simulator factory: %v", err)
	}
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", rawFactory)
	_ = engine.MachineDomain.Types.Contribute(td)
}

// runWorkflowDryRun runs the workflow described by `cfg` under dry-run mode and
// returns the captured ChildDoneEvent payloads keyed by child name. Fails the
// test on timeout or workflow error.
// runWorkflowDryRun runs the workflow and answers the outputs of its children's
// done events — waiting for those of the children named in expect: the
// spawner ends a child's run (which wakes its parent) before it publishes the
// child's done event, so the run may finish before the last ones arrive.
func runWorkflowDryRun(t *testing.T, cfg map[string]any, timeout time.Duration, expect ...string) (map[string]map[string]any, *engine.RunResult) {
	t.Helper()

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	doneEvents := make(map[string]map[string]any)
	var doneMu = make(chan struct{}, 1)
	doneMu <- struct{}{} // semaphore guard
	bus.SubscribeAny(func(topic string, payload any) {
		if evt, ok := payload.(*engine.ChildDoneEvent); ok {
			<-doneMu
			if m, ok := evt.Output.(map[string]any); ok {
				doneEvents[evt.ChildName] = m
			}
			doneMu <- struct{}{}
		}
	})

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{
		Config:  cfg,
		Runtime: taskapi.RuntimeContext{DryRunMode: true},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	exec := engine.NewExecutor(m, channel, nil)

	resultCh := make(chan *engine.RunResult, 1)
	errCh := make(chan error, 1)
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		r, e := exec.Run(runCtx, engine.Event{Type: "start"})
		resultCh <- r
		errCh <- e
	}()

	select {
	case r := <-resultCh:
		err := <-errCh
		if err != nil {
			t.Fatalf("workflow run error: %v", err)
		}
		deadline := time.Now().Add(timeout)
		for {
			<-doneMu
			seen := make(map[string]map[string]any, len(doneEvents))
			for k, v := range doneEvents {
				seen[k] = v
			}
			doneMu <- struct{}{}
			missing := false
			for _, name := range expect {
				if _, ok := seen[name]; !ok {
					missing = true
				}
			}
			if !missing || time.Now().After(deadline) {
				return seen, r
			}
			time.Sleep(10 * time.Millisecond)
		}
	case <-time.After(timeout + time.Second):
		t.Fatal("workflow hung past timeout")
	}
	return nil, nil
}

// TestDryRun_SequentialAgentPipeline mirrors demo-pipeline-agents: a workflow
// with 3 sequential agent steps. Under dry-run, each agent → simulator with
// the synthetic dry_run output.
func TestDryRun_SequentialAgentPipeline(t *testing.T) {
	registerSimulator(t)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "researcher",
				"task": map[string]any{
					"type":   "task",
					"prompt": "name 3 colors",
				},
			},
			map[string]any{
				"name": "analyst",
				"task": map[string]any{
					"type":   "task",
					"prompt": "categorize them",
				},
			},
			map[string]any{
				"name": "writer",
				"task": map[string]any{
					"type":   "task",
					"prompt": "compose a sentence",
				},
			},
			map[string]any{"kind": "return", "value": "results"},
		},
	}

	doneEvents, r := runWorkflowDryRun(t, cfg, 3*time.Second, "researcher", "analyst", "writer")

	if r.Next != engine.Done {
		t.Fatalf("expected Done, got %v", r.Next)
	}
	for _, name := range []string{"researcher", "analyst", "writer"} {
		out, ok := doneEvents[name]
		if !ok {
			t.Errorf("missing child.done for %q", name)
			continue
		}
		if dr, _ := out["dry_run"].(bool); !dr {
			t.Errorf("%s: dry_run marker missing", name)
		}
		if origKind, _ := out["child_kind"].(string); origKind != "task" {
			t.Errorf("%s: original kind = %q, want 'task'", name, origKind)
		}
	}
}

// TestDryRun_ParallelMixedKinds mirrors demo-magentic-pattern's topology: a
// parallel block with one ledger (passthrough) + 3 agents (substituted).
// Verifies (a) ledger is NOT substituted (stays as ledger machine) and
// (b) all 3 agents are substituted with the dry_run marker.
func TestDryRun_ParallelMixedKinds(t *testing.T) {
	registerSimulator(t)

	// Register a minimal "ledger"-named factory backed by simulator so this
	// test doesn't need to import the ledger package (avoids import cycles).
	// We just need a non-substituted kind in the mix.
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("test-passthrough", func() engine.MachineFactory {
		_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
		f, _ := createFn(context.Background())
		return f
	}())
	_ = engine.MachineDomain.Types.Contribute(td)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name":            "team",
				"kind":            "parallel",
				"on_branch_error": "collect",
				"branches": []any{
					map[string]any{
						"name": "shared-state",
						"task": map[string]any{
							"type":        "test-passthrough",
							"duration_ms": 10,
							"output":      map[string]any{"role": "passthrough"},
						},
					},
					map[string]any{
						"name": "manager",
						"task": map[string]any{
							"type": "agent",
						},
					},
					map[string]any{
						"name": "alice",
						"task": map[string]any{
							"type": "agent",
						},
					},
				},
			},
			map[string]any{"kind": "return", "value": "results"},
		},
	}

	doneEvents, r := runWorkflowDryRun(t, cfg, 3*time.Second, "shared-state", "manager", "alice")

	if r.Next != engine.Done {
		t.Fatalf("expected Done, got %v", r.Next)
	}

	// Passthrough child: real output, no dry_run marker.
	passthrough, ok := doneEvents["shared-state"]
	if !ok {
		t.Fatal("missing child.done for shared-state")
	}
	if role, _ := passthrough["role"].(string); role != "passthrough" {
		t.Errorf("shared-state: real output preserved, got %v", passthrough)
	}
	if _, hasMarker := passthrough["dry_run"]; hasMarker {
		t.Errorf("shared-state should NOT have dry_run marker (passthrough), got %v", passthrough)
	}

	// Substituted children: dry_run marker present, original kind preserved.
	for _, name := range []string{"manager", "alice"} {
		out, ok := doneEvents[name]
		if !ok {
			t.Errorf("missing child.done for %q", name)
			continue
		}
		if dr, _ := out["dry_run"].(bool); !dr {
			t.Errorf("%s: dry_run marker missing", name)
		}
		if origKind, _ := out["child_kind"].(string); origKind != "agent" {
			t.Errorf("%s: original kind = %q, want 'agent'", name, origKind)
		}
	}
}

// TestDryRun_NoLLMCallsLeak verifies that dry-run wouldn't accidentally hit
// the agent factory: if an agent kind survived substitution and was looked up,
// LookupFactory("agent") would error in this test binary (agent factory not
// registered). The fact that the workflow completes proves substitution.
func TestDryRun_NoFactoryLookupForLLMKinds(t *testing.T) {
	registerSimulator(t)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "would-be-llm",
				"task": map[string]any{
					"type": "agent", // no agent factory registered in this test
				},
			},
			map[string]any{"kind": "return", "value": "results"},
		},
	}

	_, r := runWorkflowDryRun(t, cfg, 2*time.Second)
	if r.Next != engine.Done {
		t.Fatalf("expected Done (substitution avoided agent lookup), got %v", r.Next)
	}
}
