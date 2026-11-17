// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE « À FAIRE » PAGE — the person's queue as one page of the page DSL's
// generic nodes: a tile per action, then a section per action — its heading
// is the action — with a card per item saying what is expected, what is
// already proven and carrying the buttons doing it; the notices apart, last.
// A tile is a filter: a click re-reads the page with its action — only that
// action's section then shows, every tile keeping its count, the chosen tile
// marked —; « Tout », or the chosen tile again, shows everything. What the
// person put on hold has its own tile, « En attente », last: it is shown only
// when that tile is chosen, never under « Tout » nor in its count.
// Acting on a card runs the verb that moves the work on: the Inbox's Answer, a
// hand test (requests/test), a decision's Resolve, a retry, a resume.

// projectMarkID is the element id of a card's project badge: the walk finds
// it as data-element="badge:project-mark", its key in data-project
// (app/ktw-gui/frontend/tests/live/project-mark.sh).
const projectMarkID = "project-mark"

// The verbs the page's cards run.
const (
	requestTestURI   = "ktw-agents:requests/test"
	decisionResolve  = "ktw-methods:decisions/resolve"
	inboxDismissURI  = "ktw-agents:inbox/dismiss"
	requestHoldURI   = "ktw-agents:requests/hold"
	requestUnholdURI = "ktw-agents:requests/unhold"
	heldSection      = "held"
	allTile          = "all"
	personQueueTitle = "queue_title"
)

// The page's labels, by language.
var queueLabels = map[string]map[string]string{
	"en": {
		allTile: "All", "h_" + allTile: "Everything waiting on you", "empty_action": "Nothing of this kind waits on you.",
		"no_release": "In no release", "no_increment": "outside any increment",
		heldSection: "On hold", "h_" + heldSection: "What you put aside, out of the count: choose it to take an item back",
		"hold": "Put on hold", "unhold": "Take it back", "held_by": "On hold by %s since %s",
		personQueueTitle: "To do", "empty": "Nothing waits on you.",
		api.ActionDecide: "Decide", api.ActionTest: "Test by hand", api.ActionValidate: "Validate",
		api.ActionAnswer: "Answer", api.ActionUnblock: "Unblock", api.ActionNotice: "For information",
		"h_" + api.ActionDecide:   "A choice only you make: pick one option, saying why",
		"h_" + api.ActionTest:     "What no agent can test: follow the steps, then say whether it passes or how it fails",
		"h_" + api.ActionValidate: "A result to judge: accept it, or send it back saying what to change",
		"h_" + api.ActionAnswer:   "A question asked to you: your answer goes to the work waiting on it",
		"h_" + api.ActionUnblock:  "A step failed or a request is blocked: try it again, resume it or drop it",
		"h_" + api.ActionNotice:   "Nothing to do: for your information",
		"done":                    "What was done", "challenged": "Challenged", "proven": "Already proven", "how": "How to test it", "proposed": "proposed", "open": "Open",
		"opt_remove_the_wait": "Remove the wait", "opt_drop_the_request": "Drop the request", "opt_plan_the_dependency": "Plan the dependency",
		"accept": "Accept", "send_back": "Send back…", "retry": "Try its step again", "checks": "Run the checks",
		"drop": "Drop", "accept_all": "Accept them all", "drop_confirm": "Drop it? It is not pursued.",
		"resume": "Resume", "dismiss": "Dismiss", "choose": "Choose", "send": "Send", "record": "Record",
		"means": "What it means", "pros": "For", "cons": "Against", "cost": "Cost", "if_chosen": "If chosen",
		"reject": "Reject", "criteria": "Acceptance criteria", "no_criteria": "No acceptance criterion: nothing proves it yet.",
		"verdict_pass": "passed", "verdict_fail": "failed", "verdict_unproven": "never run",
		api.ActionConversation: "Conversations", "h_" + api.ActionConversation: "The chats running in your sessions: open one to follow it",
		"asked_by": "Asked by %s", "options": "The options", "open_chat": "Open the chat",
		"conv_waiting": "waiting on you", "conv_running": "running", "since": "since %s",
		"unread":       "The inbox's questions could not be read: %s - they are not listed here.",
		"title_person": "« À faire » · %s", "n_waiting": "%d waiting", "mine": "Mine", "holding_list": "Holding list %s · members: %s",
		"agent_member": "agent %s", "due": "due %s", "overdue_on": "overdue %s", "proof": "proof: %s", "answerable": "answerable by %s",
		"taken": "taken by %s", "settles": "settles: %s", overdueKind: "overdue", "kinds": "By kind",
	},
	"fr": {
		allTile: "Tout", "h_" + allTile: "Tout ce qui vous attend", "empty_action": "Rien de ce genre ne vous attend.",
		"no_release": "Dans aucune version", "no_increment": "hors de tout incrément",
		heldSection: "En attente", "h_" + heldSection: "Ce que vous avez mis de côté, hors du compte : choisissez-la pour reprendre un élément",
		"hold": "Mettre en attente", "unhold": "Reprendre", "held_by": "En attente par %s depuis %s",
		personQueueTitle: "À faire", "empty": "Rien ne vous attend.",
		api.ActionDecide: "Décider", api.ActionTest: "Tester à la main", api.ActionValidate: "Valider",
		api.ActionAnswer: "Répondre", api.ActionUnblock: "Débloquer", api.ActionNotice: "Pour information",
		"h_" + api.ActionDecide:   "Un choix qui n'appartient qu'à vous : retenez une option, en disant pourquoi",
		"h_" + api.ActionTest:     "Ce qu'aucun agent ne peut tester : suivez les étapes, puis dites si ça passe ou comment ça échoue",
		"h_" + api.ActionValidate: "Un résultat à juger : acceptez-le, ou renvoyez-le en disant quoi changer",
		"h_" + api.ActionAnswer:   "Une question qui vous est posée : votre réponse va au travail qui l'attend",
		"h_" + api.ActionUnblock:  "Une étape a échoué ou une demande est bloquée : relancez-la, reprenez-la ou abandonnez-la",
		"h_" + api.ActionNotice:   "Rien à faire : pour votre information",
		"done":                    "Ce qui a été fait", "challenged": "Contesté", "proven": "Déjà prouvé", "how": "Comment le tester", "proposed": "proposée", "open": "Ouvrir",
		"opt_remove_the_wait": "Retirer l'attente", "opt_drop_the_request": "Abandonner la demande", "opt_plan_the_dependency": "Planifier la dépendance",
		"accept": "Accepter", "send_back": "Renvoyer…", "retry": "Relancer l'étape", "checks": "Lancer les vérifications",
		"drop": "Abandonner", "accept_all": "Tout accepter", "drop_confirm": "L'abandonner ? Il ne sera pas poursuivi.",
		"resume": "Reprendre", "dismiss": "Écarter", "choose": "Choisir", "send": "Envoyer", "record": "Enregistrer",
		"means": "Ce que cela signifie", "pros": "Pour", "cons": "Contre", "cost": "Coût", "if_chosen": "Si elle est choisie",
		"reject": "Rejeter", "criteria": "Critères d'acceptation", "no_criteria": "Aucun critère d'acceptation : rien ne le prouve encore.",
		"verdict_pass": "réussi", "verdict_fail": "échoué", "verdict_unproven": "jamais lancé",
		api.ActionConversation: "Conversations", "h_" + api.ActionConversation: "Les conversations en cours dans vos sessions : ouvrez-en une pour la suivre",
		"asked_by": "Demandé par %s", "options": "Les options", "open_chat": "Ouvrir le chat",
		"conv_waiting": "vous attend", "conv_running": "en cours", "since": "depuis le %s",
		"unread":       "Les questions de la boîte de réception n'ont pas pu être lues : %s - elles ne sont pas listées ici.",
		"title_person": "« À faire » · %s", "n_waiting": "%d en attente", "mine": "Pour moi", "holding_list": "Liste %s · membres : %s",
		"agent_member": "agent %s", "due": "échéance %s", "overdue_on": "en retard depuis le %s", "proof": "preuve : %s", "answerable": "peut répondre : %s",
		"taken": "pris par %s", "settles": "règle : %s", overdueKind: "en retard", "kinds": "Par genre",
	},
}

