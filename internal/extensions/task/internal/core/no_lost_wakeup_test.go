// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// NO LOST WAKE-UP — a waiting run hears its event wherever it runs and
// whenever the event comes.

// extWaiterMachine waits on another process's event (an EventURI), then ends.
type extWaiterMachine struct{}

const extWaiterURI = "cwm:!workspace/object/req-x/updated"

func (extWaiterMachine) Name() string                           { return "extwaiter" }
func (extWaiterMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (extWaiterMachine) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	if st.Name == "init" {
		return func(context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{NextState: "waiting", Wait: &engine.WaitSpec{Mode: engine.WaitAny,
				Conditions: []engine.WaitCondition{{Topic: engine.ExternalEventTopic(extWaiterURI), EventURI: extWaiterURI}}}}, nil
		}, nil
	}
	return func(context.Context) (*engine.StepResult, error) {
		return &engine.StepResult{NextState: "done", Output: map[string]any{"woke": ev.Type}}, nil
	}, nil
}

type extWaiterFactory struct{}

func (extWaiterFactory) Name() string { return "extwaiter" }
func (extWaiterFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return extWaiterMachine{}, nil
}

// useEvents makes every run's event bridge reach evs.
func useEvents(t *testing.T, evs eventsapi.EventService) {
	t.Helper()
	old := eventServiceOf
	eventServiceOf = func(context.Context) (eventsapi.EventService, error) { return evs, nil }
	t.Cleanup(func() { eventServiceOf = old })
}

// A workflow's child waiting on another process's event is woken by it, as a
// top-level run is: every executor of this package bridges external events.
func TestAWorkflowChildHearsAnExternalEvent(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "extwaiter"}, Impl: extWaiterFactory{}})
	evs := &bridgeEvents{}
	useEvents(t, evs)
	bus := engine.NewMemoryEventBus()
	spawner := NewLocalSpawner(engine.NewChannel(bus))
	done := make(chan struct{}, 1)
	bus.Subscribe(engine.ChildDoneTopic("asker"), func(string, any) {
		select {
		case done <- struct{}{}:
		default:
		}
	})

	if _, err := spawner.Spawn(context.Background(), engine.ChildSpec{Kind: "extwaiter", Name: "asker", ParentID: "run-parent"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child's external event subscribed", func() bool {
		evs.mu.Lock()
		defer evs.mu.Unlock()
		return len(evs.uris) == 1 && evs.uris[0] == extWaiterURI
	})
	evs.mu.Lock()
	h := evs.h
	evs.mu.Unlock()
	h(context.Background(), json.RawMessage(`{"id":"req-x"}`))
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the workflow's child never heard its external event")
	}
}

// askerMachine models ask-user on the inbox: its first step files a question
// that is answered AT ONCE — before the step has even returned its wait — and
// waits for the answer's event, rechecking the request when it resumes. A
// recheck that finds no answer waits again.
type askerMachine struct {
	bus      engine.EventBus
	answered *atomic.Bool
	checks   *atomic.Int64
}

func (askerMachine) Name() string                           { return "asker" }
func (askerMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (m askerMachine) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	wait := &engine.WaitSpec{Mode: engine.WaitAny, RecheckOnResume: true,
		Conditions: []engine.WaitCondition{{Topic: "answered." + st.RunID}}}
	if st.Name == "init" {
		return func(context.Context) (*engine.StepResult, error) {
			m.answered.Store(true)
			_ = m.bus.Publish("answered."+st.RunID, map[string]any{}) // nobody listens yet
			return &engine.StepResult{NextState: "waiting", Wait: wait}, nil
		}, nil
	}
	return func(context.Context) (*engine.StepResult, error) {
		if ev.Type == engine.TickEvent {
			m.checks.Add(1)
			if !m.answered.Load() {
				return &engine.StepResult{NextState: "waiting", Wait: wait}, nil
			}
		}
		return &engine.StepResult{NextState: "done", Output: map[string]any{"woke": ev.Type}}, nil
	}, nil
}

var askerState struct {
	bus      engine.EventBus
	answered atomic.Bool
	checks   atomic.Int64
}

type askerFactory struct{}

func (askerFactory) Name() string { return "asker" }
func (askerFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return askerMachine{bus: askerState.bus, answered: &askerState.answered, checks: &askerState.checks}, nil
}

// An answer that comes while the step that waits for it is still committing —
// before the wait is subscribed — is not lost: a wait that rechecks is
// rechecked once as soon as it is subscribed, live as after a restart.
func TestAnAnswerBeforeTheWaitIsSubscribedIsHeard(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	askerState.bus = svc.channel.Bus()
	askerState.answered.Store(false)
	askerState.checks.Store(0)
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "asker"}, Impl: askerFactory{}})
	svc.tasks["asker"] = &taskapi.Task{Name: "asker", Provider: "test", Type: "asker"}
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "asker"})
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID})
		t.Fatalf("result = %+v, %v; the answer given before the wait was subscribed was lost", res, err)
	}
	if n := askerState.checks.Load(); n != 1 {
		t.Errorf("rechecks = %d, want exactly one", n)
	}
}

// A recheck that finds nothing waits again — without rechecking again (no busy
// loop) — and the event that then comes is heard.
func TestARecheckThatFindsNothingWaitsOnce(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "checker"}, Impl: checkerFactory{}})
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.tasks["checker"] = &taskapi.Task{Name: "checker", Provider: "test", Type: "checker"}
	checkerTicks.Store(0)
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "checker"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "rechecked and waiting", func() bool {
		return checkerTicks.Load() == 1 && status(t, svc, r.RunID) == taskapi.RunStatusWaiting
	})
	time.Sleep(50 * time.Millisecond)
	if n := checkerTicks.Load(); n != 1 {
		t.Fatalf("rechecks = %d, want one", n)
	}
	_ = svc.channel.Bus().Publish("never", map[string]any{})
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("result = %+v, %v; want completed on its event", res, err)
	}
}

// eagerEvents is an EventService whose event comes the instant it is
// subscribed — the earliest an event of another process can arrive.
type eagerEvents struct{ eventsapi.EventService }

func (eagerEvents) Subscribe(ctx context.Context, _, _ string, h eventsapi.Handler) (eventsapi.Subscription, error) {
	h(ctx, json.RawMessage(`{"id":"req-x"}`))
	return bridgeSub{}, nil
}

// The run listens before its external events are bridged: an event that comes
// the instant the bridge subscribes is heard, even by a wait that does not
// recheck.
func TestAnEventAtTheInstantOfSubscribingIsHeard(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "extwaiter"}, Impl: extWaiterFactory{}})
	useEvents(t, eagerEvents{})
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.tasks["extwaiter"] = &taskapi.Task{Name: "extwaiter", Provider: "test", Type: "extwaiter"}
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "extwaiter"})
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID})
		t.Fatalf("result = %+v, %v; the event that came as the bridge subscribed was lost", res, err)
	}
}
