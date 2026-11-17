// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The deck's « À vous » tile on the Global strip counts what the rail's To do
// bubble counts: the method the rail's Inbox names as its badge
// (PersonQueueCount), on the same queue; its button opens the screen the rail
// opens.
func TestTheNeedsYouTileCountsWhatTheRailsBubbleCounts(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "Print one; read it."}}})
	toReview(t, ctx, r.ID)
	if _, err := inboxOf(t, ctx).File(ctx, inboxapi.FileInput{Title: "May I deploy?", Expects: inboxapi.ExpectsText}); err != nil {
		t.Fatal(err)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	badge, err := views.PersonQueueCount(ctx, api.PersonQueueInput{})
	if err != nil || badge.Count == 0 {
		t.Fatalf("the rail's badge reads %+v, %v; want items waiting", badge, err)
	}
	def, err := views.Deck(ctx, api.DeckInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	overview := tree.(map[string]any)["tabs"].([]any)[0]
	tile := findNode(overview, func(n map[string]any) bool { return n["data-view"] == "deck-needs-tile" })
	if tile == nil {
		t.Fatalf("no « À vous » tile on the Overview: %s", raw)
	}
	if v, _ := tile["value"].(float64); int(v) != badge.Count {
		t.Errorf("the tile counts %v; the rail's bubble %d", tile["value"], badge.Count)
	}
	open := findNode(tile, func(n map[string]any) bool { return n["data-view"] == "deck-needs-open" })
	if open == nil {
		t.Fatalf("the tile has no « Ouvrir À faire »: %v", tile)
	}
	if a, _ := open["action"].(map[string]any); a["screen"] != "todo" {
		t.Errorf("« Ouvrir À faire » opens %v; want the screen todo, the rail's To do", open["action"])
	}
}