func ql(lang, key string) string {
	if l, ok := queueLabels[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return queueLabels["en"][key]
}

// The action's look: its icon and color.
var queueLook = map[string][2]string{
	api.ActionDecide: {"arrows-split", "grape"}, api.ActionTest: {"hand-finger", "orange"},
	api.ActionValidate: {"circle-check", "teal"}, api.ActionAnswer: {"message-question", "blue"},
	api.ActionUnblock: {"alert-octagon", "red"}, api.ActionNotice: {"info-circle", "gray"},
	heldSection: {"player-pause", "gray"}, allTile: {"list-check", "indigo"},
	api.ActionConversation: {"messages", "indigo"},
}

// askedLine is who asks an inbox question and since when: its asker, else
// the module it was filed in; "" with neither.
func askedLine(it api.PersonItem, lang string) string {
	by := it.AskedBy
	if by == "" {
		by = it.Module
	}
	if by == "" {
		return ""
	}
	line := fmt.Sprintf(ql(lang, "asked_by"), by)
	if len(it.Since) >= 10 {
		line += " · " + fmt.Sprintf(ql(lang, "since"), it.Since[:10])
	}
	return line
}

// formWords are the answer forms' field labels and hints in French, by the
// form's root and field: in English a form keeps its fields' own words.
var formWords = map[string]map[string][2]string{
	"ApproveOrSendBack": {"decision": {"Votre décision", "approuvé : il continue ; refusé : il est repris avec votre note"},
		"note": {"Votre note", "Ce qu'il faut changer : requis quand vous le renvoyez"}},
	"HandTest":          {"result": {"Résultat", "réussi ou échoué"}, "note": {"Ce que vous avez vu", "Ce que vous avez vu : requis quand il échoue"}},
	"ChooseOption":      {"reason": {"Pourquoi", "Pourquoi cette option"}},
	"RejectIt":          {"reason": {"Pourquoi", "Ce qui manque ou ne va pas"}},
	"PutOnHold":         {"reason": {"Pourquoi il attend", "Pourquoi il attend (facultatif)"}},
	"AnswerTheQuestion": {"note": {"Votre réponse", "Votre réponse"}},
}

// qschema is T's inline schema with its fields' words in the person's
// language: in French the formWords of its root, labels and hints; in
// English the labels given and its fields' own hints.
func qschema[T any](root, lang string, labels map[string]string) map[string]any {
	words, fr := formWords[root]
	if lang != "fr" || !fr {
		return render.InlineSchema[T](root, labels)
	}
	l, hints := map[string]string{}, map[string]string{}
	for field, w := range words {
		l[field], hints[field] = w[0], w[1]
		if w[1] == w[0] {
			hints[field] = "" // the label says it: no hint repeating it
		}
	}
	return render.DescribeFields(render.InlineSchema[T](root, l), hints)
}

// holdForm is why an item waits.
type holdForm struct {
	Reason string `json:"reason,omitempty" jsonschema:"description=Why it waits (optional)"`
}

// testForm is a hand test's result.
type testForm struct {
	Result string `json:"result" jsonschema:"description=passes or fails,enum=passes|fails,required"`
	Note   string `json:"note,omitempty" jsonschema:"description=What you saw: required when it fails"`
}

// chooseForm is why an option is chosen.
type chooseForm struct {
	Reason string `json:"reason,omitempty" jsonschema:"description=Why this option"`
}

// PersonQueuePage answers the « À faire » page.
// The counts asked while it computes the queue take its computation; its
// answer carries its count (data.count) - the rail's badge needs no second
// read while the page is shown - and its forms already resolved
func (v *Views) PersonQueuePage(ctx context.Context, in api.PersonQueueInput) (render.Def, error) {
	q, err := sharedPersonQueue(ctx, in, false)
	if err != nil {
		return render.Def{}, err
	}
	lang := in.Language
	by := map[string][]api.PersonItem{}
	for _, it := range q.Items {
		if it.Held != nil {
			by[heldSection] = append(by[heldSection], it)
			continue
		}
		by[it.Action] = append(by[it.Action], it)
	}
	filter := queueFilter(in, by)
	tiles := []node{queueTile(lang, allTile, q.Count, filter)}
	for _, a := range api.PersonActions {
		tiles = append(tiles, queueTile(lang, a, q.ByAction[a], filter))
	}
	// The conversations: a tile
	// and a section of their own when there are some, never counted.
	if len(by[api.ActionConversation]) > 0 {
		tiles = append(tiles, queueTile(lang, api.ActionConversation, len(by[api.ActionConversation]), filter))
	}
	tiles = append(tiles, queueTile(lang, heldSection, len(by[heldSection]), filter))
	children := []node{{"kind": "grid", "cols": len(tiles), "gap": "md", "children": tiles}}
	// Whose queue and how many wait above the tiles, a compact chip per kind of question under
	// them - a click keeps that kind, a second click all.
	children = append([]node{queueHeader(q, lang)}, children...)
	if chips := kindChips(q, in.Kind, lang); chips != nil {
		children = append(children[:2], append([]node{chips}, children[2:]...)...)
	}
	// The inbox's questions could not be read: said, never silently fewer
	//
	if q.Unread != "" {
		children = append(children, node{"kind": "text", "id": "unread", "content": fmt.Sprintf(ql(lang, "unread"), q.Unread), "size": "sm", "color": "orange"})
	}
	// « Tout »: every action and the notices — what is on hold shows only
	// under its own tile.
	shown := append(append([]string{}, api.PersonActions...), api.ActionNotice, api.ActionConversation)
	if filter != "" {
		// One tile chosen: its section alone — the notices are under « Tout ».
		shown = []string{filter}
	}
	for _, a := range shown {
		items := by[a]
		if len(items) == 0 {
			continue
		}
		// The focused item (the Inbox opening its card) first.
		for i, it := range items {
			if in.Focus != "" && (it.Key == in.Focus || it.Object == in.Focus || it.Ask == in.Focus) && i > 0 {
				items = append([]api.PersonItem{it}, append(items[:i:i], items[i+1:]...)...)
				break
			}
		}
		var cards []node
		if in.Focus == "" {
			items = byPlace(items)
		}
		place := "-"
		for _, it := range items {
			// By release then increment: a line names each group (F-055).
			if p := placeOf(it, lang); in.Focus == "" && p != place {
				place = p
				cards = append(cards, node{"kind": "text", "content": p, "size": "xs", "fw": 600, "color": "dimmed"})
			}
			cards = append(cards, queueCard(it, lang, in.Focus))
		}
		look := queueLook[a]
		sec := section(fmt.Sprintf("%s — %d", ql(lang, a), len(items)), look[0], look[1], ql(lang, "h_"+a),
			node{"kind": "vbox", "gap": "sm", "children": cards})
		// Its address (data-section): the screen's walks find each section
		// by it.
		sec["data-section"] = a
		children = append(children, sec)
	}
	switch {
	case len(q.Items) == 0:
		children = append(children, node{"kind": "text", "content": ql(lang, "empty"), "size": "sm", "color": "dimmed"})
	case filter != "" && len(by[filter]) == 0:
		children = append(children, node{"kind": "text", "content": ql(lang, "empty_action"), "size": "sm", "color": "dimmed"})
	}
	return render.Def{Kind: "page", Title: ql(lang, personQueueTitle),
		DSLTree: selfContained(node{"kind": "vbox", "gap": "lg", "children": children}),
		Data:    map[string]any{"count": q.Count}}, nil
}

// byPlace orders one action's items by release then increment - the ones in
// no release last, those outside any increment last within theirs - keeping
// their own order within a group.
func byPlace(items []api.PersonItem) []api.PersonItem {
	out := slices.Clone(items)
	key := func(title, id string) string {
		if id == "" {
			return "\uffff"
		}
		return title + "\x00" + id
	}
	slices.SortStableFunc(out, func(a, b api.PersonItem) int {
		// The person's own first, then each holding list.
		if c := strings.Compare(holderKey(a), holderKey(b)); c != 0 {
			return c
		}
		if c := strings.Compare(key(a.ReleaseTitle, a.Release), key(b.ReleaseTitle, b.Release)); c != 0 {
			return c
		}
		return strings.Compare(key(a.IncrementTitle, a.Increment), key(b.IncrementTitle, b.Increment))
	})
	return out
}

// placeOf names an item's group: its release, then its increment.
func placeOf(it api.PersonItem, lang string) string {
	// Who holds it first - « Pour moi », a holding
	// list with its members.
	holder := it.HolderLabel
	if holder == "" {
		holder = ql(lang, "mine")
	}
	return holder + " · " + placeOfWork(it, lang)
}

// placeOfWork names an item's release, then its increment.
func placeOfWork(it api.PersonItem, lang string) string {
	rel, inc := it.ReleaseTitle, it.IncrementTitle
	if rel == "" {
		rel = ql(lang, "no_release")
	}
	if inc == "" {
		inc = ql(lang, "no_increment")
	}
	return rel + " › " + inc
}

// queueFilter is the action the page shows alone: the one asked, when it is
// one of the closed set or the items on hold — none when the item to show
// first (Focus) is of another action, so the card the Inbox opens is never
// hidden; the items on hold when that card is one of them.
func queueFilter(in api.PersonQueueInput, by map[string][]api.PersonItem) string {
	if in.Focus != "" && in.Action != heldSection {
		for _, it := range by[heldSection] {
			if it.Key == in.Focus || it.Object == in.Focus || it.Ask == in.Focus {
				return heldSection
			}
		}
	}
	if !slices.Contains(api.PersonActions, in.Action) && in.Action != heldSection {
		return ""
	}
	if in.Focus != "" {
		for a, items := range by {
			if a == in.Action {
				continue
			}
			for _, it := range items {
				if it.Key == in.Focus || it.Object == in.Focus || it.Ask == in.Focus {
					return ""
				}
			}
		}
	}
	return in.Action
}

// queueTile is an action's tile (allTile: every item), its count, a filter:
// a click shows only its items — the chosen one, marked, shows all again.
func queueTile(lang, a string, count int, filter string) node {
	look := queueLook[a]
	t := kpi(ql(lang, a), count, ql(lang, "h_"+a), look[1], look[0])
	chosen := a == filter || (a == allTile && filter == "")
	to := a
	if a == allTile || chosen {
		to = ""
	}
	t["action"] = render.QueryWith(ql(lang, a), map[string]any{"action": to})
	if chosen {
		t["selected"] = true
	}
	// Its address (data-tile, data-count): the
	// screen's walks find and click each tile by it.
	t["data-tile"], t["data-count"] = a, count
	return t
}

// queueCard is one item: what it is about, what is expected, what is proven,
// and what does it.
func queueCard(it api.PersonItem, lang, focus string) node {
	badges := []node{}
	// What it is about, as a mono id - an inbox question about nothing has none.
	if it.Object != "" || it.Kind == "" {
		badges = append(badges, node{"kind": "badge", "text": it.Object, "size": "xs", "variant": "outline", "color": "gray", "mono": true})
	}
	if it.Module != "" {
		badges = append(badges, node{"kind": "badge", "text": it.Module, "size": "xs", "variant": "light", "color": "indigo", "keepCase": true})
	}
	// The project holding it, when several are served (
	// mark).
	if it.Project != "" {
		// The renderer draws no element id on a badge: the walk finds the
		// mark by its data attributes, as the GUI's own marks carry their key.
		badges = append(badges, node{"kind": "badge", "id": projectMarkID, "text": it.Project, "size": "xs", "variant": "light", "color": "teal", "keepCase": true,
			"data-element": "badge:" + projectMarkID, "data-project": it.Project})
	}
	if it.Request != "" && it.Request != it.Object {
		badges = append(badges, node{"kind": "badge", "text": it.Request, "size": "xs", "variant": "outline", "color": "gray", "mono": true})
	}
	// The inbox's questions: their kind beside the
	// badges, the title of what they are about, who asks when nothing is.
	if it.KindLabel != "" {
		badges = append(badges, node{"kind": "badge", "text": it.KindLabel, "size": "xs", "variant": "light", "color": "grape"})
	}
	// What the inbox's page showed: by when - late in
	// red -, the proof, who may answer, who took it.
	badges = append(badges, questionBadges(it, lang)...)
	kids := []node{
		{"kind": "text", "content": it.Title, "size": "sm", "fw": 600},
		{"kind": "hbox", "gap": 6, "wrap": true, "children": badges},
	}
	// An inbox question that says nothing: what its action asks.
	if it.Kind != "" && strings.TrimSpace(it.Asks) == "" {
		it.Asks = ql(lang, "h_"+it.Action)
	}
	if it.AboutTitle != "" {
		kids = append(kids, node{"kind": "text", "content": it.AboutTitle, "size": "sm", "color": "dimmed"})
	}
	// What is asked - a block or refusal note can be long: three lines, the
	// rest behind « Afficher plus » (text:asks:<key>:toggle).
	kids = append(kids, noteText("asks:"+it.Key, it.Asks, "sm", ""))
	if len(it.Satisfies) > 0 {
		kids = append(kids, node{"kind": "text", "content": fmt.Sprintf(ql(lang, "settles"), strings.Join(it.Satisfies, ", ")), "size": "xs", "color": "dimmed"})
	}
	if it.Kind != "" && it.Action != api.ActionConversation {
		if by := askedLine(it, lang); by != "" {
			kids = append(kids, node{"kind": "text", "content": by, "size": "xs", "color": "dimmed"})
		}
	}
	// A question's options shown before the pick, each with its label.
	if it.Kind != "" && len(it.Options) > 0 {
		kids = append(kids, node{"kind": "markdown", "content": "**" + ql(lang, "options") + "**\n\n- " + strings.Join(it.Options, "\n- ")})
	}
	if it.ByHand != "" {
		kids = append(kids, node{"kind": "markdown", "content": "**" + ql(lang, "how") + "**\n\n" + it.ByHand, "maxHeight": 240})
	}
	if it.Reasoning != "" {
		kids = append(kids, node{"kind": "markdown", "content": it.Reasoning, "maxHeight": 320})
	}
	if it.Confirm != nil {
		if len(it.Criteria) > 0 {
			kids = append(kids, node{"kind": "markdown", "content": criteriaProse(it.Criteria, lang), "maxHeight": 320})
		} else {
			kids = append(kids, node{"kind": "text", "content": ql(lang, "no_criteria"), "size": "xs", "color": "orange"})
		}
	}
	if it.Done != "" {
		kids = append(kids, node{"kind": "markdown", "content": "**" + ql(lang, "done") + "**\n\n" + it.Done, "maxHeight": 320})
	}
	if len(it.Challenged) > 0 {
		kids = append(kids, node{"kind": "markdown", "content": "**" + ql(lang, "challenged") + "**\n\n- " + strings.Join(it.Challenged, "\n- "), "maxHeight": 200})
	}
	for _, r := range it.Refs {
		kids = append(kids, render.Link(r.Label+" "+r.ID, render.NavigateTo(r.Label+" "+r.ID, r.ID, "")))
	}
	if len(it.Proven) > 0 {
		kids = append(kids, node{"kind": "markdown", "content": "**" + ql(lang, "proven") + "**\n\n- " + strings.Join(it.Proven, "\n- "), "maxHeight": 200})
	}
	if it.Held != nil {
		held := fmt.Sprintf(ql(lang, "held_by"), it.Held.By.ID, it.Held.At)
		if it.Held.Reason != "" {
			held += " — " + it.Held.Reason
		}
		kids = append(kids, node{"kind": "text", "content": held, "size": "xs", "color": "dimmed"})
	}
	// An inbox question is answered in place by the inbox's own nodes; once
	// answered (a chat's card), who answered and what in their place.
	actions := len(kids)
	if it.Answered != "" {
		kids = append(kids, node{"kind": "badge", "text": it.Answered, "size": "sm", "variant": "light", "color": "teal"})
	} else if len(it.Answer) > 0 {
		for _, n := range it.Answer {
			kids = append(kids, node(n))
		}
	} else if it.Action != api.ActionConversation {
		kids = append(kids, cardActions(it, lang)...)
	}
	if it.Answered == "" {
		hold := holdActions(it, lang)
		// The hold is one compact button: it joins
		// the card's own row of buttons, after its answers, as the previous
		// view placed it - never alone in the middle of the card.
		if len(hold) == 1 && hold[0]["kind"] == "button" {
			kids = withButton(kids, actions, hold[0])
		} else {
			kids = append(kids, hold...)
		}
	}
	open := it.Request
	panel := RequestPanel
	if open == "" {
		open, panel = it.Object, ""
	}
	switch {
	case it.Chat != "":
		kids = append(kids, render.Link(ql(lang, "open_chat"), render.OpenPanel(ql(lang, "open_chat"), it.Chat)))
	case open != "" || it.Kind == "":
		kids = append(kids, render.Link(ql(lang, "open")+" "+open, render.NavigateTo(ql(lang, "open"), open, panel)))
	}
	card := node{"kind": "card", "withBorder": true, "p": "md", "radius": "md", "gap": "xs", "children": kids}
	// Its element id (card:<key>): the screen's walks find each card by it.
	card["id"] = it.Key
	if focus != "" && (it.Key == focus || it.Object == focus || it.Ask == focus) {
		card["shadow"] = "md"
	}
	return card
}

// cardActions are an item's buttons and forms: exactly what its action asks.
func cardActions(it api.PersonItem, lang string) []node {
	switch it.Action {
	case api.ActionValidate:
		if len(it.Wants) > 0 {
			return wantActions(it, lang)
		}
		if it.Confirm != nil {
			return confirmActions(it, lang)
		}
		if it.Ask == "" {
			return reportVerdict(it, lang)
		}
		if it.WaitsOn != "" {
			// Not yet: its hand tests come first — the buttons show, disabled,
			// saying why.
			args := map[string]any{"id": it.Ask, "by": "${person.id}"}
			return []node{{"kind": "list", "data": []node{{"id": it.Key, "waits_on": it.WaitsOn}},
				"item": node{"kind": "text", "content": "", "dataField": "$.item.waits_on", "size": "xs", "color": "orange"},
				"actions": []node{
					{"id": "accept", "label": ql(lang, "accept"), "icon": "check", "uri": inboxAnswerURI, "disabledWhen": "waits_on", "args": args},
					{"id": "send-back", "label": ql(lang, "send_back"), "icon": "arrow-back-up", "uri": inboxAnswerURI, "disabledWhen": "waits_on", "args": args},
				}}}
		}
		return []node{render.AnswerForm(qschema[approveForm]("ApproveOrSendBack", lang, map[string]string{"decision": "Your decision", "note": "Your note"}),
			render.RunURI(ql(lang, "send"), inboxAnswerURI, map[string]any{"id": it.Ask, "decision": "${value.decision}", "note": "${value.note}", "by": "${person.id}"}),
			"send", approveForm{Decision: "approved"})}
	case api.ActionTest:
		return []node{render.AnswerForm(qschema[testForm]("HandTest", lang, map[string]string{"result": "Result", "note": "What you saw"}),
			render.RunURI(ql(lang, "record"), requestTestURI, map[string]any{"id": it.Request, "deliverable": it.Deliverable,
				"result": "${value.result}", "note": "${value.note}", "by": "${person}"}),
			"hand-finger", testForm{Result: api.TestPasses})}
	case api.ActionDecide:
		if len(it.Options) == 0 {
			return answerFormOf(it, lang)
		}
		var opts []node
		for _, o := range it.Options {
			head := []node{{"kind": "text", "content": optionLabel(o, lang), "size": "sm", "fw": 600}}
			if o == it.Proposal {
				head = append(head, node{"kind": "badge", "text": ql(lang, "proposed"), "size": "xs", "variant": "light", "color": "grape"})
			}
			row := []node{{"kind": "hbox", "gap": 6, "children": head}}
			if o == it.Proposal && it.ProposalWhy != "" {
				row = append(row, node{"kind": "text", "content": it.ProposalWhy, "size": "xs", "color": "dimmed"})
			}
			if prose := optionProse(it.Prepared, o, lang); prose != "" {
				row = append(row, node{"kind": "markdown", "content": prose})
			}
			if !strings.Contains(o, "${") {
				row = append(row, render.AnswerForm(qschema[chooseForm]("ChooseOption", lang, map[string]string{"reason": "Why"}),
					render.RunURI(ql(lang, "choose"), decisionResolve, map[string]any{"id": it.Object, "selected": []string{o}, "rationale": "${value.reason}", "actor": "${person}"}),
					"check", chooseForm{}))
			}
			opts = append(opts, node{"kind": "card", "withBorder": true, "p": "sm", "radius": "sm", "gap": 4, "children": row})
		}
		return []node{{"kind": "vbox", "gap": "xs", "children": opts}}
	case api.ActionAnswer:
		return answerFormOf(it, lang)
	case api.ActionUnblock:
		item := node{"id": it.Request, "asks": it.Asks}
		if it.Todo != "" {
			item["todo"] = it.Todo
		}
		if it.Checks {
			item["checks"] = true
		}
		if it.Resume {
			item["resume"] = true
		}
		return []node{{"kind": "list", "data": []node{item}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{
				{"id": "retry", "label": ql(lang, "retry"), "icon": "player-play", "uri": deckRetryURI, "when": "todo",
					"args": node{"id": "${item.todo}", "by": "${person}", "note": "tried again from « À faire »"}},
				{"id": "check", "label": ql(lang, "checks"), "icon": "player-play", "uri": requestCheckURI, "when": "checks",
					"args": node{"id": "${item.id}", "by": "${person}"}},
				{"id": "resume", "label": ql(lang, "resume"), "icon": "player-play", "uri": deckPlanURI, "when": "resume",
					"args": node{"id": "${item.id}", "by": "${person}", "note": "resumed from « À faire »"}},
				dropAction(),
			}}}
	case api.ActionNotice:
		return []node{{"kind": "list", "data": []node{{"id": it.Object}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "dismiss", "label": ql(lang, "dismiss"), "icon": "x", "uri": inboxDismissURI, "args": node{"id": "${item.id}"}}}}}
	}
	return nil
}

