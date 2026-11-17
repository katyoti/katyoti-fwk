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

func runRaceWorkflow(t *testing.T, branches []any) (any, time.Duration) {
	t.Helper()
	return runRaceWorkflowNode(t, map[string]any{
		"name":     "the-race",
		"kind":     "race",
		"branches": branches,
	})
}

func runRaceWorkflowNode(t *testing.T, raceStep map[string]any) (any, time.Duration) {
	t.Helper()
	registerSimulatorForWkflow(t)

	cfg := map[string]any{
		"steps": []any{
			raceStep,
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

	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, err := exec.Run(runCtx, engine.Event{Type: "start"})
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result.Output, dur
}

// TestWorkflowRace_FailedBranch_SurvivorsKeepRacing — a branch failure must
// NOT settle the race: the fastest branch fails at 20ms while a slower
// branch succeeds at 150ms. The race must wait for the survivor and complete
// successfully with its output (the redundant-sources use case).
//
// Pre-fix behavior (the bug): the failed: event cancelled the survivor,
// popped the frame, and the workflow "succeeded" with empty results.
func TestWorkflowRace_FailedBranch_SurvivorsKeepRacing(t *testing.T) {
	output, _ := runRaceWorkflow(t, []any{
		map[string]any{
			"name": "fast-fail",
			"task": map[string]any{
				"type":             "simulator",
				"duration_ms":      20,
				"fail_probability": 1.0,
				"fail_message":     "fast branch exploded",
			},
		},
		map[string]any{
			"name": "slow-win",
			"task": map[string]any{
				"type":        "simulator",
				"duration_ms": 150,
				"output":      "survivor-wins",
			},
		},
	})

	om, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", output)
	}
	if errStr, _ := om["error"].(string); errStr != "" {
		t.Fatalf("race failed (%q); want the surviving branch to win", errStr)
	}
	if om["slow-win"] != "survivor-wins" {
		t.Errorf("results.slow-win = %#v, want %q (survivor's output)", om["slow-win"], "survivor-wins")
	}
}

// TestWorkflowRace_AllBranchesFail_WorkflowFails — when every branch fails
// there is no winner: the race must fail the workflow with a structured
// error, not fall through as success.
func TestWorkflowRace_AllBranchesFail_WorkflowFails(t *testing.T) {
	output, _ := runRaceWorkflow(t, []any{
		map[string]any{
			"name": "loser-a",
			"task": map[string]any{
				"type":             "simulator",
				"duration_ms":      10,
				"fail_probability": 1.0,
				"fail_message":     "a-failed",
			},
		},
		map[string]any{
			"name": "loser-b",
			"task": map[string]any{
				"type":             "simulator",
				"duration_ms":      30,
				"fail_probability": 1.0,
				"fail_message":     "b-failed",
			},
		},
	})

	om, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output carrying failure indicator, got %#v", output)
	}
	errStr, _ := om["error"].(string)
	if errStr == "" {
		t.Fatalf("output.error empty; race with all branches failed must fail the workflow, got %#v", om)
	}
	if !strings.Contains(errStr, "all 2 branches failed") {
		t.Errorf("output.error = %q, want it to report all branches failed", errStr)
	}
}

// TestWorkflowRace_Timeout_FailsAndCancels — `timeout_ms` bounds the race:
// with only slow branches, the race fails fast (well under the branch
// durations, proving the children were cancelled, not awaited).
func TestWorkflowRace_Timeout_FailsAndCancels(t *testing.T) {
	output, dur := runRaceWorkflowNode(t, map[string]any{
		"name":       "the-race",
		"kind":       "race",
		"timeout_ms": 100,
		"branches": []any{
			map[string]any{
				"name": "slow-a",
				"task": map[string]any{"type": "simulator", "duration_ms": 800, "output": "never"},
			},
			map[string]any{
				"name": "slow-b",
				"task": map[string]any{"type": "simulator", "duration_ms": 800, "output": "never"},
			},
		},
	})

	if dur > 600*time.Millisecond {
		t.Errorf("workflow took %s — the timeout should have settled the race at ~100ms", dur)
	}
	om, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output carrying failure indicator, got %#v", output)
	}
	errStr, _ := om["error"].(string)
	if !strings.Contains(errStr, "timed out after 100ms") {
		t.Errorf("output.error = %q, want the race-timeout failure", errStr)
	}
}

// TestWorkflowRace_Timeout_FastWinnerUnaffected — a branch that succeeds
// inside the window wins normally; the timeout never fires.
func TestWorkflowRace_Timeout_FastWinnerUnaffected(t *testing.T) {
	output, _ := runRaceWorkflowNode(t, map[string]any{
		"name":       "the-race",
		"kind":       "race",
		"timeout_ms": 2000,
		"branches": []any{
			map[string]any{
				"name": "quick",
				"task": map[string]any{"type": "simulator", "duration_ms": 20, "output": "quick-wins"},
			},
		},
	})
	om := output.(map[string]any)
	if om["quick"] != "quick-wins" {
		t.Errorf("results.quick = %#v, want the winner's output", om["quick"])
	}
	if errStr, _ := om["error"].(string); errStr != "" {
		t.Errorf("unexpected error %q — the timeout must not fire when a branch wins in time", errStr)
	}
}

// TestWorkflowRace_Timeout_FallbackDefault — the timeout-default pattern:
// `on_error: fallback` on the race node supplies a default value when the
// race times out.
func TestWorkflowRace_Timeout_FallbackDefault(t *testing.T) {
	output, dur := runRaceWorkflowNode(t, map[string]any{
		"name":       "the-race",
		"kind":       "race",
		"timeout_ms": 100,
		"branches": []any{
			map[string]any{
				"name": "slow",
				"task": map[string]any{"type": "simulator", "duration_ms": 800, "output": "never"},
			},
		},
		"on_error": map[string]any{
			"fallback": map[string]any{
				"name": "default-source",
				"task": map[string]any{"type": "simulator", "duration_ms": 10, "output": "default-value"},
			},
		},
	})

	if dur > 600*time.Millisecond {
		t.Errorf("workflow took %s — timeout + fallback should settle fast", dur)
	}
	om := output.(map[string]any)
	if errStr, _ := om["error"].(string); errStr != "" {
		t.Fatalf("workflow failed (%q) — fallback should have recovered", errStr)
	}
	if om["default-source"] != "default-value" {
		t.Errorf("results.default-source = %#v, want the fallback output", om["default-source"])
	}
}

// TestWorkflowRace_WinnerCancelsLosers — the happy path, pinned: first
// successful branch wins, slow losers are cancelled (the workflow terminates
// well under the loser's duration), winner output lands in results.
func TestWorkflowRace_WinnerCancelsLosers(t *testing.T) {
	output, dur := runRaceWorkflow(t, []any{
		map[string]any{
			"name": "fast-win",
			"task": map[string]any{
				"type":        "simulator",
				"duration_ms": 20,
				"output":      "fast-wins",
			},
		},
		map[string]any{
			"name": "slow-lose",
			"task": map[string]any{
				"type":        "simulator",
				"duration_ms": 500,
				"output":      "should-be-cancelled",
			},
		},
	})

	if dur > 400*time.Millisecond {
		t.Errorf("workflow took %s, want <400ms (loser should have been cancelled)", dur)
	}
	om, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", output)
	}
	if om["fast-win"] != "fast-wins" {
		t.Errorf("results.fast-win = %#v, want %q", om["fast-win"], "fast-wins")
	}
	if errStr, _ := om["error"].(string); errStr != "" {
		t.Errorf("unexpected error %q on the happy path", errStr)
	}
}
