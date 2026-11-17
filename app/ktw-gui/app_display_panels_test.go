// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// : the GUI follows a display panel by its id - each event out of
// the panel reaches the screen with the follow it answers, until the follow
// stops - and sends the person's input back on the panel's input subject,
// validated.
func TestTheGUIFollowsADisplayPanelAndSendsTheInputBack(t *testing.T) {
	ctx := context.Background()
	bus := teststub.New()
	var f panelFollows
	var mu sync.Mutex
	var got []map[string]any
	id := panel.NewID()
	followID, err := f.follow(bus, "u1", id, "", func(p map[string]any) {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := panel.OpenPanelWithID(ctx, panel.Scope{Tenant: panelTenant, Universe: "u1"},
		func(ctx context.Context, s string, b []byte) error { return bus.Publish(ctx, s, b) }, id,
		panel.Open{Title: "T", Answer: render.Answer{Data: map[string]any{"x": 1}}})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 })
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if first["follow"] != followID || first["panel"] != id || first["universe"] != "u1" {
		t.Fatalf("event payload = %v, want its follow, panel and universe", first)
	}
	var e panel.Event
	if err := json.Unmarshal(first["event"].(json.RawMessage), &e); err != nil || e.Seq != 1 || e.Kind != panel.KindOpen {
		t.Fatalf("event = %+v (%v), want the open at seq 1", e, err)
	}
	f.stop(followID)
	_ = h.Progress(ctx, "", 1, 2)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("%d events after the follow stopped, want none more", n-1)
	}

	// The person's input goes out on the panel's input subject.
	ins := make(chan []byte, 1)
	if _, err := bus.Subscribe(ctx, subjects.PanelInSubject(panelTenant, "u1", id), func(_ context.Context, m mbapi.Message) { ins <- m.Data() }); err != nil {
		t.Fatal(err)
	}
	if err := sendPanelInput(ctx, bus, "u1", `{"panel":"`+id+`","kind":"action","name":"publish","by":"jdoe"}`); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-ins:
		in, err := panel.DecodeInput(b)
		if err != nil || in.Kind != panel.InputAction || in.Name != "publish" {
			t.Fatalf("input sent = %s (%v), want the action publish", b, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no input on the panel's input subject")
	}
	if err := sendPanelInput(ctx, bus, "u1", `{"panel":"`+id+`","kind":"action"}`); err == nil {
		t.Fatal("an action naming no action was sent")
	}
	if _, err := f.follow(bus, "u1", "not-a-panel", "", func(map[string]any) {}); err == nil {
		t.Fatal("a follow of no panel id was started")
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// : no follow outlives what it was for - a universe deactivated or
// restarted ends its follows, a reloaded page ends the ones the page before
// it started, the shutdown ends them all; each ends its subscription.
func TestTheGUIEndsItsPanelFollowsWithTheirUniverseThePageAndTheGUI(t *testing.T) {
	bus := teststub.New()
	var f panelFollows
	open := func(universe, page string) {
		t.Helper()
		if _, err := f.follow(bus, universe, panel.NewID(), page, func(map[string]any) {}); err != nil {
			t.Fatal(err)
		}
	}
	f.keepPage("p1")
	open("u1", "")
	open("u1", "p1")
	open("u2", "p1")
	if f.size() != 3 {
		t.Fatalf("%d follows, want 3", f.size())
	}
	f.stopUniverse("u1")
	if f.size() != 1 {
		t.Fatalf("%d follows after u1 went, want u2's alone", f.size())
	}
	// A reload: the new page names itself; what the old one followed ends,
	// what the new one follows stays.
	open("u2", "p2")
	f.keepPage("p2")
	if f.size() != 1 {
		t.Fatalf("%d follows after the reload, want the new page's alone", f.size())
	}
	f.stopAll()
	if f.size() != 0 {
		t.Fatalf("%d follows after the shutdown, want none", f.size())
	}
}
