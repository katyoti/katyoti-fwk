// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// runSimulatorWithErr is a variant of runSimulator that surfaces the
// executor-level err so failure-path tests can assert on the error message.
// (Mirrors runEcho's pattern from the echo strategy fixture.)
func runSimulatorWithErr(t *testing.T, request *taskapi.TaskExecRequest) (any, error) {
	t.Helper()
	f := &simulatorFactory{}
	m, err := f.CreateMachine(context.Background(), request)
	if err != nil {
		return nil, err
	}
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	executor := engine.NewExecutor(m, channel, nil)
	result, err := executor.Run(context.Background(), engine.Event{Type: "start", Payload: request})
	if err != nil {
		return nil, err
	}
	if result.Next == engine.Failed && result.Error != nil {
		return nil, result.Error
	}
	return result.Output, nil
}

// TestSimulator_DurationMs_Wallclock (UC-151) — DurationMs produces a
// measurable wallclock delay. Asserts that a 100ms sleep takes ≥100ms and
// finishes promptly (≤300ms) to detect both regression and runaway sleep.
func TestSimulator_DurationMs_Wallclock(t *testing.T) {
	req := &taskapi.TaskExecRequest{
		Config: map[string]any{
			"duration_ms": 100,
			"output":      "done",
		},
	}
	start := time.Now()
	_, err := runSimulatorWithErr(t, req)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if dur < 100*time.Millisecond {
		t.Errorf("duration = %s, want ≥100ms (sleep didn't fire)", dur)
	}
	if dur > 300*time.Millisecond {
		t.Errorf("duration = %s, want ≤300ms (runaway sleep)", dur)
	}
}

// TestSimulator_FailProbability_Statistical (UC-152) — FailProbability
// triggers failures matching the configured rate over N=200 runs.
// With p=0.5, expect failures in [70, 130] (statistical bound — wider than
// 3σ for N=200 binomial to avoid flake at scale).
func TestSimulator_FailProbability_Statistical(t *testing.T) {
	if testing.Short() {
		t.Skip("statistical test skipped under -short")
	}

	const (
		n     = 200
		lower = 70
		upper = 130
	)

	fails := 0
	for i := 0; i < n; i++ {
		req := &taskapi.TaskExecRequest{
			Config: map[string]any{
				"duration_ms":      0,
				"fail_probability": 0.5,
			},
		}
		_, err := runSimulatorWithErr(t, req)
		if err != nil {
			fails++
		}
	}
	if fails < lower || fails > upper {
		t.Errorf("UC-152: with p=0.5, N=200, expected fails in [%d, %d], got %d", lower, upper, fails)
	}
}

// TestSimulator_FailMessage_Propagates (UC-153) — when FailProbability=1.0
// AND a FailMessage is set, the run's error matches the configured message
// verbatim.
func TestSimulator_FailMessage_Propagates(t *testing.T) {
	req := &taskapi.TaskExecRequest{
		Config: map[string]any{
			"duration_ms":      0,
			"fail_probability": 1.0,
			"fail_message":     "uc-153-explicit-message",
		},
	}
	_, err := runSimulatorWithErr(t, req)
	if err == nil {
		t.Fatal("expected error from FailProbability=1.0")
	}
	if err.Error() != "uc-153-explicit-message" {
		t.Errorf("err = %q, want %q", err.Error(), "uc-153-explicit-message")
	}
}
