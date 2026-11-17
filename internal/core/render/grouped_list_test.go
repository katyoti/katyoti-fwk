// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"strings"
	"testing"
)

// A GROUPED LIST: any page groups a list by a field of its
// elements, names the groups' order and labels, the field saying an element
// is overdue, the headings of a second field (counted by kind, drawn by
// holder), and bulk actions answering several elements of one group - the
// page grammar takes them and refuses them off their shape.
func TestAGroupedListIsAListGroupedByAField(t *testing.T) {
	approve := RunURI("Approve", "ktw-agents:todos/report", map[string]any{"id": "${item.id}", "verdict": "approved"})
	list := ListOf([]any{map[string]any{"id": "ht-1", "kind": "approve", "late": true}}, "ktw-agents:/data-type/human-task", "").
		GroupBy("kind", Group{Key: "approve", Label: "Validation"}, Group{Key: "decide", Label: "Décision"}).
		Overdue("late", "").
		SectionBy("who", Group{Key: "mine", Label: "Mine"}, Group{Key: "team", Label: "Holding list hl-cosmos"}).
		Bulk(approve)
	if list["groupBy"] != "kind" || list["overdueField"] != "late" {
		t.Fatalf("grouped list = %v, want groupBy kind and overdueField late", list)
	}
	if _, set := list["overdueLabel"]; set {
		t.Fatalf("an empty overdue label was set: %v (the drawer's default)", list)
	}
	if gs, _ := list["groups"].([]Group); len(gs) != 2 || gs[1].Label != "Décision" {
		t.Fatalf("groups = %v, want the two named", list["groups"])
	}
	if list["sectionBy"] != "who" || list["groupBy"] != "kind" {
		t.Fatalf("grouped list = %v, want headings by who and counts by kind", list)
	}
	if ss, _ := list["sectionGroups"].([]Group); len(ss) != 2 || ss[0].Label != "Mine" {
		t.Fatalf("sectionGroups = %v, want Mine then the holding list", list["sectionGroups"])
	}
	if _, set := ListOf([]any{}, "t", "").GroupBy("kind").SectionBy("who")["sectionGroups"]; set {
		t.Fatal("sectionGroups set with no heading named")
	}
	if err := ValidatePage(Node{"kind": "vbox", "children": []any{list}}); err != nil {
		t.Fatalf("a grouped list is refused: %v", err)
	}
	if err := ValidatePage(List([]any{}, Text("x"), false).GroupBy("kind")); err != nil {
		t.Fatalf("a grouped list with its template is refused: %v", err)
	}

	refused := map[string]Node{
		"groupBy not a text":              List([]any{}, Text("x"), false).With("groupBy", 3),
		"groupBy empty":                   List([]any{}, Text("x"), false).With("groupBy", ""),
		"a group without key":             List([]any{}, Text("x"), false).GroupBy("kind").With("groups", []any{map[string]any{"label": "A"}}),
		"groups not a list":               List([]any{}, Text("x"), false).GroupBy("kind").With("groups", "a,b"),
		"bulk without groupBy":            List([]any{}, Text("x"), false).Bulk(approve),
		"bulk not a list":                 List([]any{}, Text("x"), false).GroupBy("kind").With("bulk", map[string]any{"uri": "a:b/c"}),
		"bulk action without uri":         List([]any{}, Text("x"), false).GroupBy("kind").With("bulk", []any{map[string]any{"label": "Go"}}),
		"sectionBy not a text":            List([]any{}, Text("x"), false).GroupBy("kind").With("sectionBy", 3),
		"sectionBy empty":                 List([]any{}, Text("x"), false).GroupBy("kind").With("sectionBy", ""),
		"sectionBy without groupBy":       List([]any{}, Text("x"), false).SectionBy("who"),
		"a section without key":           List([]any{}, Text("x"), false).GroupBy("kind").SectionBy("who").With("sectionGroups", []any{map[string]any{"label": "Mine"}}),
		"sectionGroups not a list":        List([]any{}, Text("x"), false).GroupBy("kind").SectionBy("who").With("sectionGroups", "mine"),
		"sectionGroups without sectionBy": List([]any{}, Text("x"), false).GroupBy("kind").With("sectionGroups", []any{map[string]any{"key": "mine"}}),
	}
	for name, n := range refused {
		err := ValidatePage(n)
		if err == nil {
			t.Errorf("%s: passed, want refused", name)
			continue
		}
		if !strings.Contains(err.Error(), "/list") {
			t.Errorf("%s: %v does not name the list", name, err)
		}
	}
}

