// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"
	"time"
)

// layoutMachine waits on "never" and declares the layout of its saved state.
type layoutMachine struct{}

func (layoutMachine) Name() string             { return "layouted" }
func (layoutMachine) InitialState(Event) State { return State{Name: "init"} }
func (layoutMachine) StateLayout() string      { return "layouted/2" }
func (layoutMachine) Step(_ context.Context, st *State, _ Event) (ActionFunc, error) {
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "waiting", Wait: &WaitSpec{Mode: WaitAny, Conditions: []WaitCondition{{Topic: "never"}}}}, nil
	}, nil
}

// A checkpoint names the layout of the state it holds, as its machine declares
// it: a restore can tell a state saved by another version of the machine.
func TestACheckpointNamesItsStateLayout(t *testing.T) {
	store := NewMemoryStateStore()
	ex := NewExecutor(layoutMachine{}, nil, store)
	ex.SetRunID("r1")
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _, _ = ex.Run(ctx, Event{Type: "start"}) }()
	defer cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cp, err := store.Load("r1"); err == nil {
			if cp.Layout != "layouted/2" {
				t.Fatalf("checkpoint layout = %q, want the machine's", cp.Layout)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no checkpoint saved")
}
