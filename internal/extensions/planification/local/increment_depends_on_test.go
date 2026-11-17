// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT'S « WHAT IT DEPENDS ON » DRAWS ONLY ITS LINKED ITEMS
// (mock-up modules/planification/mockups/RQ-1209-depends-on.html):
// the graph increment-dependencies holds the boxes a depends-on arrow touches
// and the frame holding them; every other item of the increment is a card of
// the grid increment-unlinked below it. With no arrow, no graph: a dimmed line
// and every item a card. The graph carries the element id
// data-view=increment-dependencies (the GUI passes only data-* keys).
func TestAnIncrementsDependenciesDrawOnlyItsLinkedItems(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid", Goals: []string{"Paid invoices"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Increment.ID
	runIncrement(t, ctx, id)
	var items []string
	for _, title := range []string{"Send", "Remind", "Archive", "Export", "Print"} {
		items = append(items, file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Increment: id, Kind: api.KindBuild, By: person}).ID)
	}
	section := func() (graph []string, cards []string, dimmed bool) {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: id, Tab: "page", Language: "fr"})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Increment tab must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(b, &tree)
		var walk func(n any)
		walk = func(n any) {
			switch v := n.(type) {
			case map[string]any:
				switch {
				case v["kind"] == "graph" && v["id"] == "increment-dependencies":
					if v["data-view"] != "increment-dependencies" {
						t.Errorf("the graph carries data-view %v; want increment-dependencies, the element id its GUI test selects", v["data-view"])
					}
					for _, b := range v["data"].(map[string]any)["nodes"].([]any) {
						graph = append(graph, b.(map[string]any)["id"].(string))
					}
					return
				case v["data-view"] == "increment-unlinked":
					for _, c := range v["data"].([]any) {
						cards = append(cards, c.(map[string]any)["id"].(string))
					}
					return
				case v["content"] == "Aucune dépendance entre ses éléments.":
					dimmed = true
				}
				for _, c := range v {
					walk(c)
				}
			case []any:
				for _, c := range v {
					walk(c)
				}
			}
		}
		walk(tree)
		return
	}

	graph, cards, dimmed := section()
	if len(graph) != 0 || !dimmed || len(cards) != len(items) {
		t.Errorf("no link: graph %v, dimmed line %v, cards %v; want no graph, the dimmed line and the %d items as cards", graph, dimmed, cards, len(items))
	}

	addDependsOn(t, ctx, cwm, items[1], items[0]) // Remind waits on Send
	graph, cards, dimmed = section()
	if want := map[string]bool{id: true, items[0]: true, items[1]: true}; len(graph) != len(want) || !want[graph[0]] || !want[graph[1]] || !want[graph[2]] {
		t.Errorf("one link: the graph holds %v; want the frame %s and the linked pair %s, %s only", graph, id, items[0], items[1])
	}
	if len(cards) != 3 || cards[0] != items[2] || cards[1] != items[3] || cards[2] != items[4] || dimmed {
		t.Errorf("one link: cards %v (dimmed line %v); want the unlinked %v below the graph", cards, dimmed, items[2:])
	}
}
