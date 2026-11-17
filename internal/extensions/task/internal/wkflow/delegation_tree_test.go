// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// registerWorkflowFactoryForTest contributes the "workflow" machine kind so
// NESTED workflow children resolve through engine.LookupFactory in-package
// (production wiring is tasklocal.WorkflowExtension; tests mirror it the
// same way registerSimulatorForWkflow mirrors the simulator extension).
func registerWorkflowFactoryForTest(t *testing.T) {
	t.Helper()
	_, createFn := MachineStrategyDef.Strategy.NewTypeConfig()
	raw, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("workflow factory: %v", err)
	}
	td := MachineStrategyDef.Strategy.CreateTypeDef("workflow", raw)
	_ = engine.MachineDomain.Types.Contribute(td)
}

// TestDelegationTree_ModesAndPoliciesEndToEnd is the engine-level pin of the
// demo-delegation-tree shape: a
// coordinator-mode workflow fans out to dimension-flavored branches and a
// NESTED workflow level, runs a background delegate, and joins it. Asserted
// from the OUTPUTS (simulator record_context → __spawn_dimensions), which
// pins the full chain: branch mode → ChildSpec → BuildChildOverride →
// merged child config → runtime view. In particular:
//
//   - a `mode: worker` branch under a coordinator root carries the sticky
//     coordinator-restricts-children policy (restricted permissions) —
//     proof the branch-level `mode:` reaches the spawner (it used to be
//     silently dropped);
//   - a GRANDCHILD (leaf of the nested workflow) carries it too — the
//     transitive case through `spawnPolicies` serialization;
//   - the background delegate returns an immediate handle and a later
//     `wait for_event` joins it;
//   - the flaky branch recovers via on_error fallback.
func TestDelegationTree_ModesAndPoliciesEndToEnd(t *testing.T) {
	registerSimulatorForWkflow(t)
	registerWorkflowFactoryForTest(t)

	leaf := func(role string, durMs int) map[string]any {
		return map[string]any{
			"type":           "simulator",
			"duration_ms":    durMs,
			"record_context": true,
			"output":         map[string]any{"role": role},
		}
	}

	cfg := map[string]any{
		"_mode": "coordinator",
		"steps": []any{
			map[string]any{
				"name":            "fan-out",
				"kind":            "parallel",
				"on_branch_error": "collect",
				"branches": []any{
					map[string]any{"name": "worker-a", "mode": "worker", "task": leaf("worker-a", 50)},
					map[string]any{
						"name": "flaky-b", "mode": "worker",
						"task": map[string]any{
							"type": "simulator", "duration_ms": 20,
							"fail_probability": 1.0, "fail_message": "injected",
						},
						"on_error": map[string]any{
							"fallback": map[string]any{
								"name": "flaky-b-recovery",
								"task": leaf("flaky-b-recovery", 10),
							},
						},
					},
					map[string]any{
						"name": "sub-team", "mode": "worker",
						"task": map[string]any{
							"type":  "workflow",
							"_mode": "worker",
							"steps": []any{
								map[string]any{"name": "grandchild-a", "task": leaf("grandchild-a", 30)},
								map[string]any{"kind": "return", "value": "${results}"},
							},
						},
					},
				},
			},
			map[string]any{"name": "bg-report", "mode": "background", "task": leaf("bg-report", 150)},
			map[string]any{"kind": "wait", "for_event": "child.bg-report.done", "timeout_ms": 5000},
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
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := exec.Run(runCtx, engine.Event{Type: "start"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	out, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("output = %#v (next=%v, err=%v), want results map", result.Output, result.Next, result.Error)
	}

	// (a) Branch-mode pass-through: worker-a under coordinator → restricted.
	assertRestrictedDimensions(t, out, "worker-a")

	// (b) Grandchild (nested level) carries the sticky policy too (UC-054).
	subTeam, ok := out["sub-team"].(map[string]any)
	if !ok {
		t.Fatalf("results.sub-team = %#v, want the nested workflow's results map", out["sub-team"])
	}
	assertRestrictedDimensions(t, subTeam, "grandchild-a")

	// (c) Background delegate: immediate handle + joined by the wait.
	bg, ok := out["bg-report"].(map[string]any)
	if !ok || bg["background"] != true {
		t.Errorf("results.bg-report = %#v, want the background handle", out["bg-report"])
	}

	// (d) Flaky branch recovered via fallback (result recorded under the
	// fallback child's name; the original branch name holds no success output).
	if _, ok := out["flaky-b-recovery"].(map[string]any); !ok {
		t.Errorf("results.flaky-b-recovery missing — fallback did not record: %#v", out)
	}
}

// assertRestrictedDimensions asserts the named child's record_context output
// shows the coordinator-restricts-children policy in __spawn_dimensions
// (non-empty allowed_tools / restricted mode under the permissions view).
func assertRestrictedDimensions(t *testing.T, results map[string]any, name string) {
	t.Helper()
	child, ok := results[name].(map[string]any)
	if !ok {
		t.Fatalf("results.%s = %#v, want the simulator output map", name, results[name])
	}
	dims, ok := child["__spawn_dimensions"].(map[string]any)
	if !ok {
		t.Fatalf("%s: __spawn_dimensions missing (record_context not honored): %#v", name, child)
	}
	perms, ok := dims["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("%s: __spawn_dimensions.permissions missing: %#v", name, dims)
	}
	mode, _ := perms["permission_mode"].(string)
	allowed, _ := perms["allowed_tools"].([]any)
	if mode != "restricted" && len(allowed) == 0 {
		t.Errorf("%s: expected coordinator restriction (restricted mode or allowed_tools subset), got permissions=%#v", name, perms)
	}
}
