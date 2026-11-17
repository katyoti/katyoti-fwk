// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Test 1: Passthrough roundtrip (stateless service call) ---

type mockService struct{}

type echoInput struct {
	Message string `json:"message"`
}
type echoOutput struct {
	Echo string `json:"echo"`
}

func (s *mockService) Echo(ctx context.Context, in echoInput) (echoOutput, error) {
	return echoOutput{Echo: "echo: " + in.Message}, nil
}

func (s *mockService) Fail(ctx context.Context, in echoInput) (echoOutput, error) {
	return echoOutput{}, fmt.Errorf("forced error")
}

func TestPassthrough_Roundtrip(t *testing.T) {
	machine := WrapInterface("mock", &mockService{})
	exec := NewExecutor(machine, nil, nil)

	result, err := exec.Run(context.Background(), Event{
		Type:    "Echo",
		Payload: echoInput{Message: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Next != Done {
		t.Fatalf("expected Done, got %s", result.Next)
	}
	out, ok := result.Output.(echoOutput)
	if !ok {
		t.Fatalf("output type: got %T, want echoOutput", result.Output)
	}
	if out.Echo != "echo: hello" {
		t.Errorf("echo = %q, want %q", out.Echo, "echo: hello")
	}
}

func TestPassthrough_MethodError(t *testing.T) {
	machine := WrapInterface("mock", &mockService{})
	exec := NewExecutor(machine, nil, nil)

	result, err := exec.Run(context.Background(), Event{
		Type:    "Fail",
		Payload: echoInput{Message: "x"},
	})
	if err != nil {
		t.Fatalf("unexpected executor error: %v", err)
	}
	if result.Next != Failed {
		t.Fatalf("expected Failed, got %s", result.Next)
	}
}

func TestPassthrough_UnknownMethod(t *testing.T) {
	machine := WrapInterface("mock", &mockService{})
	exec := NewExecutor(machine, nil, nil)

	result, err := exec.Run(context.Background(), Event{Type: "NoSuchMethod"})
	if err != nil {
		t.Fatalf("unexpected executor error: %v", err)
	}
	if result.Next != Failed {
		t.Fatalf("expected Failed, got %s", result.Next)
	}
}

// --- Test 2: Multi-step state machine (counter) ---

// counterMachine: counts up from 0 to target, then completes.
// States: "counting" → "counting" → ... → "done"
type counterMachine struct {
	target int
}

type counterState struct {
	Count int `json:"count"`
}

func (m *counterMachine) Name() string { return "counter" }

func (m *counterMachine) InitialState(event Event) State {
	return State{Name: "counting", Data: &counterState{Count: 0}}
}

func (m *counterMachine) Step(ctx context.Context, state *State, event Event) (ActionFunc, error) {
	cs := m.ensureTyped(state)

	return func(ctx context.Context) (*StepResult, error) {
		cs.Count++
		if cs.Count >= m.target {
			return &StepResult{
				NextState: "done",
				Data:      cs,
				Output:    cs.Count,
			}, nil
		}
		return &StepResult{
			NextState: "counting",
			Data:      cs,
			Emit:      []Event{{Type: "counter.tick", Payload: cs.Count}},
		}, nil
	}, nil
}

func (m *counterMachine) ensureTyped(state *State) *counterState {
	if cs, ok := state.Data.(*counterState); ok {
		return cs
	}
	// After checkpoint restore: map → struct
	cs := &counterState{}
	if data, ok := state.Data.(map[string]any); ok {
		if v, ok := data["count"].(float64); ok {
			cs.Count = int(v)
		}
	}
	state.Data = cs
	return cs
}

func TestMultiStep_CounterToTarget(t *testing.T) {
	machine := &counterMachine{target: 5}
	bus := NewMemoryEventBus()
	channel := NewChannel(bus)

	var ticks []int
	bus.Subscribe("counter.tick", func(_ string, payload any) {
		ticks = append(ticks, payload.(int))
	})

	exec := NewExecutor(machine, channel, nil)
	result, err := exec.Run(context.Background(), Event{Type: "start"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Next != Done {
		t.Fatalf("expected Done, got %s", result.Next)
	}
	if result.Output.(int) != 5 {
		t.Errorf("output = %v, want 5", result.Output)
	}
	// Should have emitted 4 ticks (1,2,3,4) — tick 5 is the completion, no emit
	if len(ticks) != 4 {
		t.Errorf("ticks = %v, want 4 events", ticks)
	}
}

// --- Test 3: Suspend and resume via Channel ---

// gatedMachine: runs to a gate, suspends, resumes on "unlock", then completes.
// onSubscribed (optional, test-only) is called once its wait is subscribed
// so the test can deterministically know the executor has reached the gate
// AND registered its subscription — replaces the prior time.Sleep race.
type gatedMachine struct {
	onSubscribed func()
}

type gatedState struct {
	Phase string `json:"phase"`
}

func (m *gatedMachine) Name() string { return "gated" }

func (m *gatedMachine) InitialState(event Event) State {
	return State{Name: "running", Data: &gatedState{Phase: "before_gate"}}
}

func (m *gatedMachine) Step(ctx context.Context, state *State, event Event) (ActionFunc, error) {
	gs := m.ensureTyped(state)

	switch state.Name {
	case "running":
		return func(ctx context.Context) (*StepResult, error) {
			gs.Phase = "at_gate"
			wait := ResumeOnTopic("unlock")
			return &StepResult{
				NextState: "waiting",
				Data:      gs,
				Wait:      wait,
			}, nil
		}, nil

	case "waiting":
		// Resumed by "unlock" event
		return func(ctx context.Context) (*StepResult, error) {
			gs.Phase = "after_gate"
			return &StepResult{
				NextState: "done",
				Data:      gs,
				Output:    "completed after gate",
			}, nil
		}, nil

	default:
		return nil, fmt.Errorf("unknown state %q", state.Name)
	}
}

func (m *gatedMachine) ensureTyped(state *State) *gatedState {
	if gs, ok := state.Data.(*gatedState); ok {
		return gs
	}
	gs := &gatedState{}
	if data, ok := state.Data.(map[string]any); ok {
		if v, ok := data["phase"].(string); ok {
			gs.Phase = v
		}
	}
	state.Data = gs
	return gs
}

func TestSuspendResume_ViaChannel(t *testing.T) {
	// Deterministic synchronisation via the executor's suspend hook — fires AFTER
	// Channel.Subscribe completes the bus subscription for the unlock
	// topic. Replaces the prior time.Sleep(50ms) race (T10 / R10 audit
	// priority-9). Repeated `-race -count=N` runs are now stable.
	subscribed := make(chan struct{}, 1)
	machine := &gatedMachine{
		onSubscribed: func() { subscribed <- struct{}{} },
	}
	bus := NewMemoryEventBus()
	channel := NewChannel(bus)
	store := NewMemoryStateStore()

	exec := NewExecutor(machine, channel, store)
	exec.RegisterHook(onSuspendHook{fn: machine.onSubscribed}) // after its wait is subscribed

	// Run in goroutine — it will block on the gate
	type runResult struct {
		result *RunResult
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		r, err := exec.Run(context.Background(), Event{Type: "start"})
		done <- runResult{r, err}
	}()

	// Wait for the executor to reach the gate and the Channel to register
	// the unlock-topic subscription. The suspend hook fires only
	// after Channel.Subscribe is complete — no time-based assumption.
	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for gatedMachine's wait to be subscribed (the executor never reached the gate)")
	}

	// Publish the unlock event — Channel routes it to the inbox
	_ = bus.Publish("unlock", "go ahead")

	// Wait for completion
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("run error: %v", r.err)
		}
		if r.result.Next != Done {
			t.Fatalf("expected Done, got %s", r.result.Next)
		}
		if r.result.Output != "completed after gate" {
			t.Errorf("output = %v, want %q", r.result.Output, "completed after gate")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for executor to complete")
	}
}

// --- Test 4: Context cancellation ---

func TestContextCancellation(t *testing.T) {
	machine := &counterMachine{target: 1000} // would take many steps
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := NewExecutor(machine, nil, nil)
	result, err := exec.Run(ctx, Event{Type: "start"})
	if err != nil {
		t.Fatalf("unexpected executor error: %v", err)
	}
	if result.Next != Failed {
		t.Fatalf("expected Failed on cancelled context, got %s", result.Next)
	}
}

// --- Test 5: EventBus ---

func TestEventBus_PubSub(t *testing.T) {
	bus := NewMemoryEventBus()

	var received []any
	unsub := bus.Subscribe("test.topic", func(_ string, payload any) {
		received = append(received, payload)
	})

	_ = bus.Publish("test.topic", "msg1")
	_ = bus.Publish("test.topic", "msg2")
	_ = bus.Publish("other.topic", "ignored")

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	unsub()
	_ = bus.Publish("test.topic", "msg3")

	if len(received) != 2 {
		t.Errorf("expected 2 events after unsub, got %d", len(received))
	}
}

// --- Test 6: StateStore ---

// --- Test 7: Channel Dead Event Queue ---

// ────────────────────────────────────────────────────────────────────────
// DedupTracker + EventIDGenerator wiring tests — UC-110
// (kts-task-engine.md execution rule 10: every persistent event has a
// unique ID; executors track processed IDs in checkpoint; duplicate
// delivery → skip.)
// ────────────────────────────────────────────────────────────────────────

// eventTypes is a tiny helper for readable test failure output.
func eventTypes(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func TestStateStore_SaveLoadDelete(t *testing.T) {
	store := NewMemoryStateStore()

	cp := Checkpoint{
		State: State{Name: "test", RunID: "run-1", Version: 3},
		Event: Event{Type: "tick"},
	}

	if err := store.Save("run-1", cp); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load("run-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.State.Name != "test" || loaded.State.Version != 3 {
		t.Errorf("loaded state = %+v", loaded.State)
	}

	_ = store.Delete("run-1")
	if _, err := store.Load("run-1"); err == nil {
		t.Error("loaded after delete; want it gone")
	}
}