// confirmActions accept a want the work recorded done (its confirm), or
// reject it with the reason, which its journal keeps (its move back).
func confirmActions(it api.PersonItem, lang string) []node {
	m := it.Confirm
	return []node{
		{"kind": "list", "data": []node{{"id": it.Object}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "accept", "label": ql(lang, "accept"), "icon": "check", "uri": cwmTransition,
				"args": node{"id": "${item.id}", "to": m.AcceptTo, "transition": m.Accept, "by": "${person}", "note": "accepted from « À faire »"}}}},
		render.AnswerForm(qschema[rejectForm]("RejectIt", lang, map[string]string{"reason": "Why"}),
			render.RunURI(ql(lang, "reject"), cwmTransition, map[string]any{"id": it.Object, "to": m.RejectTo, "transition": m.Reject,
				"by": "${person}", "note": "${value.reason}"}),
			"arrow-back-up", rejectForm{}),
	}
}

// rejectForm is why a person rejects a want the work recorded done.
type rejectForm struct {
	Reason string `json:"reason" jsonschema:"description=What is missing or wrong,required"`
}

// criteriaProse is a want's acceptance criteria, each with its last result.
func criteriaProse(cs []api.Criterion, lang string) string {
	lines := make([]string, 0, len(cs))
	for _, c := range cs {
		mark := map[string]string{"pass": "✅", "fail": "❌"}[c.Status]
		if mark == "" {
			mark = "❔"
		}
		said := ql(lang, "verdict_"+c.Status)
		if said == "" {
			said = c.Status
		}
		line := mark + " " + c.ID + " — " + c.Title + " (" + said
		if c.RanAt != "" {
			line += ", " + c.RanAt
		}
		lines = append(lines, "- "+line+")")
	}
	return "**" + ql(lang, "criteria") + "**\n\n" + strings.Join(lines, "\n")
}

