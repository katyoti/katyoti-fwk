// SPDX-License-Identifier: Apache-2.0

package core

// The texts the Challenged by page writes for the person, in the languages
// the GUI speaks. A key missing in a language — or a language not listed —
// reads the English text. Data (names, ids, states) is never translated.
var msgs = map[string]map[string]string{
	"en": {
		"challengedBy": "Challenged by",
		"why":          "Each one disputes this object: open it to answer the doubt",
		"none":         "Nothing challenges it.",
		"unresolved":   "%d not in this project",
	},
	"fr": {
		"challengedBy": "Contesté par",
		"why":          "Chacun conteste cet objet : ouvrez-le pour répondre au doute",
		"none":         "Rien ne le conteste.",
		"unresolved":   "%d hors de ce projet",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
