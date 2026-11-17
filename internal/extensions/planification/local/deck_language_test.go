// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE PLANNING DECK SPEAKS THE GUI'S LANGUAGE: the
// deck's title, its tabs and its Overview - the state tiles, the slim By
// phase header (its heading and gauge text), Running
// now, the current releases - are in the person's language.
func TestThePlanningDeckSpeaksTheGUIsLanguage(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	for i := range 12 {
		file(t, ctx, req, api.FileInput{Title: fmt.Sprintf("Fix invoice rounding %d", i), Module: "billing", Kind: api.KindFix, Increment: in.ID})
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	deck := func(lang string) string {
		t.Helper()
		def, err := views.Deck(ctx, api.DeckInput{Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(def.DSLTree); err != nil {
			t.Fatalf("the deck is off the page grammar: %v", err)
		}
		tabs := def.DSLTree.(map[string]any)["tabs"].([]map[string]any)
		labels := make([]string, 0, len(tabs))
		overview := ""
		for _, p := range tabs {
			labels = append(labels, fmt.Sprint(p["label"]))
			if p["id"] == "overview" {
				raw, _ := json.Marshal(p["content"])
				overview = string(raw)
			}
		}
		return def.Title + " | " + strings.Join(labels, ", ") + " | " + overview
	}
	french := []string{"Planification — tous les modules", "Vue d'ensemble", "Par état", "Par phase", "Par module", "Demandes", "À une personne", "Livraisons", "Historique", "Groupes de tâches",
		`"label":"Ouvertes"`, "demandes non terminées", `"label":"Prêtes"`, `"label":"En cours"`, `"label":"En attente"`, `"label":"Bloquées"`, "Jour/Semaine/Mois",
		`"text":"Par phase"`, "{value} ouvertes", `"label":"construction"`,
		"En cours maintenant", "Rien ne tourne.", "Versions en cours", "Aucune version en cours."}
	english := []string{"Planification — every module", "Overview", "By state", "Needs a person", "Task groups",
		`"label":"Open"`, "requests not ended", "Day/Week/Month", `"text":"By phase"`, "{value} open",
		"Running now", "Nothing runs now.", "Current releases", "No current release."}

	en := deck("")
	for _, w := range english {
		if !strings.Contains(en, w) {
			t.Errorf("in English the deck lacks %q", w)
		}
	}
	fr := deck("fr")
	for _, w := range french {
		if !strings.Contains(fr, w) {
			t.Errorf("in French the deck lacks %q: %s", w, fr)
		}
	}
	for _, w := range []string{"Overview", "requests not ended", "Running now", "Current releases", "No current release.", "Nothing runs now."} {
		if strings.Contains(fr, w) {
			t.Errorf("in French the deck still says %q", w)
		}
	}
}
