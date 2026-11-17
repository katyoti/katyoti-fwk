// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// « À FAIRE », ONE ACTION AT A TIME — a tile is a filter: the page re-read
// with its action shows only that action's section, every tile keeping its
// count, the chosen tile marked; « Tout » (no action) shows everything.

func TestPersonQueueFilterShowsOneAction(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	review := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson,
		Deliverables: []api.Deliverable{{Title: "it builds", Check: "go version"}}})
	toReview(t, ctx, review.ID)
	asked := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Ask(ctx, api.AskInput{ID: asked.ID, Question: "Which mail server?", By: intake}); err != nil {
		t.Fatal(err)
	}
	failed := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild})
	failPastTheCap(t, ctx, stepOf(t, ctx, failed.ID, api.StepIntake).ID, "no SMTP") // past the cap: the person\'s
	openDecision(t, ctx, cwm, "Keep the old format?", true)
	openDecision(t, ctx, cwm, "Keep the old paper?", true)

	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	read := func(action string) (string, map[string]map[string]any) {
		t.Helper()
		def, err := views.PersonQueuePage(ctx, api.PersonQueueInput{Language: "fr", Action: action})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		tiles := map[string]map[string]any{}
		var walk func(n any)
		walk = func(n any) {
			switch v := n.(type) {
			case map[string]any:
				if v["kind"] == "kpi" {
					tiles[v["label"].(string)] = v
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
		return string(raw), tiles
	}
	counts := map[string]float64{"Tout": 5, "Décider": 2, "Tester à la main": 0, "Valider": 1, "Répondre": 1, "Débloquer": 1, "En attente": 0}
	sections := map[string]string{"Décider": "Décider — 2", "Valider": "Valider — 1", "Répondre": "Répondre — 1", "Débloquer": "Débloquer — 1"}
	for _, c := range []struct{ action, chosen string }{
		{"", "Tout"}, {api.ActionDecide, "Décider"}, {api.ActionAnswer, "Répondre"}, {api.ActionUnblock, "Débloquer"}, {"nonsense", "Tout"},
	} {
		page, tiles := read(c.action)
		if len(tiles) != len(counts) {
			t.Errorf("%q: tiles %v; want « Tout » and one per action", c.action, keys(tiles))
		}
		for label, want := range counts {
			tile := tiles[label]
			if tile == nil {
				t.Errorf("%q: no tile %s", c.action, label)
				continue
			}
			if tile["value"] != want {
				t.Errorf("%q: tile %s counts %v, want %v (every action keeps its count)", c.action, label, tile["value"], want)
			}
			if sel, _ := tile["selected"].(bool); sel != (label == c.chosen) {
				t.Errorf("%q: tile %s selected=%v; want only %s marked", c.action, label, sel, c.chosen)
			}
			q, _ := tile["action"].(map[string]any)["query"].(map[string]any)
			if q == nil {
				t.Errorf("%q: tile %s is no filter: %v", c.action, label, tile["action"])
			}
		}
		for label, heading := range sections {
			shown := strings.Contains(page, heading)
			if want := c.chosen == "Tout" || c.chosen == label; shown != want {
				t.Errorf("%q: section %q shown=%v, want %v", c.action, heading, shown, want)
			}
		}
	}
	// A tile asks its action; « Tout » and the chosen tile again ask all.
	_, tiles := read(api.ActionDecide)
	args := func(label string) any {
		return tiles[label]["action"].(map[string]any)["query"].(map[string]any)["args"].(map[string]any)["action"]
	}
	if args("Répondre") != api.ActionAnswer || args("Tout") != "" || args("Décider") != "" {
		t.Errorf("the tiles ask Répondre=%v Tout=%v Décider(chosen)=%v; want answer, all, all", args("Répondre"), args("Tout"), args("Décider"))
	}
}

func keys(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
