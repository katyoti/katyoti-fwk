// SPDX-License-Identifier: Apache-2.0

package core

// The texts the Goals page writes for the person, in the languages the GUI
// speaks. A key missing in a language — or a language not listed — reads the
// English text. Data (names, ids, states) is never translated.
var msgs = map[string]map[string]string{
	"en": {
		"goals":         "Goals",
		"byState":       "Goals by state",
		"states":        "States",
		"all":           "All",
		"add":           "Add",
		"addGoal":       "Add a goal",
		"noGoals":       "No goals yet: say why this exists.",
		"noneInState":   "No goal is in this state.",
		"uncovered":     "%d served by nothing",
		"nothingServes": "nothing serves it",
		"servedBy":      "served by %d",
		"asymptotic":    "asymptotic",
		"closable":      "closable",
		"reachability":  "Reachability: asymptotic is pursued, never done; closable can be reached",
	},
	"fr": {
		"goals":         "Objectifs",
		"byState":       "Objectifs par état",
		"states":        "États",
		"all":           "Tous",
		"add":           "Ajouter",
		"addGoal":       "Ajouter un objectif",
		"noGoals":       "Aucun objectif : dites pourquoi ceci existe.",
		"noneInState":   "Aucun objectif dans cet état.",
		"uncovered":     "%d servis par rien",
		"nothingServes": "rien ne le sert",
		"servedBy":      "servi par %d",
		"asymptotic":    "asymptotique",
		"closable":      "atteignable",
		"reachability":  "Atteignabilité : un objectif asymptotique se poursuit sans fin ; un objectif atteignable peut être atteint",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