// optionLabel is an option's label in the page's language: the reconciler's
// own options (waitChoice) are words of the page; any other label is shown
// as written. The choice sent is always the stored label.
func optionLabel(label, lang string) string {
	key := "opt_" + strings.ReplaceAll(strings.ToLower(label), " ", "_")
	if w := ql(lang, key); w != "" && queueLabels["en"][key] == label {
		return w
	}
	return label
}

// optionProse is an option's prepared content as its card shows it under its
// label: what it means, its pros, its cons, its cost, what follows if chosen.
func optionProse(prepared []decisionsapi.PreparedOption, label, lang string) string {
	i := slices.IndexFunc(prepared, func(p decisionsapi.PreparedOption) bool { return p.Label == label })
	if i < 0 {
		return ""
	}
	p := prepared[i]
	var parts []string
	field := func(key, v string) {
		if v != "" {
			parts = append(parts, "**"+ql(lang, key)+"** - "+v)
		}
	}
	list := func(key string, vs []string) {
		if len(vs) > 0 {
			parts = append(parts, "**"+ql(lang, key)+"**\n\n- "+strings.Join(vs, "\n- "))
		}
	}
	field("means", p.Means)
	list("pros", p.Pros)
	list("cons", p.Cons)
	field("cost", p.Cost)
	field("if_chosen", p.Consequence)
	if p.Text != "" {
		parts = append(parts, p.Text)
	}
	return strings.Join(parts, "\n\n")
}

