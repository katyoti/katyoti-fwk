// SPDX-License-Identifier: Apache-2.0

package core

// The texts the sign-off board writes for the person, in the languages the GUI
// speaks. A key missing in a language — or a language not listed — reads the
// English text. Data (ids, verdicts, the log) is never translated.
var msgs = map[string]map[string]string{
	"en": {
		"signoff":        "Sign-off",
		"noNotes":        "_No notes yet._",
		"approvals":      "Approvals",
		"saveVerdict":    "Save the verdict",
		"askSignoff":     "Ask for a sign-off",
		"approve":        "Approve",
		"approveAs":      "Approve %s as ${item.name}?",
		"reviewers":      "Reviewers",
		"noReviewers":    "Nobody reviews this object yet.",
		"open":           "Open %s",
		"contestedBy":    "Contested by",
		"noContest":      "Nothing contests this object.",
		"newItem":        "New sign-off item",
		"contestAnother": "Contest another item",
		"contest":        "Contest",
		"activity":       "Activity",
		"noActivity":     "No write recorded yet.",
	},
	"fr": {
		"signoff":        "Validation",
		"noNotes":        "_Pas encore de notes._",
		"approvals":      "Approbations",
		"saveVerdict":    "Enregistrer le verdict",
		"askSignoff":     "Demander une validation",
		"approve":        "Approuver",
		"approveAs":      "Approuver %s en tant que ${item.name} ?",
		"reviewers":      "Relecteurs",
		"noReviewers":    "Personne ne relit encore cet objet.",
		"open":           "Ouvrir %s",
		"contestedBy":    "Contesté par",
		"noContest":      "Rien ne conteste cet objet.",
		"newItem":        "Nouvel élément à valider",
		"contestAnother": "Contester un autre élément",
		"contest":        "Contester",
		"activity":       "Activité",
		"noActivity":     "Aucune écriture enregistrée pour l'instant.",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
