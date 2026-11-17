// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The Overview shows « Needs you » as a tile on the right of « By phase »,
// one row: the person queue's waiting count, one line of what each item asks
// and « Ouvrir À faire » opening the rail's To do; the Needs you card grid
// left the Overview - it stays in « Needs a person ».
// By phase is a slim header - its title, a thin phase bar with its chips, the
// tile on its right, no tall section - and Running now and the current releases sit below
// it side by side.
func TestTheOverviewShowsNeedsYouAsATileBesideByPhase(t *testing.T) {
	stands := []stand{{plan: api.PlannedObject{ID: "ui.RQ-1", Title: "ui.RQ-1"}, state: stateLabels[api.PlanBlocked]}}
	needs := needsYou{count: 3, byAction: map[string]int{api.ActionValidate: 2, api.ActionAnswer: 1}}
	tab := overviewTab("fr", stands, nil, api.PeriodWeek, map[string]int{}, needs, node{"kind": "text", "content": "next"})
	if err := render.ValidatePage(roundTrip(t, tab)); err != nil {
		t.Fatalf("the Overview does not render: %v", err)
	}

	// By phase and the tile share one slim header: its title, the phase bar,
	// the tile in the right column.
	kids := tab["children"].([]node)
	var header, boxes node
	for i, c := range kids {
		if c["data-view"] == phaseHeaderView {
			header = c
			if i+1 < len(kids) {
				boxes = kids[i+1]
			}
		}
	}
	if header == nil {
		t.Fatalf("no header holds By phase and Needs you: %v", kids)
	}
	if raw := asJSON(t, header); strings.Contains(raw, `"level":5`) || strings.Contains(raw, `"size":"xl"`) {
		t.Errorf("the header still draws By phase as a tall section: %s", raw)
	}
	cols, _ := header["children"].([]node)[0]["children"].([]node)
	if len(cols) != 3 {
		t.Fatalf("the header's row has %d columns; want the title, the bar and the tile: %v", len(cols), header)
	}
	if raw := asJSON(t, cols[0]); !strings.Contains(raw, `"text":"Par phase"`) {
		t.Errorf("the header's first column is not By phase's title: %s", raw)
	}
	gauge := cols[1]["children"].([]node)[0]
	if gauge["kind"] != "gauge" || gauge["size"] != "sm" || gauge["legend"] != true {
		t.Errorf("the header's bar is %v; want a thin gauge (sm) with its phase chips", gauge)
	}
	if a, _ := gauge["action"].(render.Action); a.Tab != tabByPhase {
		t.Errorf("the bar opens %+v; want the By phase tab", gauge["action"])
	}
	tile := cols[2]
	if tile["kind"] != "kpi" || tile["data-view"] != needsTileView {
		t.Fatalf("the header's right column is not the Needs you tile: %v", tile)
	}

	// Below the header, Running now and the current releases side by side.
	if boxes == nil || boxes["data-view"] != overviewBoxesView || boxes["cols"] != 2 || len(boxes["children"].([]node)) != 2 {
		t.Errorf("Running now and the current releases are not one row of two boxes below the header: %v", boxes)
	} else if raw := asJSON(t, boxes); !strings.Contains(raw, `"text":"En cours maintenant"`) || !strings.Contains(raw, `"content":"next"`) {
		t.Errorf("the row below the header is not Running now and the current releases: %s", raw)
	}
	if tile["value"] != 3 || tile["label"] != "À vous" || tile["description"] != "2 à valider · 1 en attente de réponse" {
		t.Errorf("the tile reads %v %q %q; want 3, « À vous », what each item asks", tile["value"], tile["label"], tile["description"])
	}
	kids, _ = tile["children"].([]node)
	if len(kids) != 1 || kids[0]["kind"] != "button" || kids[0]["data-view"] != needsOpenView || kids[0]["label"] != "Ouvrir À faire" {
		t.Fatalf("the tile's button: %v; want « Ouvrir À faire », %s", kids, needsOpenView)
	}
	if a, _ := kids[0]["action"].(render.Action); a.Screen != render.ScreenTodo {
		t.Errorf("the button opens %+v; want the screen %s (the rail's To do)", kids[0]["action"], render.ScreenTodo)
	}

	// The tile is a call to act, as request-rows-mockup.html draws it: the
	// kpi's inline variant with no icon, its button outlined in grape
	//
	if tile["variant"] != "inline" {
		t.Errorf("the tile's variant is %v; want inline", tile["variant"])
	}
	if icon, has := tile["icon"]; has {
		t.Errorf("the tile carries the icon %v; the inline tile has none", icon)
	}
	if kids[0]["variant"] != "outline" || kids[0]["color"] != "grape" {
		t.Errorf("the button is %v in %v; want outline in grape", kids[0]["variant"], kids[0]["color"])
	}

	// No Needs you card grid is left on the Overview.
	if raw := asJSON(t, tab); strings.Contains(raw, `"layout":"grid"`) || strings.Contains(raw, "À vous —") {
		t.Errorf("the Overview still lists the Needs you cards: %s", raw)
	}

	// Nothing waiting: the tile reads 0, gray, saying so; its button stays.
	empty := needsTile("en", needsYou{})
	if empty["value"] != 0 || empty["color"] != "gray" || empty["variant"] != "inline" || empty["description"] != "Nothing waits on you" || len(empty["children"].([]node)) != 1 {
		t.Errorf("an empty queue's tile: %v", empty)
	}
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func roundTrip(t *testing.T, v any) any {
	t.Helper()
	var out any
	if err := json.Unmarshal([]byte(asJSON(t, v)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