// wantActions list the wants an increment holds put forward, each accepted or
// dropped by the person, then accept every one at once.
func wantActions(it api.PersonItem, lang string) []node {
	var rows []node
	for _, w := range it.Wants {
		rows = append(rows, node{"id": w.ID, "label": w.ID + " — " + w.Title + " (" + w.State + ")"})
	}
	return []node{
		{"kind": "list", "data": rows, "item": node{"kind": "text", "content": "", "dataField": "$.item.label", "size": "sm"},
			"actions": []node{
				{"id": "accept", "label": ql(lang, "accept"), "icon": "check", "uri": acceptWantsURI,
					"args": node{"id": it.Object, "want": "${item.id}", "by": "${person}"}},
				{"id": "drop", "label": ql(lang, "drop"), "icon": "trash", "uri": cwmTransition, "dangerous": true, "confirm": ql(lang, "drop_confirm"),
					"args": node{"id": "${item.id}", "to": "dropped", "by": "${person}", "note": "dropped from « À faire », held by " + it.Object}},
			}},
		{"kind": "list", "data": []node{{"id": it.Object}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "accept-all", "label": ql(lang, "accept_all"), "icon": "checks", "uri": acceptWantsURI,
				"args": node{"id": "${item.id}", "by": "${person}"}}}},
	}
}

