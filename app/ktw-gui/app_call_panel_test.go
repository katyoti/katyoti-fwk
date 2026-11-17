// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// installSender stands for the admin universe's ktw-univers: it reads the
// panel the call was lent, reports one step into it as the method's
// progress.Emit does on the serving side (panel.Resume), then answers - or
// fails, or waits until the call is given up.
type installSender struct {
	bus     mbapi.MessageBus
	answer  json.RawMessage
	fail    error
	block   bool
	mu      sync.Mutex
	lent    *panel.Handle
	started chan struct{}
}

func (s *installSender) Call(ctx context.Context, _ string, _ any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	h, ok := panel.HandleFrom(ctx)
	s.mu.Lock()
	if ok {
		s.lent = h
	}
	s.mu.Unlock()
	if ok {
		served, err := panel.Resume(h.Ref(), func(ctx context.Context, subject string, b []byte) error { return s.bus.Publish(ctx, subject, b) })
		if err != nil {
			return nil, err
		}
		if err := served.Progress(ctx, "download ktw-acme", 1, 2); err != nil {
			return nil, err
		}
		// The answer gives the lent handle's last sequence back, as the
		// messagebus sender does (envelope.PanelSeq).
		defer h.Advance(served.Seq())
	}
	if s.started != nil {
		close(s.started)
	}
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.fail != nil {
		return nil, s.fail
	}
	return s.answer, nil
}

