// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"fmt"
	"strings"
	"testing"
)

// TestLoaderCaps_Loop verifies the upper bound on `loop.times` rejects
// unbounded counts (R6 — security.md high-finding-2). The existing positive
// path is covered by TestLoadNodes_Loop in loader_test.go.
func TestLoaderCaps_Loop(t *testing.T) {
	// Just over the cap is rejected.
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":  "loop",
				"times": MaxLoopTimes + 1,
				"body":  map[string]any{"kind": "return", "output": map[string]any{}},
			},
		},
	}
	_, err := LoadNodes(cfg)
	if err == nil {
		t.Fatal("loop.times above MaxLoopTimes should be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds cap") {
		t.Errorf("error %q should mention the cap", err.Error())
	}

	// At the cap is accepted.
	cfg["steps"].([]any)[0].(map[string]any)["times"] = MaxLoopTimes
	if _, err := LoadNodes(cfg); err != nil {
		t.Errorf("loop.times = MaxLoopTimes (%d) should be accepted, got %v", MaxLoopTimes, err)
	}
}

// TestLoaderCaps_While verifies the cap on `while.max_iterations` (R6).
func TestLoaderCaps_While(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":           "while",
				"condition":      "true",
				"max_iterations": MaxWhileIterations + 1,
				"body":           map[string]any{"kind": "return", "output": map[string]any{}},
			},
		},
	}
	_, err := LoadNodes(cfg)
	if err == nil {
		t.Fatal("while.max_iterations above cap should be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds cap") {
		t.Errorf("error %q should mention the cap", err.Error())
	}

	// At the cap is accepted.
	cfg["steps"].([]any)[0].(map[string]any)["max_iterations"] = MaxWhileIterations
	if _, err := LoadNodes(cfg); err != nil {
		t.Errorf("while.max_iterations = MaxWhileIterations (%d) should be accepted, got %v", MaxWhileIterations, err)
	}
}

// TestLoaderCaps_ParallelBranches verifies the cap on parallel branch count
// (R6 — prevents 100k concurrent agent loops).
func TestLoaderCaps_ParallelBranches(t *testing.T) {
	branches := make([]any, MaxParallelBranches+1)
	for i := range branches {
		// Branches must be delegates (non-delegate kinds are rejected at
		// parse) — use minimal inline simulator tasks.
		branches[i] = map[string]any{
			"name": fmt.Sprintf("b%d", i),
			"task": map[string]any{"type": "simulator"},
		}
	}
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "parallel", "branches": branches},
		},
	}
	_, err := LoadNodes(cfg)
	if err == nil {
		t.Fatalf("parallel with %d branches should be rejected", MaxParallelBranches+1)
	}
	if !strings.Contains(err.Error(), "cap") {
		t.Errorf("error %q should mention the cap", err.Error())
	}

	// At the cap is accepted.
	cfg["steps"].([]any)[0].(map[string]any)["branches"] = branches[:MaxParallelBranches]
	if _, err := LoadNodes(cfg); err != nil {
		t.Errorf("parallel at MaxParallelBranches (%d) should be accepted, got %v", MaxParallelBranches, err)
	}
}

// TestLoaderCaps_ForeachParallelRequiresMaxConcurrency verifies that
// foreach with mode=parallel rejects max_concurrency: 0 (was historically
// "unlimited"; now an explicit error per R6 / threat-T4).
func TestLoaderCaps_ForeachParallelRequiresMaxConcurrency(t *testing.T) {
	mkCfg := func(mode string, mc any) map[string]any {
		step := map[string]any{
			"kind":  "foreach",
			"items": "[1,2,3]",
			"mode":  mode,
			"body":  map[string]any{"kind": "return", "output": map[string]any{}},
		}
		if mc != nil {
			step["max_concurrency"] = mc
		}
		return map[string]any{"steps": []any{step}}
	}

	// parallel mode + missing max_concurrency = error.
	if _, err := LoadNodes(mkCfg("parallel", nil)); err == nil {
		t.Error("foreach mode=parallel without max_concurrency should be rejected")
	}

	// parallel mode + max_concurrency: 0 = error.
	if _, err := LoadNodes(mkCfg("parallel", 0)); err == nil {
		t.Error("foreach mode=parallel with max_concurrency=0 should be rejected (no longer 'unlimited')")
	}

	// parallel mode + above-cap = error.
	if _, err := LoadNodes(mkCfg("parallel", MaxForeachConcurrency+1)); err == nil {
		t.Errorf("foreach mode=parallel with max_concurrency=%d should be rejected", MaxForeachConcurrency+1)
	}

	// parallel mode + within range = ok.
	if _, err := LoadNodes(mkCfg("parallel", 4)); err != nil {
		t.Errorf("foreach mode=parallel max_concurrency=4 should be accepted, got %v", err)
	}

	// sequential mode ignores max_concurrency entirely — 0 is fine.
	if _, err := LoadNodes(mkCfg("sequential", 0)); err != nil {
		t.Errorf("foreach mode=sequential max_concurrency=0 should be accepted (irrelevant), got %v", err)
	}
	if _, err := LoadNodes(mkCfg("sequential", nil)); err != nil {
		t.Errorf("foreach mode=sequential without max_concurrency should be accepted, got %v", err)
	}
}
