// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestStepWait_DurationHonorsCancellation — a duration-only wait must abort
// when the run context is cancelled. Pre-fix it was a plain time.Sleep with
// the ctx discarded: `wait: {duration_ms: 3600000}` blocked Stop/cancel for
// the full hour.
func TestStepWait_DurationHonorsCancellation(t *testing.T) {
	cfg := map[string]any{
		"steps": []any{
			map[string]any{"kind": "wait", "duration_ms": 10000},
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

	runCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, err := exec.Run(runCtx, engine.Event{Type: "start"})
	dur := time.Since(start)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dur > 2*time.Second {
		t.Fatalf("Run returned after %s — the wait ignored cancellation and slept on", dur)
	}
	// The executor converts an action error into a Failed RunResult.
	if result.Next != engine.Failed || result.Error == nil {
		t.Errorf("result = {Next: %v, Error: %v}, want Failed with the cancellation error", result.Next, result.Error)
	}
}