// « À faire » on the grouped list: a heading for some groups
// alone (the conversations), the header in the page's words, headings without
// their count, and each element drawn as the node it carries - those nodes
// checked as the page's own.
func TestAGroupedListSpeaksItsPagesWordsAndDrawsItsElementsNodes(t *testing.T) {
	els := []any{map[string]any{"id": "a", "kind": "approve", "who": "mine", "card": Text("A")}}
	list := List(els, nil, false).NodeField("card").
		GroupBy("kind").SectionBy("who", Group{Key: "mine", Label: "Mine"}, Group{Key: "chats", Label: "Conversations", Only: []string{"conversation"}}).
		CountWords("Filter by kind", "Click a count to show only that kind.").NoSectionCounts()
	delete(list, "item")
	if list["nodeField"] != "card" || list["filterLabel"] != "Filter by kind" || list["sectionCounts"] != false {
		t.Fatalf("list = %v", list)
	}
	if _, set := List(nil, Text("x"), false).CountWords("", "")["filterLabel"]; set {
		t.Fatal("an empty label was set: the drawer's default")
	}
	if err := ValidatePage(Node{"kind": "vbox", "children": []any{list}}); err != nil {
		t.Fatalf("a list drawing its elements' nodes is refused: %v", err)
	}
	bad := List([]any{map[string]any{"id": "a", "card": Node{"kind": "nope"}}}, nil, false).NodeField("card")
	delete(bad, "item")
	if err := ValidatePage(bad); err == nil || !strings.Contains(err.Error(), "data[0].card") {
		t.Fatalf("an element's node of unknown kind: %v, want refused naming it", err)
	}
	refused := map[string]Node{
		"nodeField empty":          List([]any{}, Text("x"), false).With("nodeField", ""),
		"filterHint not a text":    List([]any{}, Text("x"), false).GroupBy("kind").With("filterHint", 3),
		"sectionCounts not a bool": List([]any{}, Text("x"), false).GroupBy("kind").With("sectionCounts", "no"),
		"only on a group":          List([]any{}, Text("x"), false).GroupBy("kind").With("groups", []any{map[string]any{"key": "a", "only": []any{"a"}}}),
		"only not texts":           List([]any{}, Text("x"), false).GroupBy("kind").SectionBy("who").With("sectionGroups", []any{map[string]any{"key": "a", "only": []any{3}}}),
		"only naming no group":     List([]any{}, Text("x"), false).GroupBy("kind").SectionBy("who").With("sectionGroups", []any{map[string]any{"key": "a", "only": []any{}}}),
	}
	for name, n := range refused {
		if err := ValidatePage(n); err == nil {
			t.Errorf("%s: passed, want refused", name)
		}
	}
}

// TestAGroupedListDrawsTilesSectionCardsAndPlaces: FilterTiles
// SectionCards and PlaceBy set the props the display library draws - tiles
// for the counts with a first « all » tile, a card per section, a place line
// by a field - each group's icon, colour and hint carried; ValidatePage
// refuses a style off its values, those props on a list grouped by nothing
// and places without their placeField.
func TestAGroupedListDrawsTilesSectionCardsAndPlaces(t *testing.T) {
	els := []any{map[string]any{"id": "a", "kind": "approve", "holder": "mine", "title": "A"}}
	n := List(els, Node{"kind": "text", "dataField": "$.item.title"}, false).
		GroupBy("kind", Group{Key: "approve", Label: "Validation", Icon: "circle-check", Color: "teal", Hint: "Validate"}).
		FilterTiles("Tout").SectionCards().PlaceBy("holder", Group{Key: "mine", Label: "Pour moi"})
	if n["filterStyle"] != "tiles" || n["allLabel"] != "Tout" || n["sectionStyle"] != "card" || n["placeField"] != "holder" {
		t.Fatalf("props = %v", n)
	}
	if err := ValidatePage(roundTrip(t, n)); err != nil {
		t.Fatalf("a list with tiles, cards and places is refused: %v", err)
	}
	g := roundTrip(t, n).(map[string]any)["groups"].([]any)[0].(map[string]any)
	if g["icon"] != "circle-check" || g["color"] != "teal" || g["hint"] != "Validate" {
		t.Errorf("the group's look is lost: %v", g)
	}
	for name, bad := range map[string]Node{
		"filterStyle off its values":         n.With("filterStyle", "grid"),
		"sectionStyle off its values":        n.With("sectionStyle", "box"),
		"places without placeField":          List(els, Node{"kind": "text"}, false).GroupBy("kind").With("places", []Group{{Key: "mine"}}),
		"tiles on a list grouped by nothing": List(els, Node{"kind": "text"}, false).FilterTiles(""),
		"an empty placeField":                n.With("placeField", ""),
	} {
		if err := ValidatePage(roundTrip(t, bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// roundTrip is a node as a page carries it: through JSON.
func roundTrip(t *testing.T, n Node) any {
	t.Helper()
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
