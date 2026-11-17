// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// T4 — TaskService.Run end-to-end coverage. The 643-LOC core had zero
// direct tests on the Run/Result/Status surface beyond UC-130 (run-entry
// creation) and UC-131 (task_output event). This file fills the spec'd
// gaps: missing-task error, Result retrieval, ParentRunID propagation,
// ConfigOverride merging, channel wiring.
//
// Skill auto-wrap (skill_task_library) is a separate library
// implementation, not Service.Run's surface — tracked separately as a
// tier-2 follow-up.

// TestServiceRun_UnknownTaskName — Run with a non-existent TaskName returns
// a clear error citing the missing task.
func TestServiceRun_UnknownTaskName(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	_, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName: "this-task-does-not-exist",
	})
	if err == nil {
		t.Fatal("Run with unknown TaskName should fail")
	}
	if !strings.Contains(err.Error(), "this-task-does-not-exist") {
		t.Errorf("error %q should cite the missing task name", err.Error())
	}
}

// TestServiceRun_NoTarget — Run with neither TaskName nor TaskFile nor
// TaskDef returns a clear error. Defensive guard on the union field.
func TestServiceRun_NoTarget(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	_, err := svc.Run(context.Background(), taskapi.TaskRunRequest{})
	if err == nil {
		t.Fatal("Run without any target should fail")
	}
}

// TestServiceRun_ResultReturnsCompletedOutput — Result(runID) blocks until
// terminal and returns the run's final Output map. Verifies the read-side
// of the lifecycle.
func TestServiceRun_ResultReturnsCompletedOutput(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	resp, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName: "uc-130-task",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	res, err := svc.Result(context.Background(), taskapi.ResultRequest{RunID: resp.RunID})
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if res.Error != "" {
		t.Errorf("Result.Error = %q, want empty", res.Error)
	}
	if res.Output == nil {
		t.Fatal("Result.Output is nil")
	}
	out, ok := res.Output.(map[string]any)
	if !ok {
		t.Fatalf("Result.Output type = %T, want map[string]any", res.Output)
	}
	if out["summary"] != "uc-130-ok" {
		t.Errorf("Result.Output[summary] = %v, want 'uc-130-ok'", out["summary"])
	}
}

// TestServiceRun_ResultUnknownRunID — Result on a never-issued run returns
// an error rather than blocking forever.
func TestServiceRun_ResultUnknownRunID(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := svc.Result(ctx, taskapi.ResultRequest{RunID: "run-nope-doesnt-exist"})
	if err == nil {
		t.Fatal("Result on unknown RunID should error, not block")
	}
}

// TestServiceRun_ParentRunIDPropagates — Run with an explicit ParentRunID
// stamps the entry's RunInfo with that parent. Confirms the field flows
// through unaltered (used by downstream observers to reconstruct the
// spawn tree).
func TestServiceRun_ParentRunIDPropagates(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	resp, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName:    "uc-130-task",
		ParentRunID: "run-parent-fixture",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Wait for completion so the entry has settled (avoids racing the
	// runEntry's info field with the lifecycle goroutine).
	res, err := svc.Result(context.Background(), taskapi.ResultRequest{RunID: resp.RunID})
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	_ = res

	entry, _ := svc.reg.get(resp.RunID)
	gotParent := ""
	if entry != nil {
		gotParent = entry.info.ParentRunID
	}
	if gotParent != "run-parent-fixture" {
		t.Errorf("RunInfo.ParentRunID = %q, want 'run-parent-fixture' (propagation from request)", gotParent)
	}
}

// TestServiceRun_ConfigOverrideMergesOnTaskConfig — ConfigOverride values
// merge onto the task's static config: block. Verified by observing the
// simulator's output field, which is configurable via the override.
func TestServiceRun_ConfigOverrideMergesOnTaskConfig(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	resp, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName: "uc-130-task",
		ConfigOverride: map[string]any{
			// The fixture task ships `output: {summary: "uc-130-ok"}`. Override
			// it via the request — the simulator should return our override
			// value, proving the merge fired.
			"output": map[string]any{"summary": "override-took-effect"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	res, err := svc.Result(context.Background(), taskapi.ResultRequest{RunID: resp.RunID})
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	out, ok := res.Output.(map[string]any)
	if !ok {
		t.Fatalf("Result.Output type = %T, want map", res.Output)
	}
	if out["summary"] != "override-took-effect" {
		t.Errorf("ConfigOverride didn't reach the strategy: summary = %v, want 'override-took-effect'", out["summary"])
	}
}

// TestServiceRun_ChannelWiringSharedBetweenRuns — concurrent Run() calls
// share the Service's Channel (one EventBus, one Channel for the process).
// Subscribers see events from any active run. Guard against a future
// refactor that accidentally creates per-run channels and breaks the
// existing multi-agent dispatch model.
func TestServiceRun_ChannelWiringSharedBetweenRuns(t *testing.T) {
	svc, channel := newServiceForLifecycleTest(t)

	runs := make(map[string]bool)
	var ch = make(chan string, 4)
	channel.Bus().Subscribe(engine.EventTaskOutput, func(_ string, payload any) {
		if evt, ok := payload.(*engine.TaskOutputEvent); ok {
			ch <- evt.RunID
		}
	})

	resp1, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "uc-130-task"})
	if err != nil {
		t.Fatalf("Run #1: %v", err)
	}
	resp2, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "uc-130-task"})
	if err != nil {
		t.Fatalf("Run #2: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for len(runs) < 2 {
		select {
		case id := <-ch:
			runs[id] = true
		case <-timeout:
			t.Fatalf("only observed %d task_output events, want 2 (runs: %+v)", len(runs), runs)
		}
	}
	if !runs[resp1.RunID] {
		t.Errorf("missing task_output for run #1 (%s); got %+v", resp1.RunID, runs)
	}
	if !runs[resp2.RunID] {
		t.Errorf("missing task_output for run #2 (%s); got %+v", resp2.RunID, runs)
	}
}
