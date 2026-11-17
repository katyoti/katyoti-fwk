// SPDX-License-Identifier: Apache-2.0

package core

// The texts the pane's pages (Contents, Objects) write for the person, in
// the languages the GUI speaks. A key missing in a language — or a language
// not listed — reads the English text. Data (names, ids, types, states) is
// never translated.
var msgs = map[string]map[string]string{
	"en": {
		"all":             "All",
		"types":           "Types",
		"states":          "States",
		"search":          "Search",
		"shownOf":         "%d of %d objects",
		"nothingHere":     "Nothing is filed under it yet.",
		"nothingMatches":  "Nothing matches.",
		"savedViews":      "Saved views",
		"fText":           "Text",
		"fTypes":          "Types",
		"fCapabilities":   "Capabilities",
		"fStates":         "States",
		"fIncoming":       "Pointed at by (edge kinds)",
		"fOutgoing":       "Pointing with (edge kinds)",
		"pickOrSearch":    "Pick a saved view, or search.",
		"results":         "Results",
		"firstOf":         "the first %d of %d",
		"quickText":       "A code or words",
		"quickTypes":      "Kinds",
		"inForce":         "Also narrowed by",
		"removeCriterion": "Click to remove it",
		"advanced":        "Advanced search",
		"bestMatch":       "Best match",
	},
	"fr": {
		"all":             "Tous",
		"types":           "Types",
		"states":          "États",
		"search":          "Rechercher",
		"shownOf":         "%d objets sur %d",
		"nothingHere":     "Rien n'est encore rangé dessous.",
		"nothingMatches":  "Aucun résultat.",
		"savedViews":      "Vues enregistrées",
		"fText":           "Texte",
		"fTypes":          "Types",
		"fCapabilities":   "Capacités",
		"fStates":         "États",
		"fIncoming":       "Visé par (types de liens)",
		"fOutgoing":       "Pointant par (types de liens)",
		"pickOrSearch":    "Choisissez une vue enregistrée, ou lancez une recherche.",
		"results":         "Résultats",
		"firstOf":         "les %d premiers sur %d",
		"quickText":       "Un code ou des mots",
		"quickTypes":      "Sortes",
		"inForce":         "Aussi restreint par",
		"removeCriterion": "Cliquez pour le retirer",
		"advanced":        "Recherche avancée",
		"bestMatch":       "Meilleure correspondance",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
