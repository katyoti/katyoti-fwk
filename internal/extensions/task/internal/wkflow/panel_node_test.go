// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

var panelSteps = []any{
	map[string]any{"kind": "panel", "name": "pub", "open": map[string]any{"type": "ktw-x:/panel-type/publish", "title": "Publication"}},
	map[string]any{"kind": "panel", "name": "pub", "progress": map[string]any{"label": "Pages", "current": 1, "total": 3}},
	map[string]any{"kind": "panel", "name": "pub", "ask": map[string]any{"question": "Overwrite?", "actions": []any{map[string]any{"name": "yes"}, map[string]any{"name": "no"}}}},
	map[string]any{"kind": "panel", "name": "pub", "complete": map[string]any{"status": "ok"}},
	map[string]any{"kind": "return", "value": "${results}"},
}

type panelRig struct {
	mu   sync.Mutex
	sent []panel.Event
}

func (r *panelRig) publisher(_ context.Context, evs []panel.Event) {
	r.mu.Lock()
	r.sent = append(r.sent, evs...)
	r.mu.Unlock()
}

func (r *panelRig) events() []panel.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]panel.Event(nil), r.sent...)
}

// waitAsk waits until the panel's ask went out and answers its id.
func (r *panelRig) waitAsk(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range r.events() {
			if e.Kind == panel.KindAsk {
				return e.Panel
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the panel never asked")
	return ""
}

func answer(bus *engine.MemoryEventBus, id string) {
	raw, _ := json.Marshal(panel.Input{Panel: id, Kind: panel.InputAction, Name: "yes", By: "jdoe"})
	bus.Publish(engine.ExternalEventTopic(engine.PanelInputURI(id)), json.RawMessage(raw))
}

func panelMachine(t *testing.T, ctx context.Context) engine.Machine {
	t.Helper()
	m, err := (&workflowFactory{}).CreateMachine(ctx, &taskapi.TaskExecRequest{Config: map[string]any{"steps": panelSteps}})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// A workflow drives its panel step by step: the open, the progress, the ask
// and the complete go out numbered 1..4 on one panel; the ask waits for the
// person's input and keeps it as results.<name>.
func TestAWorkflowDrivesItsPanelAndWaitsOnItsInput(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)
	rig := &panelRig{}
	exec := engine.NewExecutor(panelMachine(t, ctx), channel, nil)
	exec.SetPanelPublisher(rig.publisher)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done := make(chan *engine.RunResult, 1)
	go func() {
		res, err := exec.Run(runCtx, engine.Event{Type: "start"})
		if err != nil {
			t.Errorf("Run: %v", err)
		}
		done <- res
	}()
	id := rig.waitAsk(t)
	answer(bus, id)
	res := <-done
	evs := rig.events()
	kinds := []panel.Kind{panel.KindOpen, panel.KindProgress, panel.KindAsk, panel.KindComplete}
	if len(evs) != len(kinds) {
		t.Fatalf("%d panel events, want %d: %+v", len(evs), len(kinds), evs)
	}
	for i, e := range evs {
		if e.Kind != kinds[i] || e.Seq != int64(i+1) || e.Panel != id {
			t.Fatalf("event %d: %s seq %d on %s", i, e.Kind, e.Seq, e.Panel)
		}
	}
	got, _ := res.Output.(map[string]any)["pub"].(map[string]any)
	if got["name"] != "yes" || got["by"] != "jdoe" {
		t.Fatalf("results.pub = %#v", res.Output)
	}
}

// The ask's wait survives a restart: the run stopped while asking resumes
// from its checkpoint, waits on the same panel, and the input given then ends
// the ask; the numbering goes on from the checkpoint.
func TestAPanelAskSurvivesARestart(t *testing.T) {
	store := engine.NewMemoryStateStore()
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)
	rig := &panelRig{}
	first := engine.NewExecutor(panelMachine(t, ctx), channel, store)
	first.SetPanelPublisher(rig.publisher)
	first.SetRunID("run-panel")
	runCtx, stop := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() { _, _ = first.Run(runCtx, engine.Event{Type: "start"}); close(stopped) }()
	id := rig.waitAsk(t)
	stop() // the process dies while the panel asks
	<-stopped

	bus2 := engine.NewMemoryEventBus()
	channel2 := engine.NewChannel(bus2)
	ctx2 := withLocalSpawner(channel2)
	second := engine.NewExecutor(panelMachine(t, ctx2), channel2, store)
	second.SetPanelPublisher(rig.publisher)
	resCtx, cancel := context.WithTimeout(ctx2, 10*time.Second)
	defer cancel()
	done := make(chan *engine.RunResult, 1)
	go func() {
		res, err := second.ResumeWaiting(resCtx, "run-panel")
		if err != nil {
			t.Errorf("ResumeWaiting: %v", err)
		}
		done <- res
	}()
	time.Sleep(50 * time.Millisecond)
	answer(bus2, id)
	res := <-done
	if res == nil {
		t.Fatal("no result after the restart")
	}
	evs := rig.events()
	last := evs[len(evs)-1]
	if last.Kind != panel.KindComplete || last.Seq != 4 || last.Panel != id {
		t.Fatalf("after the restart: %+v", last)
	}
	if got, _ := res.Output.(map[string]any)["pub"].(map[string]any); got["name"] != "yes" {
		t.Fatalf("results.pub = %#v", res.Output)
	}
}

func TestAPanelStepIsOneActionOnANamedPanel(t *testing.T) {
	for name, step := range map[string]map[string]any{
		"no name":     {"kind": "panel", "open": map[string]any{"type": "t"}},
		"no action":   {"kind": "panel", "name": "p"},
		"two actions": {"kind": "panel", "name": "p", "open": map[string]any{}, "ask": map[string]any{}},
	} {
		if _, err := LoadNodes(map[string]any{"steps": []any{step}}); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}
