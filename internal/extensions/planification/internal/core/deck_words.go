// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"
)

// THE PLANNING DECK'S WORDS - its title, its tabs and
// its Overview in the person's language.

// deckWords are the deck's words, per language.
var deckWords = map[string]map[string]string{
	"en": {
		"title_all": "Planification — every module", "title_of": "Planification — %s",
		"tab_overview": "Overview", "tab_by_state": "By state", "tab_by_phase": "By phase", "tab_by_module": "By module",
		"tab_requests": "Requests", "tab_needs": "Needs a person", "tab_deliveries": "Deliveries", "tab_history": "History", "tab_groups": "Task groups",
		"tile_open": "Open", "tile_open_hint": "requests not ended",
		"tile_ready": "Ready", "tile_ready_hint": "a step may be pulled now",
		"tile_running": "Running", "tile_running_hint": "a step is pulled",
		"tile_waiting": "Waiting", "tile_waiting_hint": "an answer, or what comes before",
		"tile_blocked": "Blocked", "tile_blocked_hint": "a step failed: a person looks",
		"tile_done": "Day/Week/Month", "tile_done_hint": "ended done today, this week, this month",
		"phase_open": "{value} open", "phase_requests": "requests", "phase_inputs": "inputs", "phase_preparation": "preparation",
		"phase_build": "build", "phase_verify": "verify", "phase_release": "release",
		"running_now": "Running now", "running_now_hint": "The steps pulled now: who holds each, since when; a lapsed claim can be released", "running_none": "Nothing runs now.",
		"current_releases": "Current releases", "current_releases_hint": "The increments of each stream's current release, by priority", "current_none": "No current release.",
		"needs_tile": "Needs you", "needs_open": "Open To do", "needs_none": "Nothing waits on you",
		"needs_unblock": "%d to unblock", "needs_decide": "%d to decide", "needs_validate": "%d to validate", "needs_answer": "%d awaiting an answer", "needs_test": "%d to test by hand",
	},
	"fr": {
		"title_all": "Planification — tous les modules", "title_of": "Planification — %s",
		"tab_overview": "Vue d'ensemble", "tab_by_state": "Par état", "tab_by_phase": "Par phase", "tab_by_module": "Par module",
		"tab_requests": "Demandes", "tab_needs": "À une personne", "tab_deliveries": "Livraisons", "tab_history": "Historique", "tab_groups": "Groupes de tâches",
		"tile_open": "Ouvertes", "tile_open_hint": "demandes non terminées",
		"tile_ready": "Prêtes", "tile_ready_hint": "une étape peut être prise",
		"tile_running": "En cours", "tile_running_hint": "une étape est prise",
		"tile_waiting": "En attente", "tile_waiting_hint": "une réponse, ou ce qui vient avant",
		"tile_blocked": "Bloquées", "tile_blocked_hint": "une étape a échoué : une personne regarde",
		"tile_done": "Jour/Semaine/Mois", "tile_done_hint": "terminées aujourd'hui, cette semaine, ce mois-ci",
		"phase_open": "{value} ouvertes", "phase_requests": "demandes", "phase_inputs": "entrées", "phase_preparation": "préparation",
		"phase_build": "construction", "phase_verify": "vérification", "phase_release": "livraison",
		"running_now": "En cours maintenant", "running_now_hint": "Les étapes prises : qui tient chacune, depuis quand ; une prise expirée peut être libérée", "running_none": "Rien ne tourne.",
		"current_releases": "Versions en cours", "current_releases_hint": "Les incréments de la version en cours de chaque flux, par priorité", "current_none": "Aucune version en cours.",
		"needs_tile": "À vous", "needs_open": "Ouvrir À faire", "needs_none": "Rien ne vous attend",
		"needs_unblock": "%d à débloquer", "needs_decide": "%d à décider", "needs_validate": "%d à valider", "needs_answer": "%d en attente de réponse", "needs_test": "%d à tester à la main",
	},
}

// deckWord is a word of the deck in lang.
func deckWord(lang, key string) string {
	if l, ok := deckWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return deckWords["en"][key]
}

// deckWordf is deckWord with its values in it.
func deckWordf(lang, key string, a ...any) string {
	return fmt.Sprintf(deckWord(lang, key), a...)
}

// deckPhase is a phase's name on the By phase bar: lang's word, else its
// lower-cased title.
func deckPhase(lang, ph string) string {
	if w := deckWord(lang, "phase_"+ph); w != "" {
		if _, ok := deckWords[lang]; ok {
			return w
		}
	}
	return strings.ToLower(phaseTitle(ph))
}