// holdActions put an item on hold, with an optional reason, or take it back.
// A notice asks nothing: it is dismissed, never held.
func holdActions(it api.PersonItem, lang string) []node {
	switch {
	case it.Action == api.ActionNotice:
		return nil
	case it.Held != nil && it.HoldIsBlock:
		// The person's own block of the plan: taken back, it resumes
		// - no hold entry to remove.
		return []node{{"kind": "list", "data": []node{{"id": it.Request}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "unhold", "label": ql(lang, "unhold"), "icon": "player-play", "uri": deckPlanURI,
				"args": node{"id": "${item.id}", "by": "${person}", "note": "resumed from « À faire »"}}}}}
	case it.Held != nil:
		return []node{{"kind": "list", "data": []node{{"id": it.Key}}, "item": node{"kind": "text", "content": "", "size": "xs"},
			"actions": []node{{"id": "unhold", "label": ql(lang, "unhold"), "icon": "player-play", "uri": requestUnholdURI,
				"args": node{"key": "${item.id}", "by": "${person}"}}}}}
	}
	form := render.AnswerForm(qschema[holdForm]("PutOnHold", lang, map[string]string{"reason": "Why it waits"}),
		render.RunURI(ql(lang, "hold"), requestHoldURI, map[string]any{"key": it.Key, "reason": "${value.reason}", "by": "${person}"}),
		"player-pause", holdForm{})
	// A compact button asking
	// why in a modal, as « Refuser » does - not a form open on every card.
	open := render.Action{ID: "hold", Label: ql(lang, "hold"), Icon: "player-pause",
		Modal: &render.Modal{Title: ql(lang, "hold"), Node: form.With("id", "hold-why:"+it.Key), Size: "sm"}}
	return []node{render.Button(ql(lang, "hold"), open).With("variant", "subtle").With("size", "xs").With("id", "hold:"+it.Key)}
}

