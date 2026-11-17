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

// T2 — extended step-handler coverage. Tier-1.3 shipped tests for
// stepSpawn, stepRace, stepParallel.fail_fast, lifecycle emissions. This
// file fills the gaps the audit's test-coverage report flagged (priority-2):
// stepIf, stepSwitch, stepLoop, stepWhile.MaxIterations, stepForeach
// sequential+parallel-pin, stepAssign literal+${expr}, stepWait
// event/duration/timeout, handleNodeError OnErrorRetry-exhaustion +
// OnErrorContinue. Lands as the safety net before D1 god-method refactor.

// runWorkflow runs a workflow to completion and returns the executor's
// RunResult. The helper is intentionally low-level — tests inspect r.Output
// directly so they don't have to wrap every return value in a map.
func runWorkflow(t *testing.T, cfg map[string]any, timeout time.Duration) *engine.RunResult {
	t.Helper()
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: cfg})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	exec := engine.NewExecutor(m, channel, nil)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r, err := exec.Run(runCtx, engine.Event{Type: "start"})
	if err != nil {
		t.Fatalf("workflow run error: %v", err)
	}
	if r == nil {
		t.Fatal("workflow returned nil RunResult")
	}
	return r
}

// outputMap extracts r.Output as a map, failing the test on type mismatch.
func outputMap(t *testing.T, r *engine.RunResult) map[string]any {
	t.Helper()
	m, ok := r.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T = %v", r.Output, r.Output)
	}
	return m
}

// ── stepIf ────────────────────────────────────────────────────────────

func TestStepIf_TrueBranch(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":      "if",
				"condition": "true",
				"then":      map[string]any{"kind": "return", "value": map[string]any{"branch": "then"}},
				"else":      map[string]any{"kind": "return", "value": map[string]any{"branch": "else"}},
			},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["branch"] != "then" {
		t.Errorf("if true: branch = %v, want 'then'", out["branch"])
	}
}

func TestStepIf_FalseBranch(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":      "if",
				"condition": "false",
				"then":      map[string]any{"kind": "return", "value": map[string]any{"branch": "then"}},
				"else":      map[string]any{"kind": "return", "value": map[string]any{"branch": "else"}},
			},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["branch"] != "else" {
		t.Errorf("if false: branch = %v, want 'else'", out["branch"])
	}
}

// ── stepSwitch ────────────────────────────────────────────────────────

// String literals via R11 explicit form: ${"red"} ensures the value is the
// string "red", not the expr-lang identifier `red` (undefined → nil under
// AllowUndefinedVariables).
func TestStepSwitch_CaseMatch(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "color", "value": `${"red"}`},
			map[string]any{
				"kind": "switch",
				"on":   "state.color",
				"cases": map[string]any{
					"red":  map[string]any{"kind": "return", "value": map[string]any{"taken": "red"}},
					"blue": map[string]any{"kind": "return", "value": map[string]any{"taken": "blue"}},
				},
				"default": map[string]any{"kind": "return", "value": map[string]any{"taken": "default"}},
			},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["taken"] != "red" {
		t.Errorf("switch case match: taken = %v, want 'red'", out["taken"])
	}
}

func TestStepSwitch_DefaultBranch(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "color", "value": `${"green"}`},
			map[string]any{
				"kind": "switch",
				"on":   "state.color",
				"cases": map[string]any{
					"red":  map[string]any{"kind": "return", "value": map[string]any{"taken": "red"}},
					"blue": map[string]any{"kind": "return", "value": map[string]any{"taken": "blue"}},
				},
				"default": map[string]any{"kind": "return", "value": map[string]any{"taken": "default"}},
			},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["taken"] != "default" {
		t.Errorf("switch default: taken = %v, want 'default'", out["taken"])
	}
}

func TestStepSwitch_NoMatchNoDefault(t *testing.T) {
	// No matching case and no default — the switch step pops the frame; the
	// workflow continues to the next step (return).
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "color", "value": `${"purple"}`},
			map[string]any{
				"kind": "switch",
				"on":   "state.color",
				"cases": map[string]any{
					"red": map[string]any{"kind": "return", "value": map[string]any{"taken": "red"}},
				},
			},
			map[string]any{"kind": "return", "value": map[string]any{"taken": "after_switch"}},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["taken"] != "after_switch" {
		t.Errorf("switch no-match: taken = %v, want 'after_switch'", out["taken"])
	}
}

// ── stepLoop ──────────────────────────────────────────────────────────

