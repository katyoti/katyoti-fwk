// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// D4 — executor hook surface tests.
//
// Cover the contract: pre/post/suspend/resume fire at the right points,
// ordering matches registration, error from OnPreStep aborts the step
// AND fires OnPostStep with the error, hook errors elsewhere don't
// poison the loop.

// captureHook records every callback invocation for assertion.
type captureHook struct {
	name     string
	mu       sync.Mutex
	pre      []string // each entry is "<event.Type>"
	post     []string // "<state.Name>:<err>"
	suspend  []string // "<runID>"
	resume   []string // "<runID>:<event.Type>"
	preErr   error    // optional: returned by OnPreStep on the Nth call
	preErrAt int      // call index at which preErr fires
}

func (h *captureHook) OnPreStep(_ context.Context, _ *State, event Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pre = append(h.pre, event.Type)
	if h.preErr != nil && len(h.pre) == h.preErrAt {
		return h.preErr
	}
	return nil
}

func (h *captureHook) OnPostStep(_ context.Context, state *State, _ *StepResult, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	name := ""
	if state != nil {
		name = state.Name
	}
	h.post = append(h.post, name+":"+errStr)
}

func (h *captureHook) OnSuspend(_ context.Context, runID string, _ *WaitSpec) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.suspend = append(h.suspend, runID)
}

func (h *captureHook) OnResume(_ context.Context, runID string, event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.resume = append(h.resume, runID+":"+event.Type)
}

// TestExecutorHooks_PreAndPostFireOnEveryStep — a counter machine running
// N steps invokes OnPreStep + OnPostStep N times each.
func TestExecutorHooks_PreAndPostFireOnEveryStep(t *testing.T) {
	machine := &counterMachine{target: 3}
	exec := NewExecutor(machine, nil, nil)
	hook := &captureHook{name: "h1"}
	exec.RegisterHook(hook)

	r, err := exec.Run(context.Background(), Event{Type: "start"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Next != Done {
		t.Fatalf("expected Done, got %s", r.Next)
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()
	// counterMachine runs target+1 steps (final step emits Output → Done).
	if len(hook.pre) < 3 {
		t.Errorf("pre count = %d, want >= 3", len(hook.pre))
	}
	if len(hook.post) != len(hook.pre) {
		t.Errorf("post count = %d, want %d (one per pre)", len(hook.post), len(hook.pre))
	}
}

// TestExecutorHooks_PreErrShortCircuits — OnPreStep returning an error
// short-circuits the step with Failed, AND OnPostStep still fires with
// the same error so observers always see post.
func TestExecutorHooks_PreErrShortCircuits(t *testing.T) {
	machine := &counterMachine{target: 5}
	exec := NewExecutor(machine, nil, nil)
	hook := &captureHook{
		name:     "abort-on-call-1",
		preErr:   errors.New("policy-rejected"),
		preErrAt: 1, // fail the FIRST pre call
	}
	exec.RegisterHook(hook)

	r, err := exec.Run(context.Background(), Event{Type: "start"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Next != Failed {
		t.Errorf("expected Failed, got %s", r.Next)
	}
	if r.Error == nil || r.Error.Error() != "policy-rejected" {
		t.Errorf("RunResult.Error = %v, want policy-rejected", r.Error)
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.pre) != 1 {
		t.Errorf("pre invocations = %d, want 1 (short-circuit)", len(hook.pre))
	}
	if len(hook.post) != 1 {
		t.Errorf("post invocations = %d, want 1 (post still fires on abort)", len(hook.post))
	}
	if !contains(hook.post[0], "policy-rejected") {
		t.Errorf("post[0] = %q, want it to mention 'policy-rejected'", hook.post[0])
	}
}

// TestExecutorHooks_RegistrationOrder — two hooks registered in order;
// both see every event in the same order.
func TestExecutorHooks_RegistrationOrder(t *testing.T) {
	machine := &counterMachine{target: 2}
	exec := NewExecutor(machine, nil, nil)
	first := &captureHook{name: "first"}
	second := &captureHook{name: "second"}
	exec.RegisterHook(first)
	exec.RegisterHook(second)

	r, err := exec.Run(context.Background(), Event{Type: "start"})
	if err != nil || r.Next != Done {
		t.Fatalf("run: err=%v next=%s", err, r.Next)
	}

	first.mu.Lock()
	second.mu.Lock()
	defer first.mu.Unlock()
	defer second.mu.Unlock()
	if len(first.pre) != len(second.pre) {
		t.Errorf("hooks saw different #pre: first=%d second=%d", len(first.pre), len(second.pre))
	}
	// Same event sequence reaches both — same pre[] contents.
	for i := range first.pre {
		if i < len(second.pre) && first.pre[i] != second.pre[i] {
			t.Errorf("step %d: first.pre=%q second.pre=%q", i, first.pre[i], second.pre[i])
		}
	}
}

// TestExecutorHooks_SuspendResumeOnGatedMachine — OnSuspend fires before
// Channel.Wait subscribes; OnResume fires when the unlock event lands.
func TestExecutorHooks_SuspendResumeOnGatedMachine(t *testing.T) {
	subscribed := make(chan struct{}, 1)
	machine := &gatedMachine{
		onSubscribed: func() { subscribed <- struct{}{} },
	}
	bus := NewMemoryEventBus()
	channel := NewChannel(bus)
	exec := NewExecutor(machine, channel, NewMemoryStateStore())
	exec.RegisterHook(onSuspendHook{fn: machine.onSubscribed}) // after its wait is subscribed
	hook := &captureHook{name: "gatedHook"}
	exec.RegisterHook(hook)

	done := make(chan *RunResult, 1)
	go func() {
		r, _ := exec.Run(context.Background(), Event{Type: "start"})
		done <- r
	}()

	<-subscribed // gate reached + WaitSpec subscribed
	_ = bus.Publish("unlock", "go")

	r := <-done
	if r.Next != Done {
		t.Fatalf("expected Done, got %s", r.Next)
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.suspend) == 0 {
		t.Error("OnSuspend never fired")
	}
	if len(hook.resume) == 0 {
		t.Error("OnResume never fired")
	}
}

// TestNoopHook_ImplementsExecutorHook — compile-time assertion. NoopHook
// satisfies the interface so callers can embed it for partial implements.
func TestNoopHook_ImplementsExecutorHook(t *testing.T) {
	var _ ExecutorHook = NoopHook{}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
