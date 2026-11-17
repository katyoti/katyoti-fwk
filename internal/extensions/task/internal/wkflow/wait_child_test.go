// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// outcomeSpawner starts one background child and answers its outcome.
type outcomeSpawner struct {
	outcome machine.ChildOutcome
	ended   bool
}

func (*outcomeSpawner) Spawn(_ context.Context, spec machine.ChildSpec) (machine.ChildHandle, error) {
	return machine.ChildHandle{RunID: "run-x", Name: spec.Name, Status: machine.ChildRunning}, nil
}
func (*outcomeSpawner) Cancel(string, string) error                   { return nil }
func (*outcomeSpawner) Result(string) (any, bool)                     { return nil, false }
func (*outcomeSpawner) MarkChildReady(string)                         {}
func (s *outcomeSpawner) Outcome(string) (machine.ChildOutcome, bool) { return s.outcome, s.ended }

// stepUntilWaiting steps m from its initial state until a step waits.
func stepUntilWaiting(t *testing.T, m *workflowMachine) (machine.State, *machine.StepResult) {
	t.Helper()
	state := m.InitialState(machine.Event{})
	state.RunID = "run-wf"
	for i := 0; i < 10; i++ {
		action, err := m.Step(context.Background(), &state, machine.Event{Type: machine.TickEvent})
		if err != nil {
			t.Fatal(err)
		}
		res, err := action(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		state.Name, state.Data = res.NextState, res.Data
		if res.Wait != nil {
			return state, res
		}
	}
	t.Fatal("the workflow never waited")
	return state, nil
}

// `wait for_event: child.x.done` waits on its child's run — a recheck or a
// restart does not end it; it ends when x is done; when x failed, .done will
// never come: the step fails, it does not wait for ever.
func TestAWaitForAChildSettlesFromItsOutcome(t *testing.T) {
	steps := []any{
		map[string]any{"name": "x", "mode": "background", "task": map[string]any{"type": "simulator"}},
		map[string]any{"kind": "wait", "for_event": "child.x.done"},
	}
	build := func(sp *outcomeSpawner) *workflowMachine {
		m, err := (&workflowFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{"steps": steps}})
		if err != nil {
			t.Fatal(err)
		}
		wm := m.(*workflowMachine)
		wm.spawner = sp
		return wm
	}
	sp := &outcomeSpawner{}
	m := build(sp)
	state, res := stepUntilWaiting(t, m)
	if c := res.Wait.Conditions; len(c) != 1 || c[0].Topic != machine.RunEndedTopic("run-x") || !res.Wait.RecheckOnResume {
		t.Fatalf("wait %+v; want its child's run end, rechecked", res.Wait)
	}
	step := func(ev machine.Event) *machine.StepResult {
		t.Helper()
		action, err := m.Step(context.Background(), &state, ev)
		if err != nil {
			t.Fatal(err)
		}
		r, err := action(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, ev := range []machine.Event{{Type: machine.TickEvent}, {Type: machine.RestartedEvent}} {
		if r := step(ev); r.Wait == nil {
			t.Fatalf("a %s ended the wait before its child ended", ev.Type)
		}
	}
	sp.ended, sp.outcome = true, machine.ChildOutcome{Status: machine.ChildFailed, Error: "boom"}
	if r := step(machine.Event{Type: machine.TickEvent}); r.Wait != nil || r.NextState != "failed" {
		t.Fatalf("x failed: step %+v; want the step failed, not waiting for ever", r)
	}
	sp.outcome = machine.ChildOutcome{Status: machine.ChildDone}
	if r := step(machine.Event{Type: machine.TickEvent}); r.Wait != nil || r.NextState == "failed" {
		t.Fatalf("x done: step %+v; want the wait over", r)
	}
}
