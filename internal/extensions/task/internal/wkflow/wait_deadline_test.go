// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A wait keeps the absolute deadline it took when it began: a 24-hour wait
// rechecked after 23 hours — a restore's tick, a restart, the bus reconnected —
// arms what is left, about an hour, never 24 hours again. So for a child wait
// (rechecked while its child runs) and for a timer. A frame saved before the
// step kept a deadline arms the full length, as before.
func TestARecheckedWaitKeepsItsAbsoluteDeadline(t *testing.T) {
	clock := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	defer func(old func() time.Time) { now = old }(now)
	now = func() time.Time { return clock }

	const day = 24 * time.Hour
	cases := map[string][]any{
		"a child wait": {
			map[string]any{"name": "x", "mode": "background", "task": map[string]any{"type": "simulator"}},
			map[string]any{"kind": "wait", "for_event": "child.x.done", "timeout_ms": int(day / time.Millisecond)},
		},
		"a timer": {
			map[string]any{"name": "pause", "kind": "wait", "duration_ms": int(day / time.Millisecond)},
		},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			clock = time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
			m, err := (&workflowFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{"steps": steps}})
			if err != nil {
				t.Fatal(err)
			}
			wm := m.(*workflowMachine)
			wm.spawner = &outcomeSpawner{} // x runs: never ended
			state, res := stepUntilWaiting(t, wm)
			if res.Wait.Timeout != day {
				t.Fatalf("armed with %v; want 24h", res.Wait.Timeout)
			}
			step := func(ev machine.Event) *machine.StepResult {
				t.Helper()
				action, err := wm.Step(context.Background(), &state, ev)
				if err != nil {
					t.Fatal(err)
				}
				r, err := action(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if r.Wait == nil {
					t.Fatalf("a %s %q ended the wait", ev.Type, ev.Label)
				}
				return r
			}
			clock = clock.Add(23 * time.Hour)
			rechecks := []machine.Event{
				{Type: machine.TickEvent}, // a restore's tick
				{Type: machine.RestartedEvent},
			}
			if name == "a child wait" { // the bus reconnected: published on the wait's topic
				rechecks = append(rechecks, machine.Event{Label: "event", Payload: map[string]any{"recheck": "the bus reconnected"}})
			}
			for _, ev := range rechecks {
				if got := step(ev).Wait.Timeout; got != time.Hour {
					t.Errorf("rechecked after 23h by a %s %q: armed with %v; want 1h", ev.Type, ev.Label, got)
				}
			}
			clock = clock.Add(2 * time.Hour) // past the deadline: fires at once
			if got := step(machine.Event{Type: machine.TickEvent}).Wait.Timeout; got <= 0 || got > time.Millisecond {
				t.Errorf("rechecked past its deadline: armed with %v; want at once", got)
			}

			// A frame saved before its step kept a deadline: the full length.
			ws := state.Data.(*WorkflowState)
			frame := ws.Active[len(ws.Active)-1]
			delete(frame.Local, deadlineKey)
			if got := step(machine.Event{Type: machine.TickEvent}).Wait.Timeout; got != day {
				t.Errorf("a frame with no deadline: armed with %v; want 24h as before", got)
			}
		})
	}
}
