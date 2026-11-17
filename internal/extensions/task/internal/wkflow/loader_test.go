// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"testing"
)

func TestLoadNodes_BasicSequence(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "step1",
				"task": map[string]any{
					"type":        "simulator",
					"duration_ms": 100,
					"output":      "hello",
				},
			},
			map[string]any{
				"name": "step2",
				"task": map[string]any{
					"type":   "simulator",
					"output": "world",
				},
				"input": map[string]any{
					"prev": "results.step1",
				},
			},
		},
	}

	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}

	if root.Kind != NodeSequence {
		t.Fatalf("root.Kind = %q, want sequence", root.Kind)
	}
	if len(root.Sequence.Steps) != 2 {
		t.Fatalf("steps count = %d, want 2", len(root.Sequence.Steps))
	}

	// First step
	s1 := root.Sequence.Steps[0]
	if s1.Kind != NodeDelegate {
		t.Errorf("step1.Kind = %q, want delegate", s1.Kind)
	}
	if s1.Delegate.ChildName != "step1" {
		t.Errorf("step1.ChildName = %q, want step1", s1.Delegate.ChildName)
	}
	if s1.Delegate.ChildKind != "simulator" {
		t.Errorf("step1.ChildKind = %q, want simulator", s1.Delegate.ChildKind)
	}

	// Second step with input bindings
	s2 := root.Sequence.Steps[1]
	if s2.Delegate.InputBinds["prev"] != "results.step1" {
		t.Errorf("step2.InputBinds[prev] = %v, want 'results.step1'", s2.Delegate.InputBinds["prev"])
	}
}

func TestLoadNodes_ReturnNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "work",
				"task": map[string]any{
					"type":   "simulator",
					"output": "done",
				},
			},
			map[string]any{
				"kind":  "return",
				"value": "results.work",
			},
		},
	}

	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}

	if len(root.Sequence.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(root.Sequence.Steps))
	}

	ret := root.Sequence.Steps[1]
	if ret.Kind != NodeReturn {
		t.Errorf("ret.Kind = %q, want return", ret.Kind)
	}
	if ret.Return.Value != "results.work" {
		t.Errorf("ret.Value = %v, want 'results.work'", ret.Return.Value)
	}
}

func TestLoadNodes_OnError(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "risky",
				"task": map[string]any{
					"type":             "simulator",
					"fail_probability": 1.0,
				},
				"on_error": "continue",
			},
		},
	}

	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}

	step := root.Sequence.Steps[0]
	if step.OnError == nil {
		t.Fatal("OnError is nil, want continue")
	}
	if step.OnError.Strategy != OnErrorContinue {
		t.Errorf("OnError.Strategy = %q, want continue", step.OnError.Strategy)
	}
}

func TestLoadNodes_OnErrorFallback(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "primary",
				"task": map[string]any{
					"type":   "simulator",
					"output": "primary result",
				},
				"on_error": map[string]any{
					"fallback": map[string]any{
						"name": "backup",
						"task": map[string]any{
							"type":   "simulator",
							"output": "fallback result",
						},
					},
				},
			},
		},
	}

	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}

	step := root.Sequence.Steps[0]
	if step.OnError == nil {
		t.Fatal("OnError is nil")
	}
	if step.OnError.Strategy != OnErrorFallback {
		t.Errorf("Strategy = %q, want fallback", step.OnError.Strategy)
	}
	if step.OnError.Fallback == nil {
		t.Fatal("Fallback node is nil")
	}
	if step.OnError.Fallback.Delegate.ChildName != "backup" {
		t.Errorf("Fallback.ChildName = %q, want backup", step.OnError.Fallback.Delegate.ChildName)
	}
}

func TestLoadNodes_MissingSteps(t *testing.T) {
	_, err := LoadNodes(map[string]any{})
	if err == nil {
		t.Error("expected error for missing steps")
	}
}

func TestLoadNodes_IfNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":      "if",
				"condition": "results.check.passed == true",
				"then": map[string]any{
					"name": "proceed",
					"task": map[string]any{"type": "simulator", "output": "ok"},
				},
				"else": map[string]any{
					"name": "fallback",
					"task": map[string]any{"type": "simulator", "output": "fallback"},
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeIf {
		t.Fatalf("Kind = %q, want if", step.Kind)
	}
	if step.If.Condition != "results.check.passed == true" {
		t.Errorf("Condition = %q", step.If.Condition)
	}
	if step.If.Then.Kind != NodeDelegate {
		t.Errorf("Then.Kind = %q, want delegate", step.If.Then.Kind)
	}
	if step.If.Else == nil {
		t.Fatal("Else is nil")
	}
}