func TestStepLoop_NTimes(t *testing.T) {
	// stepReturn evaluates a top-level string value as an expression but
	// doesn't recurse into map-literal values. Tests that need state read
	// back into output use the string-return form and r.Output is then
	// the evaluated primitive.
	cases := []struct {
		name  string
		times int
		want  int64
	}{
		{"N=1", 1, 1},
		{"N=10", 10, 10},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := map[string]any{
				"steps": []any{
					map[string]any{"kind": "assign", "target": "counter", "value": 0},
					map[string]any{
						"kind":  "loop",
						"times": tc.times,
						"body":  map[string]any{"kind": "assign", "target": "counter", "value": "${state.counter + 1}"},
					},
					map[string]any{"kind": "return", "value": "${state.counter}"},
				},
			}
			r := runWorkflow(t, cfg, 2*time.Second)
			got, ok := toInt64(r.Output)
			if !ok {
				t.Fatalf("loop %s: output wrong type: %T = %v", tc.name, r.Output, r.Output)
			}
			if got != tc.want {
				t.Errorf("loop %s: counter = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

// ── stepWhile.MaxIterations ───────────────────────────────────────────

func TestStepWhile_MaxIterationsCap(t *testing.T) {
	// while(true) with max_iterations=3 must terminate at exactly 3 iterations.
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "counter", "value": 0},
			map[string]any{
				"kind":           "while",
				"condition":      "true",
				"max_iterations": 3,
				"body":           map[string]any{"kind": "assign", "target": "counter", "value": "${state.counter + 1}"},
			},
			map[string]any{"kind": "return", "value": "${state.counter}"},
		},
	}
	r := runWorkflow(t, cfg, 2*time.Second)
	got, _ := toInt64(r.Output)
	if got != 3 {
		t.Errorf("while max_iterations: counter = %d, want 3 (cap holds)", got)
	}
}

// ── stepForeach ───────────────────────────────────────────────────────

func TestStepForeach_Sequential(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "sum", "value": 0},
			map[string]any{
				"kind":  "foreach",
				"items": "[10, 20, 30]",
				"mode":  "sequential",
				"body":  map[string]any{"kind": "assign", "target": "sum", "value": "${state.sum + loop.item}"},
			},
			map[string]any{"kind": "return", "value": "${state.sum}"},
		},
	}
	r := runWorkflow(t, cfg, 2*time.Second)
	got, _ := toInt64(r.Output)
	if got != 60 {
		t.Errorf("foreach sequential: sum = %d, want 60", got)
	}
}

// TestStepForeach_NoLoopMagicKeyLeak — D1 quirk #3 (audit F-WF-4) closure.
// Pre-fix, foreach wrote `_loop_index` / `_loop_item` keys into ws.Data,
// where they leaked into the user expression scope as `state._loop_*`.
// Post-fix, foreach stashes the iteration state in frame.Local; ws.Data
// stays free of the magic keys. The body sees the loop via `loop.item` /
// `loop.index` (the typed Scope.Loop path), and `state._loop_item` is
// undefined.
func TestStepForeach_NoLoopMagicKeyLeak(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "leaked", "value": "${state._loop_item}"},
			map[string]any{
				"kind":  "foreach",
				"items": "[1, 2, 3]",
				"body":  map[string]any{"kind": "assign", "target": "last_loop_item", "value": "${loop.item}"},
			},
			// After the foreach completes, the loop magic keys should NOT have
			// leaked into state. Read them — they should evaluate to nil. The
			// pre-fix code would have surfaced state._loop_item == 3 (the last
			// iteration's item) which is exactly the leak we're closing.
			map[string]any{"kind": "assign", "target": "post_loop_leak", "value": "${state._loop_item}"},
			map[string]any{"kind": "return", "value": map[string]any{
				"leaked":         "${state.leaked}",         // before the foreach — must be nil
				"last_loop_item": "${state.last_loop_item}", // inside foreach via loop.item
				"post_loop_leak": "${state.post_loop_leak}", // after foreach — must be nil
			}},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["leaked"] != nil {
		t.Errorf("pre-foreach state._loop_item leaked: got %v, want nil", out["leaked"])
	}
	if n, ok := toInt64(out["last_loop_item"]); !ok || n != 3 {
		t.Errorf("loop.item should reach %v during foreach body; got %v (%T)", 3, out["last_loop_item"], out["last_loop_item"])
	}
	if out["post_loop_leak"] != nil {
		t.Errorf("post-foreach state._loop_item leaked: got %v, want nil", out["post_loop_leak"])
	}
}

