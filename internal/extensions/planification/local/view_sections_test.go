// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE PLANNING VIEWS ARE BUILT OF SECTIONS: the deck's
// History, Deliveries and Task groups and the Delivery view's Running are
// sections loaded when their tab opens - the view reads none of them to
// open - and each section's load answers its tab.
func TestThePlanningViewsAreBuiltOfSections(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	done := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: done.ID, Outcome: api.PlanDone, By: person, Note: "done by hand"}); err != nil {
		t.Fatal(err)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	streams := streams(t, ctx)
	tabsOf := func(def render.Def) map[string]map[string]any {
		t.Helper()
		if err := render.ValidatePage(def.DSLTree); err != nil {
			t.Fatalf("off the page grammar: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		out := map[string]map[string]any{}
		for _, tb := range tree["tabs"].([]any) {
			m := tb.(map[string]any)
			out[m["id"].(string)] = m
		}
		return out
	}
	js := func(v any) string { raw, _ := json.Marshal(v); return string(raw) }

	// The deck's History, Deliveries and Task groups are sections, each
	// loading its tab; the Overview is built at once.
	deck, err := views.Deck(ctx, api.DeckInput{Target: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	lazy := tabsOf(deck)
	for _, tab := range []string{api.DeckTabHistory, api.DeckTabDeliveries, api.DeckTabGroups} {
		sec, _ := lazy[tab]["content"].(map[string]any)
		load, _ := sec["load"].(map[string]any)
		args, _ := load["args"].(map[string]any)
		if load["method"] != "ktw-agents:views/deck-tab" || args["tab"] != tab || args["target"] != "billing" || sec["id"] != "deck:"+tab {
			t.Fatalf("the deck's %s tab = %s; want a section loading views/deck-tab for it", tab, js(sec))
		}
		got, err := views.DeckTab(ctx, api.DeckTabInput{Target: "billing", Tab: tab})
		if err != nil {
			t.Fatalf("DeckTab %s: %v", tab, err)
		}
		if err := render.ValidatePage(got.DSLTree); err != nil {
			t.Fatalf("DeckTab %s is off the page grammar: %v", tab, err)
		}
		if strings.Contains(js(got.DSLTree), `"load"`) {
			t.Errorf("DeckTab %s answers a section again: %s", tab, js(got.DSLTree))
		}
		if tab == api.DeckTabHistory && !strings.Contains(js(got.DSLTree), done.ID) {
			t.Errorf("DeckTab history does not list %s: %s", done.ID, js(got.DSLTree))
		}
	}
	if lazy["overview"]["content"] == nil || strings.Contains(js(lazy["overview"]["content"]), `"method":"ktw-agents:views/deck-tab"`) {
		t.Error("the Overview is not built at once")
	}
	if _, err := views.DeckTab(ctx, api.DeckTabInput{Tab: "overview"}); err == nil {
		t.Error("DeckTab answered a tab it does not load")
	}

	// Delivery's Running is a section, its badge counted without it.
	delivery, err := streams.DeliveryPage(ctx, api.DeliveryPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	running := tabsOf(delivery)["running"]
	sec, _ := running["content"].(map[string]any)
	if load, _ := sec["load"].(map[string]any); load["method"] != "ktw-agents:streams/running-page" {
		t.Fatalf("Delivery's Running = %s; want a section loading streams/running-page", js(sec))
	}
	page, err := streams.RunningPage(ctx, api.DeliveryPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(page.DSLTree); err != nil {
		t.Fatalf("RunningPage is off the page grammar: %v", err)
	}
	if _, ok := running["badge"].(float64); !ok {
		t.Errorf("Running's badge is %v; want the count of the increments being built", running["badge"])
	}
}
