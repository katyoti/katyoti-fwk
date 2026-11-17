// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A block or refusal note can be long: « À faire »'s card and the release
// checklist's rows draw it as a text clamped to three lines, expandable, the
// toggle's id coming from the text's.
func TestALongNoteIsClampedToThreeLinesExpandable(t *testing.T) {
	isClampedNote := func(n node, id string) bool {
		return n["kind"] == "text" && n["lineClamp"] == 3 && n["expandable"] == true && n["id"] == id
	}
	long := "It is blocked: " + strings.Repeat("a long reason that runs on and on, ", 20) + " — resume it once that is settled, or drop it."
	it := api.PersonItem{Key: "ops.RQ-436", Object: "ops.RQ-436", Request: "ops.RQ-436",
		Module: "planification", Title: "a blocked request", Action: api.ActionUnblock, Asks: long}
	card := queueCard(it, "fr", "")
	var note node
	for _, k := range card["children"].([]node) {
		if k["content"] == long {
			note = k
		}
	}
	if note == nil || !isClampedNote(note, "asks:ops.RQ-436") {
		t.Fatalf("« À faire »'s card: the note must be a text clamped to 3 lines, expandable, id asks:<key>; got %v", note)
	}
	if card["id"] != "ops.RQ-436" {
		t.Errorf("the card keeps its id (card:<key>): %v", card["id"])
	}

	row := itemTitles("blocked")
	var meta node
	for _, k := range row["children"].([]node) {
		if k["dataField"] == "$.item.meta" {
			meta = k
		}
	}
	if meta == nil || !isClampedNote(meta, "blocked-meta") {
		t.Fatalf("the release checklist's row: its meta must be a text clamped to 3 lines, expandable, id <tree>-meta; got %v", meta)
	}
	// Each row's note draws its own toggle: the id binds the row's item id
	// (text:<tree>-meta:<item id>:toggle).
	if meta["idField"] != "$.item.id" {
		t.Errorf("the release checklist's row note must bind its id per row (idField $.item.id); got %v", meta["idField"])
	}

	// Both draw: the page grammar takes them, two trees side by side
	// keeping their toggle ids apart.
	tree := func(key string) node {
		return node{"kind": "tree", "data": []node{{"id": "x", "title": "x", "state": "blocked", "meta": long}}, "item": itemTitles(key), "badgeField": "state"}
	}
	if err := render.ValidatePage(node{"kind": "vbox", "children": []node{card, tree("running"), tree("blocked")}}); err != nil {
		t.Fatalf("the notes must draw: %v", err)
	}
}
