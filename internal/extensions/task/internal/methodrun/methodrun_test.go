// SPDX-License-Identifier: Apache-2.0

package methodrun

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// fakeActions runs one method: it records the call and answers run's result.
type fakeActions struct {
	mu   sync.Mutex
	got  []actionsapi.RunInput
	run  func(ctx context.Context) (json.RawMessage, error)
	seen chan struct{}
}

func (f *fakeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (f *fakeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	f.mu.Lock()
	f.got = append(f.got, in)
	f.mu.Unlock()
	if f.seen != nil {
		close(f.seen)
	}
	v, err := f.run(ctx)
	return actionsapi.RunOutput{URI: in.URI, Value: v}, err
}

// rig wires the seams to a stub bus and fake actions, and collects the
// panel's events.
func rig(t *testing.T, acts *fakeActions) (*teststub.Stub, func() []panel.Event) {
	t.Helper()
	bus := teststub.New()
	pa, pb, ps := resolveActionService, resolveBus, resolveScope
	resolveActionService = func(context.Context) (actionsapi.ActionService, error) { return acts, nil }
	resolveBus = func(context.Context) (mbapi.MessageBus, error) { return bus, nil }
	resolveScope = func(context.Context) panel.Scope { return panel.Scope{Tenant: "t", Universe: "u"} }
	t.Cleanup(func() { resolveActionService, resolveBus, resolveScope = pa, pb, ps })
	var mu sync.Mutex
	var evs []panel.Event
	if _, err := bus.Subscribe(context.Background(), "ktw.t.universe.u.panel.*.out", func(_ context.Context, m mbapi.Message) {
		if e, err := panel.DecodeEvent(m.Data()); err == nil {
			mu.Lock()
			evs = append(evs, e)
			mu.Unlock()
		}
	}); err != nil {
		t.Fatal(err)
	}
	return bus, func() []panel.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]panel.Event(nil), evs...)
	}
}

func runIt(t *testing.T, ctx context.Context, cfg map[string]any, input map[string]any) (any, error) {
	t.Helper()
	m, err := (&factory{}).CreateMachine(ctx, &taskapi.TaskExecRequest{Config: cfg, Input: input})
	if err != nil {
		t.Fatal(err)
	}
	st := m.InitialState(engine.Event{})
	act, err := m.Step(ctx, &st, engine.Event{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := act(engine.WithRunID(ctx, "run-1"))
	if err != nil {
		return nil, err
	}
	return res.Output, nil
}

// waitEvents waits for n events of the panel (the stub bus delivers apart).
func waitEvents(t *testing.T, events func() []panel.Event, n int) []panel.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if evs := events(); len(evs) >= n {
			return evs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("got %d panel events, want %d", len(events()), n)
	return nil
}

func kinds(evs []panel.Event) map[panel.Kind]panel.Event {
	out := map[panel.Kind]panel.Event{}
	for _, e := range evs {
		out[e.Kind] = e
	}
	return out
}

// A method run opens its panel, calls the method with the panel's id in the
// call's context, completes the panel with the method's value and answers
// {panel, value}.
func TestAMethodRunCallsTheMethodUnderItsPanel(t *testing.T) {
	acts := &fakeActions{run: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"pages":3}`), nil }}
	_, events := rig(t, acts)
	id := panel.NewID()
	out, err := runIt(t, context.Background(), taskapi.MethodRunDef("ktw-example:example/push", id, "Publication", "")["config"].(map[string]any), map[string]any{"dossier": "billing"})
	if err != nil {
		t.Fatal(err)
	}
	o, ok := out.(Output)
	if !ok || o.Panel != id || string(o.Value) != `{"pages":3}` {
		t.Fatalf("output %#v", out)
	}
	if len(acts.got) != 1 || acts.got[0].URI != "ktw-example:example/push" || acts.got[0].Context["panel"] != id || string(acts.got[0].Input) != `{"dossier":"billing"}` {
		t.Fatalf("the method was called with %+v", acts.got)
	}
	k := kinds(waitEvents(t, events, 2))
	if op := k[panel.KindOpen]; op.Open == nil || op.Open.Type != taskapi.ProgressPanelType || op.Open.Title != "Publication" || op.Open.Owner != "run-1" || op.Panel != id {
		t.Fatalf("open %+v", op)
	}
	if c := k[panel.KindComplete]; c.Complete == nil || c.Complete.Status != StatusOK {
		t.Fatalf("complete %+v", c)
	}
}

// A method that fails completes its panel failed, and the run fails.
func TestAFailedMethodCompletesItsPanelFailed(t *testing.T) {
	acts := &fakeActions{run: func(context.Context) (json.RawMessage, error) { return nil, errors.New("space not found") }}
	_, events := rig(t, acts)
	if _, err := runIt(t, context.Background(), map[string]any{"uri": "ktw-example:example/push"}, nil); err == nil {
		t.Fatal("a failed method gave a run that succeeded")
	}
	if c := kinds(waitEvents(t, events, 2))[panel.KindComplete]; c.Complete == nil || c.Complete.Status != StatusFailed {
		t.Fatalf("complete %+v", c)
	}
}

// The person's « cancel » on the panel ends the method's context; the panel
// completes cancelled - and so does a stop of the run (its context ended).
func TestCancelOnThePanelStopsTheMethod(t *testing.T) {
	seen := make(chan struct{})
	acts := &fakeActions{seen: seen, run: func(ctx context.Context) (json.RawMessage, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return json.RawMessage(`"never cancelled"`), nil
		}
	}}
	bus, events := rig(t, acts)
	id := panel.NewID()
	done := make(chan error, 1)
	go func() {
		_, err := runIt(t, context.Background(), map[string]any{"uri": "ktw-sources:sources/sync", "panel": id}, nil)
		done <- err
	}()
	<-seen
	raw, _ := json.Marshal(panel.Input{Panel: id, Kind: panel.InputCancel})
	if err := bus.Publish(context.Background(), subjects.PanelInSubject("t", "u", id), raw); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled run succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the panel's cancel never reached the method")
	}
	if c := kinds(waitEvents(t, events, 2))[panel.KindComplete]; c.Complete == nil || c.Complete.Status != StatusCancelled {
		t.Fatalf("complete %+v", c)
	}
}

func TestAMethodRunNeedsAMethodURI(t *testing.T) {
	for name, cfg := range map[string]map[string]any{
		"no uri":       {},
		"a task":       {"uri": "ktw-agents:tasks/refine"},
		"bad panel id": {"uri": "ktw-sources:sources/sync", "panel": "p1"},
	} {
		if _, err := (&factory{}).CreateMachine(context.Background(), &taskapi.TaskExecRequest{Config: cfg}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// : a method run started on an object names it on its panel's
// open - the object a kept outcome lands on; none, no object.
func TestAMethodRunOnAnObjectNamesItOnItsPanel(t *testing.T) {
	acts := &fakeActions{run: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{}`), nil }}
	_, events := rig(t, acts)
	id := panel.NewID()
	if _, err := runIt(t, context.Background(), taskapi.MethodRunDef("ktw-example:example/push", id, "Publication", "m.D-001")["config"].(map[string]any), nil); err != nil {
		t.Fatal(err)
	}
	if op := kinds(waitEvents(t, events, 2))[panel.KindOpen]; op.Open == nil || op.Open.Object != "m.D-001" {
		t.Fatalf("the panel opens on the run's object: %+v", op.Open)
	}
	if _, has := taskapi.MethodRunDef("ktw-example:example/push", id, "", "")["config"].(map[string]any)["object"]; has {
		t.Error("a run on no object names none")
	}
}
