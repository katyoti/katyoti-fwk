// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// registerSimulatorForCore registers the simulator factory in MachineDomain
// so Service.Run can dispatch `type: simulator`. Idempotent across tests.
func registerSimulatorForCore(t *testing.T) {
	t.Helper()
	_, createFn := simulator.MachineStrategyDef.Strategy.NewTypeConfig()
	raw, err := createFn(context.Background())
	if err != nil {
		t.Fatalf("simulator factory: %v", err)
	}
	td := simulator.MachineStrategyDef.Strategy.CreateTypeDef("simulator", raw)
	_ = engine.MachineDomain.Types.Contribute(td)
}

// newServiceForLifecycleTest builds a minimal *Service with a simulator
// factory registered + a single `uc-130` task pre-registered. Bypasses
// NewTaskService's heavy bootstrap so the test exercises just the Run
// lifecycle. The returned channel + service share an in-memory bus so
// subscribers can observe task.output events.
func newServiceForLifecycleTest(t *testing.T) (*Service, *engine.Channel) {
	t.Helper()
	registerSimulatorForCore(t)

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	bgMgr := NewBackgroundTaskManager(channel)

	tasks := map[string]*taskapi.Task{
		"uc-130-task": {
			Name:     "uc-130-task",
			Provider: "test",
			Type:     "simulator",
			Config: map[string]any{
				"duration_ms": 0,
				"output":      map[string]any{"summary": "uc-130-ok"},
			},
		},
	}

	svc := &Service{
		tasks:   tasks,
		channel: channel,
		bgMgr:   bgMgr,
		reg:     newRunRegistry(),
	}
	// Wire circular refs so bgManager can dispatch via the service if needed.
	if bg, ok := bgMgr.(*bgManager); ok {
		bg.taskSvc = svc
		bg.appCtx = context.Background()
	}
	return svc, channel
}

// TestRun_CreatesRunEntry (UC-130) — Service.Run inserts a runEntry into
// the runs map keyed by the returned RunID, and the entry transitions from
// Running to Completed once the underlying machine finishes.
func TestRun_CreatesRunEntry(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)

	resp, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName: "uc-130-task",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.RunID == "" {
		t.Fatal("Run did not produce a RunID")
	}

	// Entry must be in the runs map immediately after Run returns.
	entry, ok := svc.reg.get(resp.RunID)
	if !ok {
		t.Fatalf("runs[%q] not found", resp.RunID)
	}

	// Run answers the run's status as it stands once launched: Running, or
	// already Completed when the run's goroutine finished first (a zero-length
	// simulator run on a fast or single-CPU runner does) — never anything else.
	if resp.Status != taskapi.RunStatusRunning && resp.Status != taskapi.RunStatusCompleted {
		t.Errorf("initial status = %q, want %q or %q", resp.Status, taskapi.RunStatusRunning, taskapi.RunStatusCompleted)
	}

	// Wait for completion via the entry's done channel.
	select {
	case <-entry.done:
	case <-time.After(2 * time.Second):
		t.Fatal("entry.done not closed within 2s")
	}

	// Terminal status is Completed for a successful simulator run.
	svc.mu.RLock()
	finalStatus := entry.info.Status
	svc.mu.RUnlock()
	if finalStatus != taskapi.RunStatusCompleted {
		t.Errorf("final status = %q, want %q", finalStatus, taskapi.RunStatusCompleted)
	}
}

// TestRun_CompletionEvent (UC-131) — Service.Run publishes a `task.output`
// event on the bus when the run completes (the Phase C.2 terminal event).
// Subscribers can observe the run's final Output without polling.
func TestRun_CompletionEvent(t *testing.T) {
	svc, channel := newServiceForLifecycleTest(t)

	evtCh := make(chan *engine.TaskOutputEvent, 1)
	channel.Bus().Subscribe(engine.EventTaskOutput, func(_ string, payload any) {
		if evt, ok := payload.(*engine.TaskOutputEvent); ok {
			select {
			case evtCh <- evt:
			default:
			}
		}
	})

	resp, err := svc.Run(context.Background(), taskapi.TaskRunRequest{
		TaskName: "uc-130-task", // same simulator task; produces a map output
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case evt := <-evtCh:
		if evt.RunID != resp.RunID {
			t.Errorf("event.RunID = %q, want %q", evt.RunID, resp.RunID)
		}
		if evt.Status != string(taskapi.RunStatusCompleted) {
			t.Errorf("event.Status = %q, want %q", evt.Status, taskapi.RunStatusCompleted)
		}
		if evt.Output == nil {
			t.Error("event.Output is nil; simulator's output map should have ridden through")
		} else if evt.Output["summary"] != "uc-130-ok" {
			t.Errorf("output.summary = %v, want uc-130-ok", evt.Output["summary"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UC-131: task.output event not published within 2s")
	}
}
