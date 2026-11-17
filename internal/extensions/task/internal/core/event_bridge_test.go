// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// bridgeEvents is an EventService that keeps each subscription's handler and
// context.
type bridgeEvents struct {
	eventsapi.EventService
	mu   sync.Mutex
	uris []string
	h    eventsapi.Handler
	ctx  context.Context
}

type bridgeSub struct{}

func (bridgeSub) Unsubscribe() error { return nil }

func (b *bridgeEvents) Subscribe(ctx context.Context, uri, _ string, h eventsapi.Handler) (eventsapi.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.uris, b.h, b.ctx = append(b.uris, uri), h, ctx
	return bridgeSub{}, nil
}

// While a run waits on another process's event, that event is forwarded onto
// the in-process topic it waits on; when it resumes, the subscription ends. A
// condition with no external event subscribes nothing.
func TestAnExternalEventReachesTheWaitingRun(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	evs := &bridgeEvents{}
	h := newEventBridgeHook(bus, nil)
	h.events = func(context.Context) (eventsapi.EventService, error) { return evs, nil }

	uri := "cwm:!workspace/object/req-1/updated"
	got := make(chan any, 1)
	bus.Subscribe(engine.ExternalEventTopic(uri), func(_ string, p any) { got <- p })

	h.OnSuspend(context.Background(), "run-1", &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{
		{Topic: "local.only"},
		{Topic: engine.ExternalEventTopic(uri), EventURI: uri},
	}})
	if len(evs.uris) != 1 || evs.uris[0] != uri {
		t.Fatalf("subscribed %v, want the external event only", evs.uris)
	}
	evs.h(context.Background(), json.RawMessage(`{"id":"req-1"}`))
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("the external event did not reach the in-process topic")
	}

	// Kept across a resume: the run that waits again on the same event
	// (a recheck that found no answer) loses nothing in between.
	h.OnResume(context.Background(), "run-1", engine.Event{})
	first := evs.ctx
	h.OnSuspend(context.Background(), "run-1", &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{
		{Topic: engine.ExternalEventTopic(uri), EventURI: uri},
	}})
	if first.Err() != nil || len(evs.uris) != 1 {
		t.Errorf("the same wait again re-subscribed (%d subscriptions, first ended: %v); want it kept", len(evs.uris), first.Err())
	}

	// Another wait replaces it.
	other := "cwm:!workspace/object/req-2/updated"
	h.OnSuspend(context.Background(), "run-1", &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{
		{Topic: engine.ExternalEventTopic(other), EventURI: other},
	}})
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Error("the previous wait's subscription outlives the next wait")
	}
	if len(evs.uris) != 2 || evs.uris[1] != other {
		t.Errorf("subscribed %v, want the new event", evs.uris)
	}
}

// checkerMachine waits with RecheckOnResume on the topic "never": a recheck (a
// tick) finds nothing and waits again; the event ends it. checkerTicks counts
// its rechecks.
type checkerMachine struct{}

var checkerTicks atomic.Int64

func (checkerMachine) Name() string                           { return "checker" }
func (checkerMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (checkerMachine) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	wait := &engine.WaitSpec{Conditions: []engine.WaitCondition{{Topic: "never"}}, Mode: engine.WaitAny, RecheckOnResume: true}
	if st.Name == "init" {
		return func(context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{NextState: "waiting", Wait: wait}, nil
		}, nil
	}
	return func(context.Context) (*engine.StepResult, error) {
		if ev.Type == engine.TickEvent {
			checkerTicks.Add(1)
			return &engine.StepResult{NextState: "waiting", Wait: wait}, nil
		}
		return &engine.StepResult{NextState: "done", Output: map[string]any{"woke": ev.Type}}, nil
	}, nil
}

type checkerFactory struct{}

func (checkerFactory) Name() string { return "checker" }
func (checkerFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return checkerMachine{}, nil
}

// A wait marked RecheckOnResume is rechecked after a restart — its event may
// have come while no process listened: the machine steps once with a tick and
// checks for itself, then waits again; its event then ends it.
func TestARecheckedWaitStepsAtOnceAfterARestart(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "checker"}, Impl: checkerFactory{}})
	dir := scratchDir(t)
	first := durableService(t, dir)
	first.tasks["checker"] = &taskapi.Task{Name: "checker", Provider: "test", Type: "checker"}
	ctx := context.Background()
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "checker", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	second.tasks["checker"] = first.tasks["checker"]
	ticks := checkerTicks.Load()
	if rep := second.RestoreRuns(ctx); len(rep.Resumed) != 1 {
		t.Fatalf("restore = %+v, want it resumed", rep)
	}
	eventually(t, "rechecked after the restart, waiting again", func() bool {
		return checkerTicks.Load() == ticks+1 && status(t, second, r.RunID) == taskapi.RunStatusWaiting
	})
	_ = second.channel.Bus().Publish("never", map[string]any{})
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("result = %+v, %v; want completed on its event", res, err)
	}
	if out, _ := res.Output.(map[string]any); out["woke"] != "never" {
		t.Errorf("woke by %v, want its event", res.Output)
	}
	_ = first.channel.Bus().Publish("never", map[string]any{})
}
