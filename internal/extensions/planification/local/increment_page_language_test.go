// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// THE INCREMENT PAGE'S ACTIONS SPEAK THE GUI'S LANGUAGE (
// serving): the Increment tab's actions - the release select and Change release, Priority
// Put on hold and its form, Resume, Run in Claude Code and its modal, the More
// menu's Edit goals, Add a dependency, Take out, Drop it and their drawers and
// confirms - read French for a French person, no English word left.
func TestTheIncrementPagesActionsSpeakTheGUIsLanguage(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	inc := increments(t, w.ctx)
	page := func(lang string) string {
		t.Helper()
		def, err := inc.PanePage(w.ctx, api.IncrementPaneInput{ID: w.built, Tab: "page", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Increment tab must display: %v", err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	french := []string{`"aria-label":"Version"`, "Changer de version", `"aria-label":"Priorité"`, `"Suspendre"`, "Le suspendre", "Pourquoi il attend",
		"Lancer dans Claude Code", "Le lancer dans Claude Code", "Ouvrez Claude Code dans le projet et lancez :", "Chaque session traite les tâches",
		`"Plus"`, "Modifier les objectifs", "Ses objectifs de livraison", "Enregistrer les objectifs de livraison",
		"Ajouter une dépendance", "L'incrément qu'il attend", "L'attendre", `"Retirer"`, "Le retirer de sa version ? Il en attend une autre.",
		`"Abandonner"`, "Abandonner cet incrément ? Son travail n'est plus pris."}
	english := []string{`"aria-label":"Release"`, "Change release", `"aria-label":"Priority"`, "Put on hold", "Put it on hold", "Why it waits",
		"Run in Claude Code", "Run it in Claude Code", "Open Claude Code in the project", "Each session works",
		`"label":"More"`, "Edit goals", "Its delivery goals", "Save the delivery goals",
		"Add a dependency", "The increment it waits on", "Wait on it", "Take out", "Take it out of its release",
		"Drop it", "Drop this increment", "Mark it done", "Resume"}

	fr := page("fr")
	for _, s := range french {
		if !strings.Contains(fr, s) {
			t.Errorf("in French the Increment tab lacks %q: %s", s, fr)
		}
	}
	for _, s := range english {
		if strings.Contains(fr, s) {
			t.Errorf("in French the Increment tab still says %q", s)
		}
	}
	if en := page("en"); !strings.Contains(en, "Put on hold") || !strings.Contains(en, "Change release") || strings.Contains(en, "Suspendre") {
		t.Errorf("in English the Increment tab's actions are not English: %s", en)
	}

	// Held, it offers « Reprendre ».
	if _, err := inc.Hold(w.ctx, api.IncrementHoldInput{ID: w.built, Reason: "Waits on the bank", By: person}); err != nil {
		t.Fatal(err)
	}
	if held := page("fr"); !strings.Contains(held, `"Reprendre"`) || strings.Contains(held, "Resume") {
		t.Errorf("a held increment's Increment tab in French offers no « Reprendre »: %s", held)
	}
}
