// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// THE ANSWERS OF THE INBOX'S OWN QUESTIONS - what answers a human task or an
// entry, by its kind, in the person's language. « À faire » is planification's
// page reused: it draws these nodes as they are in its
// cards, and so does a chat's inline card (chat.go); the inbox
// has no page of its own any more.

// The verbs the inbox's own cards run.
const (
	inboxAnswerURI  = "ktw-agents:inbox/answer"
	inboxDismissURI = "ktw-agents:inbox/dismiss"
	todosReportURI  = "ktw-agents:todos/report"
	todosPullURI    = "ktw-agents:todos/pull"
)

// The labels of the answers, by language.
var labels = map[string]map[string]string{
	"en": {
		"title": "« À faire » · %s", "title_bare": "« À faire »", "waiting": "%d waiting", "mine": "Mine",
		"list": "Holding list %s · members: %s", "nothing": "Nothing of this kind.", "recent": "Recent",
		"due": "due", "overdue": "overdue", "proof": "proof", "answerable": "answerable by", "taken": "taken by %s",
		"about": "about %s", "settles": "settles: %s", "open": "Open", "take": "Take it", "approve": "Approve",
		"reject": "Reject", "send": "Send", "dismiss": "Dismiss", "empty": "Nothing waits on you.",
		"filter_label": "Filter by kind", "filter_hint": "Click a count to show only that kind.",
		"agent": "agent %s", "decision": "Your decision", "note": "Your note", "answer": "Your answer",
		"choice": "Your choice", "form_asked": "The form asked:",
		"choose": "Choose", "all_good": "All good: I validate", "failed": "Something failed", "passkey": " (passkey)",
		"why": "Why?", "what_failed": "What failed?", "rule": "The rule an always keeps", "form_given": "Form answered",
		"conversations": "Conversations", "chat": "Chat · %s", "chat_kind": "chat", "waiting_you": "waiting on you",
		"running": "running", "last": "last activity %s", "last_word": "last: « %s »", "open_chat": "Open the chat",
		"answered": "answered by %s · %s", "relayed": "relayed by %s, confirmed by %s",
		"h_decision": "Allow or deny the act - once or always",
		"h_rule":     "The rule an always keeps: one the question proposes or one of the person's own",
		"all":        "All", "h_recent": "For information: nothing to answer",
	},
	"fr": {
		"title": "« À faire » · %s", "title_bare": "« À faire »", "waiting": "%d en attente", "mine": "Pour moi",
		"list": "Liste %s · membres : %s", "nothing": "Rien de ce genre.", "recent": "Récent",
		"due": "échéance", "overdue": "en retard", "proof": "preuve", "answerable": "peut répondre", "taken": "pris par %s",
		"about": "sur %s", "settles": "règle : %s", "open": "Ouvrir", "take": "Je le prends", "approve": "Approuver",
		"reject": "Refuser", "send": "Envoyer", "dismiss": "Écarter", "empty": "Rien ne vous attend.",
		"filter_label": "Filtrer par genre", "filter_hint": "Cliquez un compte pour ne voir que ce genre.",
		"agent": "agent %s", "decision": "Votre décision", "note": "Votre note", "answer": "Votre réponse",
		"choice": "Votre choix", "form_asked": "Le formulaire demandé :",
		"choose": "Choisir", "all_good": "Tout est bon : je valide", "failed": "Quelque chose a échoué", "passkey": " (clé d'accès)",
		"why": "Pourquoi ?", "what_failed": "Qu'est-ce qui a échoué ?", "rule": "La règle qu'un « toujours » garde", "form_given": "Formulaire rempli",
		"conversations": "Conversations", "chat": "Chat · %s", "chat_kind": "chat", "waiting_you": "vous attend",
		"running": "en cours", "last": "dernière activité %s", "last_word": "dernier mot : « %s »", "open_chat": "Ouvrir le chat",
		"answered": "répondu par %s · %s", "relayed": "relayé par %s, confirmé par %s",
		"h_decision": "Autoriser ou refuser l'acte - une fois ou toujours",
		"h_rule":     "La règle qu'un « toujours » garde : une que la question propose ou une des vôtres",
		"all":        "Tout", "h_recent": "Pour information : rien à répondre",
	},
}

// kindLabels are the built-in kinds' labels in French; a kind a pack adds
// shows its own label.
var kindLabels = map[string]map[string]string{
	"fr": {
		api.KindApprove: "Validation", api.KindDecide: "Décision", api.KindTestByHand: "Test à la main",
		api.KindForm: "Réponse à donner", api.KindProvide: "Données à fournir", api.KindChoose: "Choix",
		api.KindPermission: "Permission", api.KindUnblock: "Déblocage", api.KindAccept: "Acceptation",
		KindOverdue: "en retard", api.KindConversation: "conversations",
	},
}

// L is a label of the page in the person's language.
func L(lang, key string) string {
	if l, ok := labels[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return labels["en"][key]
}

// KindLabel is a kind's label in the person's language: its own otherwise.
func KindLabel(lang, kind, own string) string {
	if s := kindLabels[lang][kind]; s != "" {
		return s
	}
	if own != "" {
		return own
	}
	return kind
}

type node = render.Node

// textForm is an answer to give.
type textForm struct {
	Note string `json:"note" jsonschema:"description=Your answer,required"`
}

// choiceForm is a choice among a question's options, with a note: a
// decision says why.
type choiceForm struct {
	Choice string `json:"choice" jsonschema:"description=Your choice,required"`
	Note   string `json:"note,omitempty" jsonschema:"description=Your note"`
}

// verdictKinds are the kinds answered by a verdict.
var verdictKinds = map[string]bool{
	api.KindApprove: true, api.KindAccept: true, api.KindPermission: true, api.KindTestByHand: true, api.KindUnblock: true,
}

// inboxControls answer the inbox's own items: a human task by its to-do's
// report, through its card's answer area (humanTaskAnswer); an entry by the
// inbox's Answer - a verdict as the card draws one; a notice by Dismiss.
func inboxControls(it api.WaitingItem, lang string) []node {
	switch {
	case it.Kind == kindNotice:
		if it.Entry == "" {
			return nil
		}
		return []node{{"kind": "list", "data": []node{{"id": it.Entry}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "dismiss", "label": L(lang, "dismiss"), "icon": "x", "uri": inboxDismissURI, "args": node{"id": "${item.id}"}}}}}
	case it.Todo != "":
		return humanTaskAnswer(it, lang)
	case it.Entry != "":
		if it.Kind == api.KindChoose {
			return nil // chosen on the decision it is about: « Ouvrir »
		}
		if it.Kind == api.KindForm {
			return []node{render.AnswerForm(render.InlineSchema[textForm]("Answer", map[string]string{"note": L(lang, "answer")}),
				render.RunURI(L(lang, "send"), inboxAnswerURI, map[string]any{"id": it.Entry, "decision": "approved", "note": "${value.note}", "by": "${person.id}"}),
				"send", textForm{}).With("id", "answer:"+it.Entry)}
		}
		return []node{verdictButtons(it.Entry, lang, "approve", "reject", "why", it.Proof,
			render.RunURI(L(lang, "approve"), inboxAnswerURI, map[string]any{"id": it.Entry, "decision": "approved", "by": "${person.id}"}),
			render.RunURI(L(lang, "reject"), inboxAnswerURI, map[string]any{"id": it.Entry, "decision": "rejected", "note": "${value.note}", "by": "${person.id}"}))}
	}
	return nil
}
