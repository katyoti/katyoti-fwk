// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// rechecker waits for an answer that may come with no event, checking again
// whenever it is woken (as an agent's awaiting call does).
type rechecker struct{}

var recheckerSteps atomic.Int64

func (rechecker) Name() string                           { return "rechecker" }
func (rechecker) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (rechecker) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	runID := st.RunID
	return func(context.Context) (*engine.StepResult, error) {
		recheckerSteps.Add(1)
		return &engine.StepResult{NextState: "awaiting", Wait: &engine.WaitSpec{Mode: engine.WaitAny, RecheckOnResume: true,
			Conditions: []engine.WaitCondition{{Topic: "answered." + runID}}}}, nil
	}, nil
}

// When the bus reconnects (events published meanwhile are lost), a run whose
// wait rechecks is woken once to check; a run waiting for a given event is not.
func TestAReconnectRechecksTheWaitsThatRecheck(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, rechecker{})
	ctx := context.Background()
	recheckerSteps.Store(0)
	a, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "rechecker"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "both waiting", func() bool {
		return status(t, svc, a.RunID) == taskapi.RunStatusWaiting && status(t, svc, w.RunID) == taskapi.RunStatusWaiting
	})
	steps := recheckerSteps.Load()

	svc.RecheckWaits()
	eventually(t, "the rechecking run checked again", func() bool { return recheckerSteps.Load() > steps })
	time.Sleep(50 * time.Millisecond)
	if st := status(t, svc, w.RunID); st != taskapi.RunStatusWaiting {
		t.Errorf("the run waiting for its event = %s; want it still waiting (a tick is not its event)", st)
	}
	// Woken once: its step, and the one recheck its new wait gets — then it waits.
	settled := recheckerSteps.Load()
	time.Sleep(50 * time.Millisecond)
	if got := recheckerSteps.Load(); got != settled || got > steps+2 {
		t.Errorf("stepped %d times after the reconnect; want it woken once, then waiting", got-steps)
	}
	if st := status(t, svc, a.RunID); st != taskapi.RunStatusWaiting {
		t.Errorf("the rechecking run = %s; want it waiting again", st)
	}
	wake(t, svc, w.RunID)
}
