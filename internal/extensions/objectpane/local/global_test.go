// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// A GLOBAL panel is a tab of the Global strip — by order then name, its source
// kept to be called with a target; its filter still places it on objects, and
// a global one with an empty filter shows on none.
func TestGlobalPanelsAreTheGlobalStrip(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/planned.yaml":      "name: planned\n",
		"object-types/thing.yaml":        "name: thing\ntree: node\ncapabilities: [planned]\n",
		"object-panels/deck.yaml":        "name: Planification\norder: 20\nglobal: true\nfilter: {capabilities: [planned]}\nsource: 'ktw-agents:planning/deck'\ntargets: 'ktw-agents:planning/deck-targets'\n",
		"object-panels/only-global.yaml": "name: Overview\norder: 10\nglobal: true\n" + ext,
		"object-panels/on-objects.yaml":  "name: Local\nfilter: {capabilities: [planned]}\n" + ext,
	})
	got, err := svc.GlobalPanels(ctx, api.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Panels) != 2 || got.Panels[0].ID != "home:only-global" || got.Panels[1].ID != "home:deck" {
		t.Fatalf("global strip = %v, want only-global then deck (by order)", ids(got.Panels))
	}
	if d := got.Panels[1]; d.Kind != api.PanelKindSource || d.Source != "ktw-agents:planning/deck" || d.Name != "Planification" || d.Targets != "ktw-agents:planning/deck-targets" {
		t.Errorf("the deck = %+v, want a source panel kept for its target", d)
	}

	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	by := panelByID(detail(t, ctx, svc, "m.thing", false).Panels)
	if _, ok := by["tab:deck"]; !ok {
		t.Errorf("a global panel with a filter is on the objects it admits too: %v", by)
	}
	if _, ok := by["tab:only-global"]; ok {
		t.Errorf("a global panel with an empty filter showed on an object")
	}
	if _, ok := by["tab:on-objects"]; !ok {
		t.Errorf("an ordinary panel went missing")
	}
}

// A global panel may declare a COUNT — the method answering how many things it
// holds for the person — and the Global strip carries it, for the rail to show
// beside the panel's name; a panel declaring none carries none.
func TestGlobalPanelCount(t *testing.T) {
	ctx, _, svc := paneWorld(t, map[string]string{
		"object-panels/asks.yaml": "name: To do\norder: 5\nglobal: true\nsource: 'ktw-agents:views/person-queue-page'\ncount: 'ktw-agents:views/person-queue-count'\n" + ext,
		"object-panels/deck.yaml": "name: Planification\norder: 20\nglobal: true\nsource: 'ktw-agents:views/deck'\n" + ext,
	})
	got, err := svc.GlobalPanels(ctx, api.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	by := panelByID(got.Panels)
	if c := by["home:asks"].Count; c != "ktw-agents:views/person-queue-count" {
		t.Errorf("the declared count = %q, want ktw-agents:views/person-queue-count", c)
	}
	if c := by["home:deck"].Count; c != "" {
		t.Errorf("a panel declaring no count carries %q", c)
	}
}

// A panel may declare its EMPTY state — the sentence a person
// reads when it has nothing to show and the one action that starts filling it
// — and both the Global strip and an object's tabs carry it; a panel
// declaring none carries none.
func TestPanelEmptyState(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"capabilities/planned.yaml": "name: planned\n",
		"object-types/thing.yaml":   "name: thing\ntree: node\ncapabilities: [planned]\n",
		"object-panels/files.yaml": "name: My files\nglobal: true\nfilter: {capabilities: [planned]}\nsource: 'ktw-x:views/files'\n" +
			"empty: {text: 'No file yet.', label: 'Create my first', action: 'ktw-x:tasks/new-file', input: {kind: study}}\n",
		"object-panels/deck.yaml": "name: Planification\nglobal: true\nsource: 'ktw-x:views/deck'\n" + ext,
	})
	got, err := svc.GlobalPanels(ctx, api.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	by := panelByID(got.Panels)
	e := by["home:files"].Empty
	if e == nil || e.Text != "No file yet." || e.Label != "Create my first" || e.Action != "ktw-x:tasks/new-file" || e.Input["kind"] != "study" {
		t.Fatalf("the declared empty state = %+v, want its text, label, action and input", e)
	}
	if by["home:deck"].Empty != nil {
		t.Errorf("a panel declaring no empty state carries one")
	}
	create(t, ctx, cwm, cwmapi.Object{ID: "m.thing", Type: "thing"})
	if e := panelByID(detail(t, ctx, svc, "m.thing", false).Panels)["tab:files"].Empty; e == nil || e.Text != "No file yet." {
		t.Errorf("on an object the panel's empty state = %+v", e)
	}
}
