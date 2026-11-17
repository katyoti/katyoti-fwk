// SPDX-License-Identifier: Apache-2.0

package core

// The texts the Features page writes for the person, in the languages the GUI
// speaks. A key missing in a language — or a language not listed — reads the
// English text. Data (names, ids, states) is never translated.
var msgs = map[string]map[string]string{
	"en": {
		"features":      "Features",
		"byState":       "Features by state",
		"states":        "States",
		"all":           "All",
		"add":           "Add",
		"addFeature":    "Add a feature",
		"noFeatures":    "No features yet: what would deliver its goals?",
		"noneInState":   "No feature is in this state.",
		"orphans":       "%d serve no goal",
		"serves":        "serves",
		"servesNothing": "serves no goal",
		"openGoal":      "Open the goal",
		"cod":           "Cost of delay: what it costs not to have it yet — the higher, the sooner",
	},
	"fr": {
		"features":      "Fonctionnalités",
		"byState":       "Fonctionnalités par état",
		"states":        "États",
		"all":           "Toutes",
		"add":           "Ajouter",
		"addFeature":    "Ajouter une fonctionnalité",
		"noFeatures":    "Aucune fonctionnalité : qu'est-ce qui livrerait ses objectifs ?",
		"noneInState":   "Aucune fonctionnalité dans cet état.",
		"orphans":       "%d ne servent aucun objectif",
		"serves":        "sert",
		"servesNothing": "ne sert aucun objectif",
		"openGoal":      "Ouvrir l'objectif",
		"cod":           "Coût du retard : ce que coûte le fait de ne pas l'avoir encore — plus il est haut, plus c'est pressé",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
