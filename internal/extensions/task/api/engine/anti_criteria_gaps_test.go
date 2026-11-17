// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// Anti-criteria probes — the named failure modes from the per-UC files
// (modules/tasks/use-cases/, cm0e). One test per anti-criterion; each
// asserts the NEGATIVE: what the engine must NOT do.

// ── UC-001-AC2 — Run must not spin forever ────────────────────────────

// spinnerMachine never returns Output and never waits: the pathological
// non-terminating machine.
type spinnerMachine struct{}

func (m *spinnerMachine) Name() string { return "spinner" }
func (m *spinnerMachine) InitialState(Event) State {
	return State{Name: "spinning", Data: map[string]any{}}
}
func (m *spinnerMachine) Step(ctx context.Context, state *State, event Event) (ActionFunc, error) {
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "spinning", Data: state.Data}, nil
	}, nil
}

func TestRun_DeadlineStopsNonTerminatingMachine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	exec := NewExecutor(&spinnerMachine{}, nil, nil)
	done := make(chan struct{})
	var result *RunResult
	var runErr error
	go func() {
		result, runErr = exec.Run(ctx, Event{Type: "start"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not terminate after ctx deadline — non-terminating machine spins forever")
	}
	if runErr != nil {
		t.Fatalf("Run returned hard error %v; want Failed result", runErr)
	}
	if result.Next != Failed {
		t.Fatalf("Next = %s, want Failed", result.Next)
	}
	if !errors.Is(result.Error, context.DeadlineExceeded) {
		t.Fatalf("Error = %v, want context.DeadlineExceeded", result.Error)
	}
}

// ── UC-002-AC3 — Resume must not silently restart on missing checkpoint ──

// trackingMachine counts Step invocations so tests can assert the machine
// was never driven.
type trackingMachine struct {
	mu    sync.Mutex
	steps int
}

func (m *trackingMachine) Name() string { return "tracking" }
func (m *trackingMachine) InitialState(Event) State {
	return State{Name: "running", Data: map[string]any{}}
}
func (m *trackingMachine) Step(ctx context.Context, state *State, event Event) (ActionFunc, error) {
	m.mu.Lock()
	m.steps++
	m.mu.Unlock()
	return func(context.Context) (*StepResult, error) {
		return &StepResult{NextState: "done", Data: state.Data, Output: "ran"}, nil
	}, nil
}
func (m *trackingMachine) stepCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.steps
}

func TestResume_MissingCheckpointErrors(t *testing.T) {
	machine := &trackingMachine{}
	exec := NewExecutor(machine, nil, NewMemoryStateStore()) // empty store

	result, err := exec.ResumeWaiting(context.Background(), "ghost-run")
	if err == nil {
		t.Fatalf("ResumeWaiting of a missing checkpoint must error; got result %+v", result)
	}
	if !strings.Contains(err.Error(), "ghost-run") {
		t.Errorf("error %q does not name the missing runID", err)
	}
	if machine.stepCount() != 0 {
		t.Fatalf("machine was stepped %d times — Resume silently restarted the run", machine.stepCount())
	}
}

// ── UC-003-AC2 / UC-013-AC2 — wrong events must not wake suspended runs ──

// topicGatedMachine suspends on a configurable topic (the gatedMachine
// pattern, parameterized so two runs can wait on distinct gates).
type topicGatedMachine struct {
	topic        string
	onSubscribed func()
}

func (m *topicGatedMachine) Name() string { return "topic-gated" }
func (m *topicGatedMachine) InitialState(Event) State {
	return State{Name: "running", Data: map[string]any{}}
}
func (m *topicGatedMachine) Step(ctx context.Context, state *State, event Event) (ActionFunc, error) {
	switch state.Name {
	case "running":
		return func(context.Context) (*StepResult, error) {
			wait := ResumeOnTopic(m.topic)
			return &StepResult{NextState: "waiting", Data: state.Data, Wait: wait}, nil
		}, nil
	case "waiting":
		return func(context.Context) (*StepResult, error) {
			return &StepResult{NextState: "done", Data: state.Data, Output: "woke:" + event.Type}, nil
		}, nil
	default:
		return nil, fmt.Errorf("unknown state %q", state.Name)
	}
}

// startGated runs a topic-gated machine on the shared channel and returns
// its completion channel. Blocks until the WaitSpec subscription is live.
func startGated(t *testing.T, channel *Channel, topic string) <-chan *RunResult {
	t.Helper()
	subscribed := make(chan struct{}, 1)
	machine := &topicGatedMachine{topic: topic, onSubscribed: func() { subscribed <- struct{}{} }}
	exec := NewExecutor(machine, channel, NewMemoryStateStore())
	exec.RegisterHook(onSuspendHook{fn: machine.onSubscribed}) // after its wait is subscribed
	done := make(chan *RunResult, 1)
	go func() {
		r, err := exec.Run(context.Background(), Event{Type: "start"})
		if err != nil {
			t.Errorf("run error: %v", err)
		}
		done <- r
	}()
	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		t.Fatalf("executor never reached the %s gate", topic)
	}
	return done
}

