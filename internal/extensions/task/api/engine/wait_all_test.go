// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"
)

// allWaiter waits for two events at once, then ends with what woke it.
type allWaiter struct{}

func (allWaiter) Name() string             { return "all-waiter" }
func (allWaiter) InitialState(Event) State { return State{Name: "init"} }
func (allWaiter) Step(_ context.Context, st *State, ev Event) (ActionFunc, error) {
	if st.Name == "init" {
		return func(context.Context) (*StepResult, error) {
			return &StepResult{NextState: "woken", Wait: &WaitSpec{Mode: WaitAll, Conditions: []WaitCondition{{Topic: "a"}, {Topic: "b"}}}}, nil
		}, nil
	}
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "done", Output: map[string]any{"type": ev.Type, "events": ev.Payload}}, nil
	}, nil
}

// A machine waiting for ALL its conditions is given every event that met
// them — it was given only the first.
func TestAWaitForAllGivesEveryEvent(t *testing.T) {
	ch := NewChannel(NewMemoryEventBus())
	ex := NewExecutor(allWaiter{}, ch, NewMemoryStateStore())
	done := make(chan *RunResult, 1)
	go func() {
		r, _ := ex.Run(context.Background(), Event{Type: "start"})
		done <- r
	}()
	time.Sleep(50 * time.Millisecond)
	_ = ch.Bus().Publish("a", "first")
	_ = ch.Bus().Publish("b", "second")
	select {
	case r := <-done:
		out, _ := r.Output.(map[string]any)
		evs, _ := out["events"].([]Event)
		if out["type"] != AllEvent || len(evs) != 2 {
			t.Fatalf("woken by %v with %v; want both events", out["type"], out["events"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("never woken")
	}
}
