// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestWorkflowParallel_FailFast_CancelsSiblings (UC-155) — a workflow of N
// parallel simulators with `on_branch_error: fail_fast` (the default) — when
// one branch fails fast, the siblings are cancelled. The workflow's terminal
// state is failed, not done.
//
// Fixture: branch-c fails at 20ms; branches-a/b are 500ms simulators.
// Expected: workflow terminates well under 500ms (proves siblings were
// cancelled, not awaited).
func TestWorkflowParallel_FailFast_CancelsSiblings(t *testing.T) {
	registerSimulatorForWkflow(t)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "fanout",
				"kind": "parallel",
				// fail_fast is the default; declare explicitly for clarity.
				"on_branch_error": "fail_fast",
				"branches": []any{
					map[string]any{
						"name": "slow-a",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 500,
							"output":      "should-be-cancelled-a",
						},
					},
					map[string]any{
						"name": "slow-b",
						"task": map[string]any{
							"type":        "simulator",
							"duration_ms": 500,
							"output":      "should-be-cancelled-b",
						},
					},
					map[string]any{
						"name": "fast-fail-c",
						"task": map[string]any{
							"type":             "simulator",
							"duration_ms":      20,
							"fail_probability": 1.0,
							"fail_message":     "uc-155-fail",
						},
					},
				},
			},
			map[string]any{"kind": "return", "value": "results"},
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

	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := exec.Run(runCtx, engine.Event{Type: "start"})
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Workflow must terminate fast (well under the 500ms sibling duration) —
	// proves the siblings were cancelled, not awaited to completion.
	if dur > 400*time.Millisecond {
		t.Errorf("UC-155: workflow took %s, want <400ms (siblings should have been cancelled)", dur)
	}

	// The workflow's Output map carries a failure indicator on fail_fast.
	// (Executor maps any non-nil Output to Next=Done; the workflow signals
	// failure via output.error, not via the executor's Next field.)
	om, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("UC-155: expected map output carrying failure indicator, got %#v", result.Output)
	}
	errStr, _ := om["error"].(string)
	if errStr == "" {
		t.Errorf("UC-155: output.error empty; expected failure-branch indicator, got %#v", om)
	} else if !strings.Contains(errStr, "branch failed") && !strings.Contains(errStr, "fast-fail-c") && !strings.Contains(errStr, "uc-155-fail") {
		t.Errorf("UC-155: output.error = %q, want failure indicator for fast-fail-c", errStr)
	}
}
