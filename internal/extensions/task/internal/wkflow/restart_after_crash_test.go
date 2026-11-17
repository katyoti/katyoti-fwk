// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// countingSpawner counts the children it is asked to spawn; ended says
// whether "child-run" has ended (done, with output "ok").
type countingSpawner struct {
	spawned int
	ended   bool
}

func (s *countingSpawner) Spawn(context.Context, machine.ChildSpec) (machine.ChildHandle, error) {
	s.spawned++
	return machine.ChildHandle{RunID: "child-run"}, nil
}
func (*countingSpawner) Cancel(string, string) error { return nil }
func (*countingSpawner) Result(string) (any, bool)   { return nil, false }
func (*countingSpawner) MarkChildReady(string)       {}
func (s *countingSpawner) Outcome(string) (machine.ChildOutcome, bool) {
	return machine.ChildOutcome{Status: machine.ChildDone, Output: "ok"}, s.ended
}

func delegatingWorkflow(t *testing.T, sp *countingSpawner) *workflowMachine {
	t.Helper()
	m, err := (&workflowFactory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: map[string]any{
		"steps": []any{map[string]any{"name": "child", "kind": "delegate", "task": map[string]any{"type": "simulator"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wm := m.(*workflowMachine)
	wm.spawner = sp
	return wm
}

// A workflow working when its process died, awaiting the child it had
// started, restarts awaiting that child — its run, restored with it — never
// starting it again; a child that ended meanwhile is settled at once.
func TestARestartedWorkflowAwaitsItsChildAgain(t *testing.T) {
	first := &countingSpawner{}
	m := delegatingWorkflow(t, first)
	if !machine.RestartsAfterCrash(m) {
		t.Fatal("the workflow does not restart after a crash")
	}
	state := m.InitialState(machine.Event{})
	for i := 0; i < 5 && first.spawned == 0; i++ { // sequence → delegate: spawn
		action, err := m.Step(context.Background(), &state, machine.Event{Type: machine.TickEvent})
		if err != nil {
			t.Fatal(err)
		}
		res, err := action(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		state.Name, state.Data = res.NextState, res.Data
	}
	if first.spawned != 1 {
		t.Fatalf("spawned %d before the crash, want 1", first.spawned)
	}

	// Saved, the process dies, read back in a new one.
	b, err := json.Marshal(state.Data)
	if err != nil {
		t.Fatal(err)
	}
	var saved any
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	again := &countingSpawner{}
	m2 := delegatingWorkflow(t, again)
	restored := machine.State{Name: state.Name, Data: saved}
	action, err := m2.Step(context.Background(), &restored, machine.Event{Type: machine.RestartedEvent})
	if err != nil {
		t.Fatalf("restarted: %v", err)
	}
	res, err := action(context.Background())
	if err != nil {
		t.Fatalf("restarted step: %v", err)
	}
	if again.spawned != 0 || res.Wait == nil || len(res.Wait.Conditions) != 1 || res.Wait.Conditions[0].Topic != machine.RunEndedTopic("child-run") {
		t.Fatalf("after the restart: spawned %d, wait %+v; want its child awaited (its run), not started again", again.spawned, res.Wait)
	}

	// The child ended meanwhile: the restarted workflow settles it at once.
	ended := &countingSpawner{ended: true}
	m3 := delegatingWorkflow(t, ended)
	var saved2 any
	_ = json.Unmarshal(b, &saved2)
	restored2 := machine.State{Name: state.Name, Data: saved2}
	action, err = m3.Step(context.Background(), &restored2, machine.Event{Type: machine.RestartedEvent})
	if err != nil {
		t.Fatal(err)
	}
	res, err = action(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ws := res.Data.(*WorkflowState)
	if ended.spawned != 0 || ws.Results["child"] != "ok" {
		t.Errorf("a child that ended while the workflow was down: spawned %d, results %v; want its result, nothing started", ended.spawned, ws.Results)
	}
}
