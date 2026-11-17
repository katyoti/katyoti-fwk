// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A workflow's wait for a duration is a machine wait — a timer: the step
// returns at once with a wait that only a timeout ends, so the run holds no
// slot meanwhile and its deadline is saved with it (a restart keeps it).
func TestADurationWaitIsATimerNotABlockedStep(t *testing.T) {
	m, err := (&workflowFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{
		"steps": []any{map[string]any{"name": "pause", "kind": "wait", "duration_ms": 60000}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := m.InitialState(machine.Event{})
	for i := 0; i < 5; i++ {
		action, err := m.Step(context.Background(), &state, machine.Event{Type: machine.TickEvent})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		start := time.Now()
		res, err := action(ctx)
		cancel()
		if err != nil || time.Since(start) > 500*time.Millisecond {
			t.Fatalf("step %d took %v (%v) — the duration wait blocked inside its step", i, time.Since(start), err)
		}
		state.Name, state.Data = res.NextState, res.Data
		if res.Wait != nil {
			if res.Wait.Timeout != time.Minute || len(res.Wait.Conditions) != 0 {
				t.Errorf("wait = %+v; want a 60 s timer and nothing else", res.Wait)
			}
			return
		}
	}
	t.Fatal("no wait returned: the duration is not waited as a machine")
}
