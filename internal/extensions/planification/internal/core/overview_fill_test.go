// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Running now and the current releases each take the window's height below them, their
// body - each card's last child - scrolling inside the box, so the page does
// not scroll.
func TestTheOverviewBoxesFillTheHeightAndScrollTheirBody(t *testing.T) {
	stands := []stand{{plan: api.PlannedObject{ID: "ui.RQ-1", Title: "ui.RQ-1"}, state: stateLabels[standRunning]}}
	current := section("Current releases", "rocket", "teal", "", node{"kind": "vbox", "data-view": currentReleasesView, "children": []node{}})
	tab := overviewTab("en", stands, nil, api.PeriodWeek, map[string]int{}, needsYou{}, current)
	if err := render.ValidatePage(roundTrip(t, tab)); err != nil {
		t.Fatalf("the Overview does not render: %v", err)
	}
	var boxes node
	for _, c := range tab["children"].([]node) {
		if c["data-view"] == overviewBoxesView {
			boxes = c
		}
	}
	if boxes == nil {
		t.Fatalf("no row of boxes in the Overview")
	}
	kids := boxes["children"].([]node)
	if len(kids) != 2 {
		t.Fatalf("the row holds %d boxes; want Running now and the current releases", len(kids))
	}
	for i, box := range kids {
		if box["kind"] != "card" || box["height"] != "fill" {
			t.Errorf("box %d is %v with height %v; want a card of height fill", i, box["kind"], box["height"])
			continue
		}
		body := box["children"].([]node)
		if last := body[len(body)-1]; last["kind"] == "heading" || last["kind"] == "text" {
			t.Errorf("box %d's last child is its %v, not its body: the body would not scroll", i, last["kind"])
		}
	}
}