func TestStepForeach_ParallelModePinnedAsSequential(t *testing.T) {
	// Pin current behavior: mode=parallel + max_concurrency executes
	// sequentially today (TODO in stepForeach line 600). When true parallel
	// lands, the charter Test plan cell must be updated alongside this test.
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "sum", "value": 0},
			map[string]any{
				"kind":            "foreach",
				"items":           "[1, 2, 3, 4]",
				"mode":            "parallel",
				"max_concurrency": 2,
				"body":            map[string]any{"kind": "assign", "target": "sum", "value": "${state.sum + loop.item}"},
			},
			map[string]any{"kind": "return", "value": "${state.sum}"},
		},
	}
	r := runWorkflow(t, cfg, 2*time.Second)
	got, _ := toInt64(r.Output)
	if got != 10 {
		t.Errorf("foreach parallel (pinned-sequential): sum = %d, want 10", got)
	}
}

// ── stepAssign ────────────────────────────────────────────────────────

func TestStepAssign_LiteralValue(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "x", "value": 42},
			map[string]any{"kind": "return", "value": "${state.x}"},
		},
	}
	r := runWorkflow(t, cfg, 2*time.Second)
	got, ok := toInt64(r.Output)
	if !ok || got != 42 {
		t.Errorf("assign literal: x = %v, want 42", r.Output)
	}
}

func TestStepAssign_WrappedExpression(t *testing.T) {
	// stepAssign honors ${...} explicit syntax. Bare strings stay literal.
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "x", "value": 10},
			map[string]any{"kind": "assign", "target": "y", "value": "${state.x * 2}"},
			map[string]any{"kind": "return", "value": "${state.y}"},
		},
	}
	r := runWorkflow(t, cfg, 2*time.Second)
	got, ok := toInt64(r.Output)
	if !ok || got != 20 {
		t.Errorf("assign ${expr}: y = %v, want 20", r.Output)
	}
}

// ── stepWait ──────────────────────────────────────────────────────────

func TestStepWait_DurationOnly(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "wait", "duration_ms": 50},
			map[string]any{"kind": "return", "value": map[string]any{"done": true}},
		},
	}
	start := time.Now()
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("wait duration_ms=50 elapsed %v, want >= 40ms", elapsed)
	}
	if out["done"] != true {
		t.Errorf("after wait: done = %v, want true", out["done"])
	}
}

func TestStepWait_EventResume(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "wait", "for_event": "external.ready"},
			map[string]any{"kind": "return", "value": map[string]any{"resumed": true}},
		},
	}

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: cfg})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	exec := engine.NewExecutor(m, channel, nil)

	// Publish the event after a brief delay so the workflow suspends first.
	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish("external.ready", map[string]any{"signal": "go"})
	}()

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := exec.Run(runCtx, engine.Event{Type: "start"})
	if err != nil {
		t.Fatalf("workflow run error: %v", err)
	}
	out, _ := r.Output.(map[string]any)
	if out == nil || out["resumed"] != true {
		t.Errorf("wait for_event resume: resumed = %v, want true (out=%+v)", out["resumed"], out)
	}
}

func TestStepWait_TimeoutResume(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"kind":       "wait",
				"for_event":  "never.published",
				"timeout_ms": 50,
			},
			map[string]any{"kind": "return", "value": map[string]any{"timed_out": true}},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["timed_out"] != true {
		t.Errorf("wait timeout: timed_out = %v, want true", out["timed_out"])
	}
}

// ── handleNodeError ───────────────────────────────────────────────────

func TestHandleNodeError_OnErrorRetryExhausted(t *testing.T) {
	// Spawn a simulator that always fails. With on_error: retry, max_retries:
	// 2, the workflow attempts 3 spawns total (1 + 2 retries) then propagates
	// the failure. The workflow's NextAction is Done (Output is set, which
	// is the workflow-machine signaling convention) but Output carries the
	// "error" key with the retry-exhaustion message.
	registerSimulator(t)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "always-fails",
				"task": map[string]any{
					"type":             "simulator",
					"fail_probability": 1.0,
					"fail_message":     "synthetic failure",
				},
				"on_error": map[string]any{
					"strategy":    "retry",
					"max_retries": 2,
				},
			},
		},
	}
	r := runWorkflow(t, cfg, 5*time.Second)
	out, _ := r.Output.(map[string]any)
	if out == nil {
		t.Fatalf("expected map output, got %T = %v", r.Output, r.Output)
	}
	errMsg, _ := out["error"].(string)
	if errMsg == "" {
		t.Fatalf("output missing 'error' key; got %+v", out)
	}
	if !strings.Contains(errMsg, "2 retries") {
		t.Errorf("output.error = %q, want it to mention '2 retries' (handleNodeError exhaustion suffix)", errMsg)
	}
	// D1 quirk #2: the typed Failure companion should mirror the map shape.
	if r.Failure == nil {
		t.Fatal("expected typed RunResult.Failure to be populated (quirk #2)")
	}
	if !strings.Contains(r.Failure.Reason, "2 retries") {
		t.Errorf("Failure.Reason = %q, want it to mention '2 retries'", r.Failure.Reason)
	}
	if r.Failure.Retries != 2 {
		t.Errorf("Failure.Retries = %d, want 2", r.Failure.Retries)
	}
	if r.Failure.Child != "always-fails" {
		t.Errorf("Failure.Child = %q, want 'always-fails' (the spawn name)", r.Failure.Child)
	}
}