func TestLoadNodes_ParallelNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind": "parallel",
				"branches": []any{
					map[string]any{"name": "a", "task": map[string]any{"type": "simulator"}},
					map[string]any{"name": "b", "task": map[string]any{"type": "simulator"}},
				},
				"on_branch_error": "collect",
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeParallel {
		t.Fatalf("Kind = %q, want parallel", step.Kind)
	}
	if len(step.Parallel.Branches) != 2 {
		t.Errorf("branches = %d, want 2", len(step.Parallel.Branches))
	}
	if step.Parallel.OnBranchError != "collect" {
		t.Errorf("OnBranchError = %q, want collect", step.Parallel.OnBranchError)
	}
}

func TestLoadNodes_ForeachNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":  "foreach",
				"items": "inputs.topics",
				"mode":  "parallel",
				"body": map[string]any{
					"name": "worker",
					"task": map[string]any{"type": "simulator"},
				},
				"max_concurrency": 5,
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeForeach {
		t.Fatalf("Kind = %q, want foreach", step.Kind)
	}
	if step.Foreach.Items != "inputs.topics" {
		t.Errorf("Items = %q", step.Foreach.Items)
	}
	if step.Foreach.Mode != "parallel" {
		t.Errorf("Mode = %q, want parallel", step.Foreach.Mode)
	}
	if step.Foreach.MaxConcurrency != 5 {
		t.Errorf("MaxConcurrency = %d, want 5", step.Foreach.MaxConcurrency)
	}
}

func TestLoadNodes_LoopNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":  "loop",
				"times": 3,
				"body": map[string]any{
					"name": "attempt",
					"task": map[string]any{"type": "simulator"},
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeLoop {
		t.Fatalf("Kind = %q, want loop", step.Kind)
	}
	if step.Loop.Times != 3 {
		t.Errorf("Times = %d, want 3", step.Loop.Times)
	}
}

func TestLoadNodes_WhileNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":           "while",
				"condition":      "state.quality < 0.9",
				"max_iterations": 10,
				"body": map[string]any{
					"name": "refine",
					"task": map[string]any{"type": "simulator"},
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeWhile {
		t.Fatalf("Kind = %q, want while", step.Kind)
	}
	if step.While.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", step.While.MaxIterations)
	}
}

func TestLoadNodes_AssignNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":   "assign",
				"target": "ready",
				"value":  true,
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeAssign {
		t.Fatalf("Kind = %q, want assign", step.Kind)
	}
	if step.Assign.Target != "ready" {
		t.Errorf("Target = %q, want ready", step.Assign.Target)
	}
	if step.Assign.Value != true {
		t.Errorf("Value = %v, want true", step.Assign.Value)
	}
}

func TestLoadNodes_WaitNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":        "wait",
				"duration_ms": 1000,
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeWait {
		t.Fatalf("Kind = %q, want wait", step.Kind)
	}
	if step.Wait.DurationMs != 1000 {
		t.Errorf("DurationMs = %d, want 1000", step.Wait.DurationMs)
	}
}

func TestLoadNodes_ParallelOnBranchErrorValidation(t *testing.T) {
	// Valid: fail_fast
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":            "parallel",
				"on_branch_error": "fail_fast",
				"branches": []any{
					map[string]any{"name": "a", "task": map[string]any{"type": "simulator"}},
				},
			},
		},
	}
	if _, err := LoadNodes(cfg); err != nil {
		t.Errorf("fail_fast should be valid: %v", err)
	}

	// Valid: collect
	cfg["steps"].([]any)[0].(map[string]any)["on_branch_error"] = "collect"
	if _, err := LoadNodes(cfg); err != nil {
		t.Errorf("collect should be valid: %v", err)
	}

	// Invalid: arbitrary value
	cfg["steps"].([]any)[0].(map[string]any)["on_branch_error"] = "retry"
	if _, err := LoadNodes(cfg); err == nil {
		t.Error("expected error for invalid on_branch_error 'retry'")
	}
}