// answerFormOf answers an item's question in the Inbox's own Answer - or,
// a step asked as a human task (no entry), by its
// to-do's report.
func answerFormOf(it api.PersonItem, lang string) []node {
	if it.Ask == "" {
		if it.Todo == "" {
			return nil
		}
		return []node{render.AnswerForm(qschema[answerForm]("AnswerTheQuestion", lang, map[string]string{"note": "Your answer"}),
			render.RunURI(ql(lang, "send"), todosReportURI, map[string]any{"id": it.Todo, "status": "success", "answer": "${value.note}", "summary": "${value.note}", "by": "${person}"}),
			"send", answerForm{})}
	}
	return []node{render.AnswerForm(qschema[answerForm]("AnswerTheQuestion", lang, map[string]string{"note": "Your answer"}),
		render.RunURI(ql(lang, "send"), inboxAnswerURI, map[string]any{"id": it.Ask, "decision": "approved", "note": "${value.note}", "by": "${person.id}"}),
		"send", answerForm{})}
}

// todosReportURI reports a to-do: a person's step asked as a human task is
// answered so.
const todosReportURI = "ktw-agents:todos/report"

// reportVerdict validates a step asked as a human task (no inbox entry,
// ) by its to-do's report: accept, or send back with
// what to change - not before the hand tests it waits on.
func reportVerdict(it api.PersonItem, lang string) []node {
	if it.Todo == "" {
		return nil
	}
	if it.WaitsOn != "" {
		// Not yet: its hand tests come first - the buttons show, disabled,
		// saying why.
		verdict := func(v string) map[string]any {
			return map[string]any{"id": it.Todo, "status": "success", "verdict": v, "summary": v, "by": "${person}"}
		}
		return []node{{"kind": "list", "data": []node{{"id": it.Key, "waits_on": it.WaitsOn}},
			"item": node{"kind": "text", "content": "", "dataField": "$.item.waits_on", "size": "xs", "color": "orange"},
			"actions": []node{
				{"id": "accept", "label": ql(lang, "accept"), "icon": "check", "uri": todosReportURI, "disabledWhen": "waits_on", "args": verdict(api.VerdictApproved)},
				{"id": "send-back", "label": ql(lang, "send_back"), "icon": "arrow-back-up", "uri": todosReportURI, "disabledWhen": "waits_on", "args": verdict(api.VerdictRejected)},
			}}}
	}
	return []node{render.AnswerForm(qschema[approveForm]("ApproveOrSendBack", lang, map[string]string{"decision": "Your decision", "note": "Your note"}),
		render.RunURI(ql(lang, "send"), todosReportURI, map[string]any{"id": it.Todo, "status": "success", "verdict": "${value.decision}",
			"summary": "${value.note}", "by": "${person}"}),
		"send", approveForm{Decision: "approved"})}
}