func TestHandleNodeError_OnErrorContinue(t *testing.T) {
	// Spawn that always fails with on_error: continue. The workflow should
	// skip the failed step and complete normally with the next step's result.
	registerSimulator(t)

	cfg := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "always-fails",
				"task": map[string]any{
					"type":             "simulator",
					"fail_probability": 1.0,
				},
				"on_error": map[string]any{
					"strategy": "continue",
				},
			},
			map[string]any{
				"name": "after-failure",
				"task": map[string]any{
					"type":   "simulator",
					"output": map[string]any{"reached": true},
				},
			},
			map[string]any{"kind": "return", "value": map[string]any{"survived": true}},
		},
	}
	r := runWorkflow(t, cfg, 5*time.Second)
	out, _ := r.Output.(map[string]any)
	if out == nil {
		t.Fatalf("expected map output, got %T = %v", r.Output, r.Output)
	}
	if _, hasErr := out["error"]; hasErr {
		t.Errorf("OnErrorContinue should not surface error key; got %+v", out)
	}
	if out["survived"] != true {
		t.Errorf("OnErrorContinue: workflow should reach the final return; got %+v", out)
	}
}

// ── stepReturn map recursion (D1 quirk #1) ────────────────────────────

// TestStepReturn_RecursesIntoMapValues — D1 quirk #1 closure. Before this
// fix, `return: {x: "${expr}", nested: {y: "${other}"}}` returned the
// expression strings literally because stepReturn only evaluated the
// top-level if it was a string. Now resolveReturnValue walks the value
// tree and evaluates every wrapped string.
//
// Note: string literals in `assign.value` are wrapped as `"${'alice'}"`
// to dodge D1 quirk #3 (bare-string `"alice"` evaluates to nil via the
// legacy bare-expr eval path). That quirk is a separate fix scoped out
// of this test.
func TestStepReturn_RecursesIntoMapValues(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "assign", "target": "name", "value": "${'alice'}"},
			map[string]any{"kind": "assign", "target": "count", "value": 42},
			map[string]any{
				"kind": "return",
				"value": map[string]any{
					"who":   "${state.name}",
					"score": "${state.count}",
					"deep": map[string]any{
						"hello": "${state.name + '!'}",
					},
					"list": []any{
						"${state.count}",
						"${'plain'}", // wrapped literal to dodge quirk #3
					},
					"literal_int": 7, // non-string passes through untouched
				},
			},
		},
	}
	out := outputMap(t, runWorkflow(t, cfg, 2*time.Second))
	if out["who"] != "alice" {
		t.Errorf("top-level ${...}: got %v, want 'alice'", out["who"])
	}
	if n, ok := toInt64(out["score"]); !ok || n != 42 {
		t.Errorf("top-level numeric expr: got %v (%T), want 42", out["score"], out["score"])
	}
	deep, ok := out["deep"].(map[string]any)
	if !ok {
		t.Fatalf("deep should be a map; got %T = %v", out["deep"], out["deep"])
	}
	if deep["hello"] != "alice!" {
		t.Errorf("nested ${...}: got %v, want 'alice!'", deep["hello"])
	}
	list, ok := out["list"].([]any)
	if !ok {
		t.Fatalf("list should be []any; got %T = %v", out["list"], out["list"])
	}
	if n, ok := toInt64(list[0]); !ok || n != 42 {
		t.Errorf("list[0] expr: got %v (%T), want 42", list[0], list[0])
	}
	if list[1] != "plain" {
		t.Errorf("list[1] wrapped literal: got %v, want 'plain'", list[1])
	}
	if n, ok := toInt64(out["literal_int"]); !ok || n != 7 {
		t.Errorf("non-string literal: got %v (%T), want 7", out["literal_int"], out["literal_int"])
	}
}

// ── small helpers ─────────────────────────────────────────────────────

// toInt64 coerces common numeric types to int64. expr-lang returns int64 for
// integer arithmetic; YAML/JSON sometimes hand us int or float64 depending
// on the source. Mirrors the matcher's toInt64 in the L2 runner.
func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}