func TestSuspend_NonMatchingEventDoesNotWake(t *testing.T) {
	bus := NewMemoryEventBus()
	channel := NewChannel(bus)
	done := startGated(t, channel, "uc003.expected")

	// The anti-assertion: a non-matching topic must NOT wake the run.
	_ = bus.Publish("uc003.other", "noise")
	select {
	case r := <-done:
		t.Fatalf("non-matching event woke the suspended run: %+v", r)
	case <-time.After(150 * time.Millisecond):
		// still suspended — correct
	}

	_ = bus.Publish("uc003.expected", "go")
	select {
	case r := <-done:
		if r.Next != Done {
			t.Fatalf("Next = %s, want Done", r.Next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matching event did not wake the run")
	}
}

func TestDispatch_WrongRunGateDoesNotResume(t *testing.T) {
	bus := NewMemoryEventBus()
	channel := NewChannel(bus) // SHARED channel — the isolation under test
	doneA := startGated(t, channel, "uc013.run-a.done")
	doneB := startGated(t, channel, "uc013.run-b.done")

	// Waking A must not leak into B.
	_ = bus.Publish("uc013.run-a.done", "for A")
	select {
	case r := <-doneA:
		if r.Next != Done {
			t.Fatalf("run A: Next = %s, want Done", r.Next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run A did not resume on its own gate")
	}
	select {
	case r := <-doneB:
		t.Fatalf("run B resumed on run A's event (cross-run mis-delivery): %+v", r)
	case <-time.After(150 * time.Millisecond):
		// B still suspended — correct
	}

	_ = bus.Publish("uc013.run-b.done", "for B")
	select {
	case r := <-doneB:
		if r.Next != Done {
			t.Fatalf("run B: Next = %s, want Done", r.Next)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run B did not resume on its own gate")
	}
}

// ── UC-005-AC2 — double-unsubscribe must be safe ─────────────────────

func TestUnsub_DoubleCallSafe(t *testing.T) {
	bus := NewMemoryEventBus()
	var aHits, bHits int
	unsubA := bus.Subscribe("uc005.topic", func(string, any) { aHits++ })
	_ = bus.Subscribe("uc005.topic", func(string, any) { bHits++ })

	unsubA()
	unsubA() // the anti-assertion: second call must neither panic...

	_ = bus.Publish("uc005.topic", "x")
	if aHits != 0 {
		t.Errorf("A fired after unsub: %d", aHits)
	}
	if bHits != 1 { // ...nor remove any other subscriber.
		t.Errorf("double-unsub disturbed sibling subscriber: bHits = %d, want 1", bHits)
	}
}

// ── UC-009-AC2 — no event published before the StateStore commit ─────

// orderRecordingStore logs every Save into a shared ordered log.
type orderRecordingStore struct {
	inner StateStore
	log   *orderedLog
}

type orderedLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *orderedLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
}
func (l *orderedLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func (s *orderRecordingStore) Save(runID string, cp Checkpoint) error {
	s.log.add("save")
	return s.inner.Save(runID, cp)
}
func (s *orderRecordingStore) Load(runID string) (Checkpoint, error) { return s.inner.Load(runID) }
func (s *orderRecordingStore) Delete(runID string) error             { return s.inner.Delete(runID) }

func TestCommitOrder_NoEmitBeforeSave(t *testing.T) {
	log := &orderedLog{}
	store := &orderRecordingStore{inner: NewMemoryStateStore(), log: log}
	bus := NewMemoryEventBus() // synchronous delivery — publish order IS log order
	bus.Subscribe("counter.tick", func(string, any) { log.add("emit") })
	channel := NewChannel(bus)

	exec := NewExecutor(&counterMachine{target: 4}, channel, store)
	if _, err := exec.Run(context.Background(), Event{Type: "start"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries := log.snapshot()
	saves, emits := 0, 0
	for i, e := range entries {
		switch e {
		case "save":
			saves++
		case "emit":
			emits++
			if emits > saves {
				t.Fatalf("event published BEFORE its StateStore commit at log[%d]: %v", i, entries)
			}
		}
	}
	if emits == 0 || saves == 0 {
		t.Fatalf("probe inert (saves=%d emits=%d) — counterMachine shape changed?", saves, emits)
	}
}

// ── UC-019-AC3 — subject builders pinned (token-drift guard) ─────────

// TestSubjects_TokenLayoutPin pins the exact wire strings of the core
// subject constructors. Any builder change that shifts token offsets
// breaks this test BEFORE it silently breaks a parser somewhere else
// (the pin-every-wire-format-parser-to-its-builder rule).
func TestSubjects_TokenLayoutPin(t *testing.T) {
	pins := []struct {
		name string
		got  string
		want string
	}{
		{"tenant-event", subjects.TenantEventSubject("t1", "boom"), "ktw.t1.event.boom"},
		{"universe-event", subjects.UniverseEventSubject("t1", "u1", "boom"), "ktw.t1.universe.u1.event.boom"},
		{"session-event", subjects.SessionEventSubject("t1", "u1", "s1", "boom"), "ktw.t1.universe.u1.session.s1.event.boom"},
		{"agent-message", subjects.SessionAgentMessageSubject("t1", "u1", "s1", "ada"), "ktw.t1.universe.u1.session.s1.to.ada"},
		{"task-lifecycle", subjects.TaskLifecycleSubject("t1", "u1", "s1", "tk1", "started"), "ktw.t1.universe.u1.session.s1.task.tk1.started"},
	}
	for _, p := range pins {
		if p.got != p.want {
			t.Errorf("%s drifted:\n  got  %s\n  want %s", p.name, p.got, p.want)
		}
		if strings.Count(p.got, ".") != strings.Count(p.want, ".") {
			t.Errorf("%s token COUNT drifted (parsers index by token offset)", p.name)
		}
	}
}

// onSuspendHook calls fn when the run suspends — after its wait's
// subscriptions are in place (the executor subscribes, then tells).
type onSuspendHook struct {
	NoopHook
	fn func()
}

func (h onSuspendHook) OnSuspend(context.Context, string, *WaitSpec) {
	if h.fn != nil {
		h.fn()
	}
}