func (s *installSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

// panelEvents collects the events out of one panel.
func panelEvents(t *testing.T, bus mbapi.MessageBus, universe, id string) func() []panel.Event {
	t.Helper()
	var mu sync.Mutex
	var got []panel.Event
	sub, err := bus.Subscribe(context.Background(), subjects.PanelOutSubject(panelTenant, universe, id), func(_ context.Context, m mbapi.Message) {
		if e, err := panel.DecodeEvent(m.Data()); err == nil {
			mu.Lock()
			got = append(got, e)
			mu.Unlock()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return func() []panel.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]panel.Event(nil), got...)
	}
}

// lastEvent is the panel's end once it came (the stub bus delivers each
// event on its own: their order on arrival is not the sequence's).
func lastEvent(events func() []panel.Event) panel.Event {
	return eventOf(events(), panel.KindComplete)
}

// eventOf is the first event of kind.
func eventOf(es []panel.Event, kind panel.Kind) panel.Event {
	for _, e := range es {
		if e.Kind == kind {
			return e
		}
	}
	return panel.Event{}
}

// : a control-plane install runs as a
// direct KTSP call lent a progress panel the GUI opened on the admin
// universe's panel server - the method's steps go into it, numbered after
// its open, and the panel completes with the call's answer; a failure
// completes it failed, with the error the screen shows.
func TestAnInstallCallIsLentThePanelTheGUIOpenedAndCompletesIt(t *testing.T) {
	ctx := context.Background()
	bus := teststub.New()

	h, err := openCallPanel(ctx, bus, "admin", "Install ktw-acme")
	if err != nil {
		t.Fatal(err)
	}
	events := panelEvents(t, bus, "admin", h.ID())
	s := &installSender{bus: bus, answer: json.RawMessage(`{"name":"ktw-acme","version":"1.2.0"}`)}

	got := callInPanel(ctx, bus, s, h, "admin", "ktw-univers:marketplace/install", map[string]any{"name": "ktw-acme"})
	if got.Error != "" || string(got.Response) != `{"name":"ktw-acme","version":"1.2.0"}` {
		t.Fatalf("call = %+v, want the install's answer", got)
	}
	if s.lent == nil || s.lent.ID() != h.ID() {
		t.Fatalf("the call was lent %v, want the panel %s (the ktw.meta.panel header's handle)", s.lent, h.ID())
	}
	waitUntil(t, func() bool { return len(events()) == 2 })
	es := events()
	if step := eventOf(es, panel.KindProgress); step.Seq != 2 || step.Progress == nil {
		t.Fatalf("events = %+v, want the method's step at seq 2 then the end", es)
	}
	end := eventOf(es, panel.KindComplete)
	if end.Seq != 3 || end.Complete == nil || end.Complete.Status != callPanelOK {
		t.Fatalf("end = %+v, want complete ok at seq 3, after the method's step", end)
	}
	if b, _ := json.Marshal(end.Complete.Result); !strings.Contains(string(b), `"version":"1.2.0"`) {
		t.Errorf("the panel's result = %s, want the install's answer", b)
	}

	// A failed install completes its panel failed, with the error.
	h2, _ := openCallPanel(ctx, bus, "admin", "Update")
	events2 := panelEvents(t, bus, "admin", h2.ID())
	got = callInPanel(ctx, bus, &installSender{bus: bus, fail: errors.New("download refused")}, h2, "admin", "ktw-univers:marketplace/update", nil)
	if !strings.Contains(got.Error, "download refused") {
		t.Fatalf("a failed call = %+v, want its error", got)
	}
	waitUntil(t, func() bool { return lastEvent(events2).Kind == panel.KindComplete })
	if end := lastEvent(events2); end.Complete.Status != callPanelFailed {
		t.Errorf("a failed install's panel ends %q, want failed", end.Complete.Status)
	}
}

// The person's Cancel - on the panel itself, or WorkspaceCancelCall by the
// call's id - gives the install up and completes its panel cancelled.
func TestACancelGivesTheLentCallUpAndCompletesItsPanelCancelled(t *testing.T) {
	ctx := context.Background()
	bus := teststub.New()

	// On the panel: its cancel input.
	h, _ := openCallPanel(ctx, bus, "admin", "Install")
	events := panelEvents(t, bus, "admin", h.ID())
	s := &installSender{bus: bus, block: true, started: make(chan struct{})}
	res := make(chan KTSPCallResult, 1)
	go func() { res <- callInPanel(ctx, bus, s, h, "admin", "ktw-univers:marketplace/install-prereqs", nil) }()
	<-s.started
	in, _ := json.Marshal(panel.Input{Panel: h.ID(), Kind: panel.InputCancel, By: "jdoe"})
	if err := bus.Publish(ctx, subjects.PanelInSubject(panelTenant, "admin", h.ID()), in); err != nil {
		t.Fatal(err)
	}
	if got := <-res; !strings.Contains(got.Error, "cancelled") {
		t.Fatalf("a cancelled call = %+v, want it said cancelled", got)
	}
	waitUntil(t, func() bool { return lastEvent(events).Kind == panel.KindComplete })
	if end := lastEvent(events); end.Complete.Status != callPanelCancelled {
		t.Errorf("the panel ends %q, want cancelled", end.Complete.Status)
	}

	// By the call's id, as WorkspaceCancelCall does.
	h2, _ := openCallPanel(ctx, bus, "admin", "Doctor")
	events2 := panelEvents(t, bus, "admin", h2.ID())
	s2 := &installSender{bus: bus, block: true, started: make(chan struct{})}
	callCtx, done := guiCalls.begin(ctx, "mkt-test-1")
	defer done()
	go func() { res <- callInPanel(callCtx, bus, s2, h2, "admin", "ktw-univers:marketplace/doctor", nil) }()
	<-s2.started
	guiCalls.cancel("mkt-test-1")
	if got := <-res; !strings.Contains(got.Error, "cancelled") {
		t.Fatalf("a call given up by its id = %+v, want it said cancelled", got)
	}
	waitUntil(t, func() bool { return lastEvent(events2).Kind == panel.KindComplete })
	if end := lastEvent(events2); end.Complete.Status != callPanelCancelled {
		t.Errorf("the panel ends %q, want cancelled", end.Complete.Status)
	}
}

// The panel opened under a call's id is handed to that call once: a second
// call under the same id finds none.
func TestACallPanelIsHandedToItsCallOnce(t *testing.T) {
	h, err := openCallPanel(context.Background(), teststub.New(), "admin", "Install")
	if err != nil {
		t.Fatal(err)
	}
	var c callPanels
	c.open = map[string]*panel.Handle{}
	c.put("c1", h)
	if got, ok := c.take("c1"); !ok || got != h {
		t.Fatalf("take = %v %v, want the panel opened under c1", got, ok)
	}
	if _, ok := c.take("c1"); ok {
		t.Fatal("the panel was handed out twice")
	}
	if got, err := openCallPanel(context.Background(), nil, "admin", "x"); err == nil || got != nil {
		t.Fatal("a panel opened with no bus, want refused")
	}
}

// A lent call whose budget passes is a failure, not a cancel (
// verify): the person gave nothing up, so the panel ends failed, with the
// deadline's error the screen shows.
func TestALentCallWhoseBudgetPassesCompletesItsPanelFailed(t *testing.T) {
	bus := teststub.New()
	h, _ := openCallPanel(context.Background(), bus, "admin", "Install")
	events := panelEvents(t, bus, "admin", h.ID())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	callCtx, done := guiCalls.begin(ctx, "mkt-budget-1")
	defer done()
	got := callInPanel(callCtx, bus, &installSender{bus: bus, block: true}, h, "admin", "ktw-univers:marketplace/install", nil)
	if got.Error == "" || strings.Contains(got.Error, "cancelled") {
		t.Fatalf("a call past its budget = %+v, want its deadline error, not a cancel", got)
	}
	waitUntil(t, func() bool { return lastEvent(events).Kind == panel.KindComplete })
	end := lastEvent(events)
	if end.Complete.Status != callPanelFailed {
		t.Fatalf("a call past its budget ends its panel %q, want failed", end.Complete.Status)
	}
	if b, _ := json.Marshal(end.Complete.Result); !strings.Contains(string(b), "deadline") {
		t.Errorf("the panel's result = %s, want the deadline's error", b)
	}
}