// holderKey orders the holders: the person's own first, then the holding
// lists by their name.
func holderKey(it api.PersonItem) string {
	if it.Holder == "" || it.Holder == holderMine {
		return ""
	}
	return "\x01" + it.HolderLabel
}

// queueHeader is whose queue it is and how many wait: « « À faire » · <the
// person> » and « n en attente ».
func queueHeader(q api.PersonQueueOutput, lang string) node {
	who := q.PersonName
	if who == "" {
		who = q.Person
	}
	title := ql(lang, personQueueTitle)
	if who != "" {
		title = fmt.Sprintf(ql(lang, "title_person"), who)
	}
	return node{"kind": "hbox", "id": "todo-header", "justify": "space-between", "align": "center", "wrap": true, "children": []node{
		{"kind": "heading", "text": title, "level": 3},
		{"kind": "badge", "text": fmt.Sprintf(ql(lang, "n_waiting"), q.Count), "variant": "filled", "color": "blue", "size": "md"},
	}}
}

// kindChips is one compact line of a chip per kind of question waiting -
// and the overdue ones -, each its count, a click keeping that kind, a
// second click (the chosen chip) all again; none when nothing waits.
func kindChips(q api.PersonQueueOutput, chosen, lang string) node {
	counts := append([]inboxapi.KindCount{}, q.Kinds...)
	if q.Overdue > 0 {
		counts = append(counts, inboxapi.KindCount{Kind: overdueKind, Label: ql(lang, overdueKind), Count: q.Overdue})
	}
	if len(counts) == 0 {
		return nil
	}
	chips := make([]node, 0, len(counts))
	for _, c := range counts {
		to, variant, color := c.Kind, "light", "gray"
		if c.Kind == chosen {
			to, variant, color = "", "filled", "blue"
		}
		if c.Kind == overdueKind {
			color = "red"
		}
		label := c.Label
		if label == "" {
			label = c.Kind
		}
		chips = append(chips, render.Button(fmt.Sprintf("%s %d", label, c.Count), render.QueryWith(label, map[string]any{"kind": to})).
			With("size", "xs").With("variant", variant).With("color", color).
			With("data-group-filter", c.Kind).With("data-count", c.Count))
	}
	return node{"kind": "hbox", "id": "kinds", "gap": 6, "wrap": true, "align": "center", "children": append([]node{
		{"kind": "text", "content": ql(lang, "kinds"), "size": "xs", "color": "dimmed"}}, chips...)}
}

// questionBadges are what the inbox's page showed of a question beside its
// badges: by when (late in red), the proof, who may answer, who took it.
func questionBadges(it api.PersonItem, lang string) []node {
	var out []node
	day := func(at string) string {
		if len(at) >= 10 {
			return at[:10]
		}
		return at
	}
	if it.Due != "" {
		text, color := fmt.Sprintf(ql(lang, "due"), day(it.Due)), "gray"
		if it.Overdue {
			text, color = fmt.Sprintf(ql(lang, "overdue_on"), day(it.Due)), "red"
		}
		out = append(out, node{"kind": "badge", "text": text, "size": "xs", "variant": "outline", "color": color})
	}
	if it.Proof != "" {
		out = append(out, node{"kind": "badge", "text": fmt.Sprintf(ql(lang, "proof"), it.Proof), "size": "xs", "variant": "outline", "color": "gray"})
	}
	if it.AnswerableBy != "" {
		out = append(out, node{"kind": "badge", "text": fmt.Sprintf(ql(lang, "answerable"), it.AnswerableBy), "size": "xs", "variant": "outline", "color": "gray"})
	}
	if it.TakenBy != "" {
		out = append(out, node{"kind": "badge", "text": fmt.Sprintf(ql(lang, "taken"), it.TakenBy), "size": "xs", "variant": "light", "color": "orange"})
	}
	return out
}

// withButton puts a button at the end of the last row of buttons among the
// card's answer nodes (kids from index from); with no row, in a row of its
// own aligned as the card's other actions.
func withButton(kids []node, from int, b node) []node {
	for i := len(kids) - 1; i >= from; i-- {
		if kids[i]["kind"] != "hbox" {
			continue
		}
		row := node{}
		for k, v := range kids[i] {
			row[k] = v
		}
		switch c := row["children"].(type) {
		case []node:
			row["children"] = append(append([]node{}, c...), b)
		case []any:
			row["children"] = append(append([]any{}, c...), b)
		case []render.Node: // the inbox's answer rows
			row["children"] = append(append([]render.Node{}, c...), render.Node(b))
		default:
			continue
		}
		kids[i] = row
		return kids
	}
	return append(kids, node{"kind": "hbox", "gap": 8, "wrap": true, "children": []node{b}})
}