func TestLoadNodes_OnErrorRetry(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "flaky",
				"task": map[string]any{"type": "simulator"},
				"on_error": map[string]any{
					"strategy":    "retry",
					"max_retries": 3,
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.OnError == nil {
		t.Fatal("OnError is nil")
	}
	if step.OnError.Strategy != OnErrorRetry {
		t.Errorf("Strategy = %q, want retry", step.OnError.Strategy)
	}
	if step.OnError.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", step.OnError.MaxRetries)
	}
}

func TestLoadNodes_NamedTask(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"name":     "delegate",
				"taskName": "hello-world",
			},
		},
	}

	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}

	step := root.Sequence.Steps[0]
	if step.Delegate.ChildKind != "task" {
		t.Errorf("ChildKind = %q, want task", step.Delegate.ChildKind)
	}
	if step.Delegate.Config["_taskName"] != "hello-world" {
		t.Errorf("Config._taskName = %v, want hello-world", step.Delegate.Config["_taskName"])
	}
}

// TestLoadNodes_SwitchNode (UC-024) — switch loader parses `on` + `cases` map
// + optional `default`, producing a SwitchNode AST. Missing `on` and missing
// `cases` are rejected.
func TestLoadNodes_SwitchNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind": "switch",
				"on":   "results.classify.bucket",
				"cases": map[string]any{
					"warm": map[string]any{
						"name": "warm-branch",
						"task": map[string]any{"type": "simulator", "output": "warm"},
					},
					"cool": map[string]any{
						"name": "cool-branch",
						"task": map[string]any{"type": "simulator", "output": "cool"},
					},
				},
				"default": map[string]any{
					"name": "default-branch",
					"task": map[string]any{"type": "simulator", "output": "default"},
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeSwitch {
		t.Fatalf("Kind = %q, want switch", step.Kind)
	}
	if step.Switch == nil {
		t.Fatal("Switch is nil")
	}
	if step.Switch.On != "results.classify.bucket" {
		t.Errorf("Switch.On = %q", step.Switch.On)
	}
	if len(step.Switch.Cases) != 2 {
		t.Errorf("Switch.Cases len = %d, want 2", len(step.Switch.Cases))
	}
	if _, ok := step.Switch.Cases["warm"]; !ok {
		t.Error("Switch.Cases missing 'warm'")
	}
	if step.Switch.Default == nil {
		t.Error("Switch.Default is nil")
	}

	// Negative: missing `on` is a parse error.
	if _, err := LoadNodes(map[string]any{
		"steps": []any{map[string]any{"kind": "switch", "cases": map[string]any{}}},
	}); err == nil {
		t.Error("expected error when `on` is missing")
	}
}

// TestLoadNodes_RaceNode (UC-031) — race loader parses `branches` as a list
// of step nodes, producing a RaceNode AST.
func TestLoadNodes_RaceNode(t *testing.T) {
	config := map[string]any{
		"steps": []any{
			map[string]any{
				"kind": "race",
				"branches": []any{
					map[string]any{
						"name": "fast",
						"task": map[string]any{"type": "simulator", "duration_ms": 10},
					},
					map[string]any{
						"name": "slow",
						"task": map[string]any{"type": "simulator", "duration_ms": 100},
					},
				},
			},
		},
	}
	root, err := LoadNodes(config)
	if err != nil {
		t.Fatalf("LoadNodes: %v", err)
	}
	step := root.Sequence.Steps[0]
	if step.Kind != NodeRace {
		t.Fatalf("Kind = %q, want race", step.Kind)
	}
	if step.Race == nil {
		t.Fatal("Race is nil")
	}
	if len(step.Race.Branches) != 2 {
		t.Errorf("Race.Branches len = %d, want 2", len(step.Race.Branches))
	}
	if step.Race.Branches[0].Kind != NodeDelegate {
		t.Errorf("Branches[0].Kind = %q, want delegate", step.Race.Branches[0].Kind)
	}

	// Negative: missing `branches` is a parse error.
	if _, err := LoadNodes(map[string]any{
		"steps": []any{map[string]any{"kind": "race"}},
	}); err == nil {
		t.Error("expected error when `branches` is missing")
	}
}
