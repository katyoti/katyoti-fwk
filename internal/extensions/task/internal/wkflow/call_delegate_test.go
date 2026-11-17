// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ── loader validations ─────────────────────────────────────────────

func TestLoadNodes_DelegateFormExclusivity(t *testing.T) {
	// Two forms at once → error.
	_, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{
			"name": "x", "kind": "delegate",
			"uri":  "ktw-x:tasks/foo",
			"task": map[string]any{"type": "simulator"},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("two addressing forms must be rejected, got %v", err)
	}

	// Zero forms → error.
	_, err = LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "x", "kind": "delegate"},
	}})
	if err == nil {
		t.Error("delegate without any addressing form must be rejected")
	}

	// URI form parses.
	root, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "x", "kind": "delegate", "uri": "ktw-x:tasks/foo", "mode": "background", "max_duration_s": 30},
	}})
	if err != nil {
		t.Fatalf("uri form: %v", err)
	}
	d := root.Sequence.Steps[0].Delegate
	if d.URI != "ktw-x:tasks/foo" || d.Mode != "background" || d.MaxDurationS != 30 {
		t.Errorf("delegate fields = %+v", d)
	}
}

func TestLoadNodes_CallValidations(t *testing.T) {
	// Missing uri → error.
	if _, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "c", "kind": "call"},
	}}); err == nil {
		t.Error("call without uri must be rejected")
	}
	// Dimension params on call → error (not silent ignore).
	if _, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:svc/m", "mode": "background"},
	}}); err == nil || !strings.Contains(err.Error(), "not valid on a call") {
		t.Error("dimension params on call must be rejected loudly")
	}
	// Missing name → error (it keys results + events).
	if _, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"kind": "call", "uri": "ktw-x:svc/m"},
	}}); err == nil {
		t.Error("call without name must be rejected")
	}
	// Timeout defaulted when absent; cap enforced.
	root, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:svc/m"},
	}})
	if err != nil {
		t.Fatalf("minimal call: %v", err)
	}
	if got := root.Sequence.Steps[0].Call.TimeoutMs; got != DefaultCallTimeoutMs {
		t.Errorf("TimeoutMs = %d, want default %d", got, DefaultCallTimeoutMs)
	}
	if _, err := LoadNodes(map[string]any{"steps": []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:svc/m", "timeout_ms": MaxCallTimeoutMs + 1},
	}}); err == nil {
		t.Error("timeout above cap must be rejected")
	}
}

// ── call runtime (stub ActionService via the resolve seam) ─────────

type stubActions struct {
	run func(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error)
}

func (s stubActions) Describe(_ context.Context, in actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{URI: in.URI}, nil
}
func (s stubActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	return s.run(ctx, in)
}

func withStubActions(t *testing.T, svc actionsapi.ActionService) {
	t.Helper()
	prev := resolveActionService
	resolveActionService = func(_ context.Context) (actionsapi.ActionService, error) { return svc, nil }
	t.Cleanup(func() { resolveActionService = prev })
}

