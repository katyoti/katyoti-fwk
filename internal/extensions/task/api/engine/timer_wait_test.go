// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"
)

// timerMachine waits on a timer alone, then ends with the event that woke it.
type timerMachine struct{}

func (timerMachine) Name() string             { return "timer" }
func (timerMachine) InitialState(Event) State { return State{Name: "init"} }
func (timerMachine) Step(_ context.Context, st *State, ev Event) (ActionFunc, error) {
	return func(context.Context) (*StepResult, error) {
		if st.Name == "init" {
			return &StepResult{NextState: "waiting", Wait: &WaitSpec{Mode: WaitAny, Timeout: 50 * time.Millisecond, TimeoutLabel: "timeout"}}, nil
		}
		return &StepResult{NextState: "done", Output: map[string]any{"woke": ev.Label}}, nil
	}, nil
}

// A wait with no condition, only a timeout, is a timer: the run waits — its
// checkpoint saved — until the timeout ends it.
func TestATimerAloneEndsAWait(t *testing.T) {
	store := NewMemoryStateStore()
	ex := NewExecutor(timerMachine{}, nil, store)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := ex.Run(ctx, Event{Type: "start"})
	if err != nil || res.Next != Done {
		t.Fatalf("run = %+v, %v; want done by the timer", res, err)
	}
	if out, _ := res.Output.(map[string]any); out["woke"] != "timeout" {
		t.Errorf("woke by %v, want the timeout", res.Output)
	}
}
