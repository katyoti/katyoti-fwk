// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// modelessWaiter waits with a WaitSpec that names no mode, on two topics.
type modelessWaiter struct{}

func (modelessWaiter) Name() string             { return "modeless-waiter" }
func (modelessWaiter) InitialState(Event) State { return State{Name: "init"} }
func (modelessWaiter) Step(_ context.Context, st *State, ev Event) (ActionFunc, error) {
	if st.Name == "init" {
		return func(context.Context) (*StepResult, error) {
			return &StepResult{NextState: "woken", Wait: &WaitSpec{Conditions: []WaitCondition{{Topic: "a"}, {Topic: "b"}}}}, nil
		}, nil
	}
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "done", Output: map[string]any{"type": ev.Type}}, nil
	}, nil
}

// A WaitSpec built without a mode never waits for all its conditions: the
// run is refused at its wait, at once - it used to be a wait-for-all whose met
// conditions a restart loses.
func TestAWaitWithoutAModeIsRefusedNeverAWaitForAll(t *testing.T) {
	var zero WaitMode
	if zero == WaitAll || zero.Valid() {
		t.Fatalf("the zero WaitMode is %v; want no mode", zero)
	}
	ch := NewChannel(NewMemoryEventBus())
	ex := NewExecutor(modelessWaiter{}, ch, NewMemoryStateStore())
	done := make(chan *RunResult, 1)
	go func() {
		r, _ := ex.Run(context.Background(), Event{Type: "start"})
		done <- r
	}()
	select {
	case r := <-done:
		if r.Next != Failed || !errors.Is(r.Error, ErrWaitWithoutMode) {
			t.Fatalf("run ended %v (%v); want failed with ErrWaitWithoutMode", r.Next, r.Error)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the run waits; a wait without a mode must be refused")
	}

	// The inbox refuses it too.
	if _, err := NewMachineInbox().Get(context.Background(), &WaitSpec{Conditions: []WaitCondition{{Topic: "a"}}}); !errors.Is(err, ErrWaitWithoutMode) {
		t.Fatalf("inbox Get without a mode: %v; want ErrWaitWithoutMode", err)
	}
}

// A mode is saved by its name, and a wait saved when modes were numbers
// (WaitAll 0, WaitAny 1, WaitCustom 2) is restored with its meaning.
func TestASavedWaitKeepsItsMode(t *testing.T) {
	for _, m := range []WaitMode{WaitAny, WaitAll, WaitCustom} {
		b, err := json.Marshal(WaitSpec{Mode: m})
		if err != nil {
			t.Fatal(err)
		}
		var back WaitSpec
		if err := json.Unmarshal(b, &back); err != nil || back.Mode != m {
			t.Errorf("%v saved as %s read back as %v (%v)", m, b, back.Mode, err)
		}
	}
	for saved, want := range map[string]WaitMode{
		`{"conditions":[{"topic":"x","label":"x"}],"mode":0}`: WaitAll,
		`{"conditions":[{"topic":"x","label":"x"}],"mode":1}`: WaitAny,
		`{"conditions":[{"topic":"x","label":"x"}],"mode":2}`: WaitCustom,
	} {
		var cp Checkpoint
		if err := json.Unmarshal([]byte(`{"wait":`+saved+`}`), &cp); err != nil {
			t.Fatalf("old wait %s: %v", saved, err)
		}
		if cp.Wait.Mode != want {
			t.Errorf("old wait %s restored as %v; want %v", saved, cp.Wait.Mode, want)
		}
	}
	var bad WaitSpec
	if err := json.Unmarshal([]byte(`{"mode":"wait_some"}`), &bad); err == nil {
		t.Error("an unknown mode name was read")
	}
}

// waitAnyResumer waits for one event with WaitAny, then ends with it.
type waitAnyResumer struct{}

func (waitAnyResumer) Name() string             { return "any-resumer" }
func (waitAnyResumer) InitialState(Event) State { return State{Name: "init"} }
func (waitAnyResumer) Step(_ context.Context, st *State, ev Event) (ActionFunc, error) {
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "done", Output: map[string]any{"type": ev.Type, "label": ev.Label}}, nil
	}, nil
}

// A run suspended with WaitAny before modes were saved by name (its mode
// saved as 1) is restored as a wait for its first event, not another mode.
func TestARunSavedWithANumberedModeResumesWithIt(t *testing.T) {
	var cp Checkpoint
	old := `{"state":{"name":"waiting","runId":"old-run"},"wait":{"conditions":[{"topic":"a","label":"a"},{"topic":"b","label":"b"}],"mode":1}}`
	if err := json.Unmarshal([]byte(old), &cp); err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStateStore()
	if err := store.Save("old-run", cp); err != nil {
		t.Fatal(err)
	}
	ch := NewChannel(NewMemoryEventBus())
	ex := NewExecutor(waitAnyResumer{}, ch, store)
	done := make(chan *RunResult, 1)
	go func() {
		r, _ := ex.ResumeWaiting(context.Background(), "old-run")
		done <- r
	}()
	time.Sleep(50 * time.Millisecond)
	_ = ch.Bus().Publish("b", "only-b")
	select {
	case r := <-done:
		out, _ := r.Output.(map[string]any)
		if r.Next != Done || out["label"] != "b" {
			t.Fatalf("resumed run ended %v with %v (%v); want done, woken by b alone", r.Next, out, r.Error)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the restored WaitAny run still waits after its first event: its mode changed meaning")
	}
}