func runCallWorkflow(t *testing.T, steps []any) (any, []*engine.CallCompletedEvent) {
	t.Helper()
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	var events []*engine.CallCompletedEvent
	bus.Subscribe("call.c.completed", func(_ string, payload any) {
		if evt, ok := payload.(*engine.CallCompletedEvent); ok {
			events = append(events, evt)
		}
	})

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: map[string]any{"steps": steps}})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	exec := engine.NewExecutor(m, channel, nil)
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := exec.Run(runCtx, engine.Event{Type: "start"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return result.Output, events
}

func TestCall_Success_ResultAndEvent(t *testing.T) {
	withStubActions(t, stubActions{run: func(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
		// Echo the input back so binding resolution is covered too.
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"echo": 42}`)}, nil
	}})

	output, events := runCallWorkflow(t, []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:some-service/lookup", "input": map[string]any{"q": "hello"}},
		map[string]any{"kind": "return", "value": "${results}"},
	})

	om := output.(map[string]any)
	res, ok := om["c"].(map[string]any)
	if !ok || res["echo"] != float64(42) {
		t.Errorf("results.c = %#v, want the decoded Value", om["c"])
	}
	if len(events) != 1 || events[0].Outcome != "ok" || events[0].Attempt != 1 || events[0].URI != "ktw-x:some-service/lookup" {
		t.Errorf("call.completed events = %+v, want one ok attempt 1", events)
	}
}

func TestCall_TaskURI_RejectedTowardDelegate(t *testing.T) {
	withStubActions(t, stubActions{run: func(_ context.Context, _ actionsapi.RunInput) (actionsapi.RunOutput, error) {
		t.Fatal("Run must not be invoked for a task URI")
		return actionsapi.RunOutput{}, nil
	}})

	output, _ := runCallWorkflow(t, []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:tasks/deep-research"},
		map[string]any{"kind": "return", "value": "${results}"},
	})
	om := output.(map[string]any)
	errStr, _ := om["error"].(string)
	if !strings.Contains(errStr, "delegate") {
		t.Errorf("output.error = %q, want redirect to delegate", errStr)
	}
}

func TestCall_RetryOnError_EventPerAttempt(t *testing.T) {
	var calls atomic.Int32
	withStubActions(t, stubActions{run: func(_ context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
		if calls.Add(1) < 3 {
			return actionsapi.RunOutput{}, fmt.Errorf("transient boom")
		}
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`"recovered"`)}, nil
	}})

	output, events := runCallWorkflow(t, []any{
		map[string]any{
			"name": "c", "kind": "call", "uri": "ktw-x:svc/m",
			"on_error": map[string]any{"strategy": "retry", "max_retries": 3},
		},
		map[string]any{"kind": "return", "value": "${results}"},
	})

	om := output.(map[string]any)
	if om["c"] != "recovered" {
		t.Errorf("results.c = %#v, want recovered after retries", om["c"])
	}
	if len(events) != 3 {
		t.Fatalf("got %d call.completed events, want 3 (one per attempt)", len(events))
	}
	if events[0].Outcome != "error" || events[1].Outcome != "error" || events[2].Outcome != "ok" {
		t.Errorf("attempt outcomes = %s/%s/%s, want error/error/ok", events[0].Outcome, events[1].Outcome, events[2].Outcome)
	}
	if events[2].Attempt != 3 {
		t.Errorf("final attempt = %d, want 3", events[2].Attempt)
	}
}

func TestCall_Timeout_FailsLoud(t *testing.T) {
	withStubActions(t, stubActions{run: func(ctx context.Context, _ actionsapi.RunInput) (actionsapi.RunOutput, error) {
		<-ctx.Done() // honor ctx like a well-behaved runner
		return actionsapi.RunOutput{}, ctx.Err()
	}})

	start := time.Now()
	output, events := runCallWorkflow(t, []any{
		map[string]any{"name": "c", "kind": "call", "uri": "ktw-x:svc/slow", "timeout_ms": 100},
		map[string]any{"kind": "return", "value": "${results}"},
	})
	if dur := time.Since(start); dur > 2*time.Second {
		t.Fatalf("took %s, want the 100ms timeout to bound the call", dur)
	}
	om := output.(map[string]any)
	errStr, _ := om["error"].(string)
	if !strings.Contains(errStr, "timeout_ms=100") {
		t.Errorf("output.error = %q, want timeout context", errStr)
	}
	if len(events) != 1 || events[0].Outcome != "timeout" {
		t.Errorf("events = %+v, want one timeout outcome", events)
	}
}

// ── delegate background mode ───────────────────────────────────────

func TestDelegate_BackgroundMode_ImmediateHandle(t *testing.T) {
	registerSimulatorForWkflow(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)

	factory := &workflowFactory{}
	m, err := factory.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: map[string]any{"steps": []any{
		map[string]any{
			"name": "bg-child",
			"mode": "background",
			"task": map[string]any{"type": "simulator", "duration_ms": 400, "output": "late"},
		},
		map[string]any{"kind": "return", "value": "${results}"},
	}}})
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

	// The workflow must NOT have awaited the 400ms child.
	if dur > 300*time.Millisecond {
		t.Errorf("workflow took %s — background delegate must not suspend on the child", dur)
	}
	om := result.Output.(map[string]any)
	handle, ok := om["bg-child"].(map[string]any)
	if !ok {
		t.Fatalf("results.bg-child = %#v, want the background handle map", om["bg-child"])
	}
	if handle["background"] != true || handle["run_id"] == "" || handle["run_id"] == nil {
		t.Errorf("handle = %#v, want background=true with a run_id", handle)
	}
}
