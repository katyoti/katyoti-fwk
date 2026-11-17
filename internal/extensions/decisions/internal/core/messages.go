// SPDX-License-Identifier: Apache-2.0

package core

// The texts the Decision page writes for the person, in the languages the GUI
// speaks. A key missing in a language — or a language not listed — reads the
// English text. Data (the question, the labels, the reasoning) is never
// translated: it is in its author's language.
var msgs = map[string]map[string]string{
	"en": {
		"open":           "open",
		"applies":        "applies",
		"superseded":     "superseded",
		"retired":        "retired",
		"dropped":        "dropped",
		"proposal":       "Proposal",
		"noProposal":     "Not prepared yet: no option is proposed.",
		"proposed":       "proposed",
		"challenge":      "Challenged proposal",
		"notChallenged":  "Not challenged yet.",
		"challengedBy":   "Challenged by %s on %s",
		"preparation":    "Preparation",
		"options":        "Options",
		"noOptions":      "No option yet: the decision is not prepared.",
		"means":          "What it means",
		"pros":           "For",
		"cons":           "Against",
		"cost":           "Cost",
		"ifChosen":       "If chosen",
		"choose":         "Choose this option",
		"reason":         "Your reason",
		"cannotChoose":   "This label cannot be chosen here (it holds a page template): decide with the resolve command.",
		"decided":        "Decided",
		"chosen":         "chosen",
		"decidedBy":      "Decided by %s on %s",
		"decidedOn":      "Decided on %s",
		"why":            "Why",
		"closedNoChoice": "Closed without a choice.",
		"notOpenAnymore": "This decision is %s: nothing is chosen here anymore.",
		"agentOwned":     "An agent's decision",
		"agentOwnedWhy":  "An agent may take it: %s. The person may reopen it.",
	},
	"fr": {
		"open":           "ouverte",
		"applies":        "en vigueur",
		"superseded":     "remplacée",
		"retired":        "retirée",
		"dropped":        "abandonnée",
		"proposal":       "Proposition",
		"noProposal":     "Pas encore préparée : aucune option n'est proposée.",
		"proposed":       "proposée",
		"challenge":      "Proposition contestée",
		"notChallenged":  "Pas encore contestée.",
		"challengedBy":   "Contestée par %s le %s",
		"preparation":    "Préparation",
		"options":        "Options",
		"noOptions":      "Aucune option : la décision n'est pas préparée.",
		"means":          "Ce que cela signifie",
		"pros":           "Pour",
		"cons":           "Contre",
		"cost":           "Coût",
		"ifChosen":       "Si elle est choisie",
		"choose":         "Choisir cette option",
		"reason":         "Votre raison",
		"cannotChoose":   "Ce libellé ne peut pas être choisi ici (il contient un gabarit de page) : décidez par la commande resolve.",
		"decided":        "Décidée",
		"chosen":         "choisie",
		"decidedBy":      "Décidée par %s le %s",
		"decidedOn":      "Décidée le %s",
		"why":            "Pourquoi",
		"closedNoChoice": "Fermée sans choix.",
		"notOpenAnymore": "Cette décision est %s : on n'y choisit plus rien.",
		"agentOwned":     "Décision d'un agent",
		"agentOwnedWhy":  "Un agent peut la prendre : %s. La personne peut la rouvrir.",
	},
}

// tr is the text of key in lang, English when lang does not have it.
func tr(lang, key string) string {
	if s, ok := msgs[lang][key]; ok {
		return s
	}
	return msgs["en"][key]
}
