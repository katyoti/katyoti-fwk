// SPDX-License-Identifier: Apache-2.0

package core

// The texts What's Next writes for the person, in the languages the GUI
// speaks. A key missing in a language — or a language not listed — reads the
// English text. Data (titles, ids, kinds, statuses) is never translated.
var msgs = map[string]map[string]string{
	"en": {
		"noCriteria":       "No acceptance criteria in this scope yet.",
		"release":          "Release or increment",
		"allWork":          "All the work",
		"narrow":           "Show",
		"criteria":         "Acceptance criteria",
		"segPass":          "pass",
		"segFailing":       "failing",
		"segNoTest":        "no test",
		"segUnproven":      "could not run",
		"passOf":           "%d / %d pass",
		"andMore":          "and %d more",
		"openRequest":      "Open the request",
		"question":         "question",
		"from":             "from %s",
		"requestsToTriage": "Requests to triage",
		"allRequests":      "All requests",
		"untitledGoal":     "(goal)",
		"featuresOf":       "%d/%d features",
		"generateFeatures": "Generate features",
		"openGoal":         "Open the goal",
		"goals":            "Goals",
		"delivered":        "delivered",
		"breakDown":        "Break down",
		"openFeature":      "Open the feature",
		"features":         "Features",
		"noFeatures":       "No features in this scope.",
		"openCriterion":    "Open the criterion",
		"looseCriteria":    "Criteria under no feature",
		"alignment":        "Alignment",
		"run":              "Run",
		"runKind":          "Run %s",
		"whatsNext":        "What's next",
		"allGreen":         "Nothing pending — all green.",
	},
	"fr": {
		"noCriteria":       "Aucun critère d'acceptation dans ce périmètre pour l'instant.",
		"criteria":         "Critères d'acceptation",
		"segPass":          "réussis",
		"segFailing":       "en échec",
		"segNoTest":        "sans test",
		"segUnproven":      "non exécutables",
		"passOf":           "%d / %d réussis",
		"andMore":          "et %d de plus",
		"openRequest":      "Ouvrir la demande",
		"question":         "question",
		"from":             "de %s",
		"requestsToTriage": "Demandes à trier",
		"allRequests":      "Toutes les demandes",
		"untitledGoal":     "(objectif)",
		"featuresOf":       "%d/%d fonctionnalités",
		"generateFeatures": "Générer les fonctionnalités",
		"openGoal":         "Ouvrir l'objectif",
		"goals":            "Objectifs",
		"delivered":        "livrée",
		"breakDown":        "Décomposer",
		"openFeature":      "Ouvrir la fonctionnalité",
		"release":          "Version ou incrément",
		"allWork":          "Tout le travail",
		"narrow":           "Afficher",
		"features":         "Fonctionnalités",
		"noFeatures":       "Aucune fonctionnalité dans ce périmètre.",
		"openCriterion":    "Ouvrir le critère",
		"looseCriteria":    "Critères hors fonctionnalité",
		"alignment":        "Alignement",
		"run":              "Lancer",
		"runKind":          "Lancer %s",
		"whatsNext":        "Et ensuite ?",
		"allGreen":         "Rien en attente — tout est vert.",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
