// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE BLOCKERS TAB'S SECTION DRAWN as its mock-up
// modules/research-planification-resilience/blockers-mockup.html: the
// increment's state with « Recompute » and the person's next action, the
// verdict with its counts (yours - incidents only you clear included -,
// agents', outside, incidents, repaired), the six groups K1-K6 - each root a
// card striped in its kind's colour, its reason in the page's language, who
// acts and the act it calls for (named as the act: answer, confirm, start a
// loop, file the request, plan, take it out) -, then what the reconciler
// already repaired, folded.

// blockerWords are the tab's words, per language.
var blockerWords = map[string]map[string]string{
	"en": {
		"verdict_blocked": "This increment cannot end today.",
		"verdict_free":    "Nothing holds this increment.",
		"mine_one":        "%d action is yours, the rest moves by itself or has an owner.",
		"mine_many":       "%d actions are yours, the rest moves by itself or has an owner.",
		"mine_none":       "None is yours: the rest moves by itself or has an owner.",
		"for_you":         "For you", "agents": "Agents", "outside": "Outside", "incidents": "Incidents", "repaired": "Repaired",
		"state_one": "blocked · %d blocker", "state_many": "blocked · %d blockers", "recompute": "Recompute",
		"next": "My next action", "who": "Who acts:", "since": "for %s", "holds": "holds %d of its pieces of work",
		"empty": "Nothing.",
		"K1":    "Your answers", "K1_lead": "A question asked to you, or an answer you gave that has not been applied yet.",
		"K2": "Your acts", "K2_lead": "An act only a person may do: confirm a feature, pass a gate, approve a proposal.",
		"K3": "Work ready for an agent", "K3_lead": "Work that can be taken now, and whether a loop will take it.",
		"K4": "Work waiting on something else", "K4_lead": "Work an agent cannot take yet, with what holds it and who holds that.",
		"K5": "Outside the increment", "K5_lead": "Another stream, no release, infrastructure, a freeze - with its owner.",
		"K6": "Incidents", "K6_lead": "States the reconciler cannot repair alone: a cycle, a wait on nothing, work with no plan. They live in the system's incident list, grouped by root and closed after two clean sweeps; only those needing a person's act reach « To do ».",
		"act_answer": "Answer", "act_act": "Act", "act_validate": "Validate", "act_challenge": "Challenge", "act_decide": "Decide", "act_confirm": "Confirm",
		"act_accept": "Accept", "act_plan": "Plan", "act_unblock": "Unblock", "act_open": "Open", "act_loop": "Start a loop %s",
		"act_chain": "See the chain", "act_request": "Open the request", "act_file_on": "Create the request on %s", "act_unwait": "Remove the wait", "act_break_cycle": "Break the cycle",
		"act_take_out": "Take it out of the increment", "take_out_confirm": "Take %s out of the increment? It stops counting in it.",
		"unwait_confirm": "Remove the wait of %s? Its step goes back to the queue.",
		"loop_title":     "Start a loop in Claude Code", "loop_open": "Nothing running takes this work: open Claude Code in the project and run:",
		"person": "You", "agent": "Agent", "system": "System", "outside_who": "Owner",
		"incident":      "incident %s · seen by %d sweeps",
		"repaired_fold": "Repaired automatically (%d) - by the reconciler, without an agent",
	},
	"fr": {
		"verdict_blocked": "Cet incrément ne peut pas finir aujourd'hui.",
		"verdict_free":    "Rien ne bloque cet incrément.",
		"mine_one":        "%d action vous revient, le reste avance seul ou a un responsable.",
		"mine_many":       "%d actions vous reviennent, le reste avance seul ou a un responsable.",
		"mine_none":       "Aucune ne vous revient : le reste avance seul ou a un responsable.",
		"for_you":         "Pour vous", "agents": "Agents", "outside": "Extérieur", "incidents": "Incidents", "repaired": "Réparés",
		"state_one": "bloqué · %d blocage", "state_many": "bloqué · %d blocages", "recompute": "Recalculer",
		"next": "Ma prochaine action", "who": "Qui agit :", "since": "depuis %s", "holds": "retient %d de ses travaux",
		"empty": "Rien.",
		"K1":    "Vos réponses", "K1_lead": "Une question qui vous est posée, ou une réponse que vous avez donnée et qui n'est pas encore appliquée.",
		"K2": "Vos actions", "K2_lead": "Un acte que seule une personne peut faire : confirmer une fonctionnalité, passer une porte, approuver une proposition.",
		"K3": "Travail prêt pour un agent", "K3_lead": "Du travail qui peut être pris maintenant, et si une boucle le prendra.",
		"K4": "Travail en attente d'autre chose", "K4_lead": "Du travail qu'un agent ne peut pas encore prendre, avec ce qui le retient et qui le retient.",
		"K5": "En dehors de l'incrément", "K5_lead": "Un autre flux, aucune version, l'infrastructure, un gel - avec son responsable.",
		"K6": "Incidents", "K6_lead": "Des états que le réconciliateur ne répare pas seul : un cycle, une attente sur rien, du travail sans plan. Ils vivent dans la liste des incidents du système, groupés par racine et clos après deux balayages propres ; seuls ceux qui demandent l'acte d'une personne arrivent dans « À faire ».",
		"act_answer": "Répondre", "act_act": "Agir", "act_validate": "Valider", "act_challenge": "Contester", "act_decide": "Décider", "act_confirm": "Confirmer",
		"act_accept": "Accepter", "act_plan": "Planifier", "act_unblock": "Débloquer", "act_open": "Ouvrir", "act_loop": "Lancer une boucle %s",
		"act_chain": "Voir la chaîne", "act_request": "Ouvrir la demande", "act_file_on": "Créer la demande sur %s", "act_unwait": "Retirer l'attente", "act_break_cycle": "Casser le cycle",
		"act_take_out": "Sortir de l'incrément", "take_out_confirm": "Sortir %s de l'incrément ? Il n'y compte plus.",
		"unwait_confirm": "Retirer l'attente de %s ? Son étape revient dans la file.",
		"loop_title":     "Lancer une boucle dans Claude Code", "loop_open": "Rien en marche ne prend ce travail : ouvrez Claude Code dans le projet et lancez :",
		"person": "Vous", "agent": "Agent", "system": "Système", "outside_who": "Responsable",
		"incident":      "incident %s · vu par %d balayages",
		"repaired_fold": "Réparé automatiquement (%d) - par le réconciliateur, sans agent",
	},
}

func blockerWord(lang, key string) string {
	if l, ok := blockerWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return blockerWords["en"][key]
}

// blockerColors are each kind's stripe, the mock-up's: yours waiting
// (yellow), agents' running (orange) or pending (gray), outside (grape),
// incidents (red).
var blockerColors = map[string]string{
	api.BlockerAnswer: "yellow", api.BlockerAct: "yellow", api.BlockerReady: "orange",
	api.BlockerHeld: "gray", api.BlockerOutside: "grape", api.BlockerIncident: "red",
}

// blockersBody is the Blockers tab's section: the verb's answer drawn - the
// state and its acts, the verdict, the six groups, and what the reconciler
// already repaired, folded. round is how many times Recompute was pressed:
// its next press reads it again.
func blockersBody(lang string, out api.IncrementBlockersOutput, round int) node {
	byKind := map[string][]api.IncrementBlocker{}
	for _, g := range out.Groups {
		byKind[g.Kind] = append(byKind[g.Kind], g.Blockers...)
	}
	n := func(kinds ...string) int {
		c := 0
		for _, k := range kinds {
			c += len(byKind[k])
		}
		return c
	}
	kids := []node{blockersBar(lang, out, byKind, round), blockersVerdict(lang, out, byKind, n)}
	for _, kind := range api.BlockerKinds {
		class := api.BlockerClass(kind)
		var rows []node
		for _, b := range byKind[kind] {
			rows = append(rows, blockerRow(lang, out.Increment, class, kind, b))
		}
		sec := render.Section(class+" · "+blockerWord(lang, class), "").
			With("description", blockerWord(lang, class+"_lead")).
			With("size", "md").With("badge", len(rows)).With("badgeColor", blockerColors[kind]).
			Empty(blockerWord(lang, "empty"), "")
		sec["children"] = rows
		if rows == nil {
			sec["children"] = []node{}
		}
		kids = append(kids, node(sec))
	}
	kids = append(kids, blockersRepaired(lang, out.Repaired))
	return node{"kind": "vbox", "gap": "md", "data-view": "increment-blocker-list", "children": kids}
}

// blockersBar is the line under the head: the increment's state - blocked
// and by how many - then Recompute and the person's next action.
func blockersBar(lang string, out api.IncrementBlockersOutput, byKind map[string][]api.IncrementBlocker, round int) node {
	left := []node{}
	if out.Total > 0 {
		key := "state_many"
		if out.Total == 1 {
			key = "state_one"
		}
		left = append(left, node(render.Badge(fmt.Sprintf(blockerWord(lang, key), out.Total), "red").With("id", "blockers-state")))
	}
	label := blockerWord(lang, "recompute")
	acts := []node{node(render.Button(label, render.SelectOn(label, blockersRoundKey, round+1)).With("id", "blockers-recompute").With("variant", "default"))}
	if next, ok := personsNext(byKind); ok {
		label := blockerWord(lang, "next")
		acts = append(acts, node(render.Button(label, render.NavigateTo(label, next.Work, "")).With("id", "blockers-next").With("variant", "filled")))
	}
	return node{"kind": "hbox", "gap": "sm", "wrap": true, "align": "center", "justify": "space-between", "data-view": "increment-blockers-bar", "children": []node{
		{"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "children": left},
		{"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "children": acts},
	}}
}

// personsNext is the person's next action: their oldest root, answers
// first, then their acts, then an incident only they clear.
func personsNext(byKind map[string][]api.IncrementBlocker) (api.IncrementBlocker, bool) {
	for _, kind := range []string{api.BlockerAnswer, api.BlockerAct, api.BlockerIncident} {
		for _, b := range byKind[kind] {
			if kind != api.BlockerIncident || b.ActorKind == api.ActorPerson {
				return b, true
			}
		}
	}
	return api.IncrementBlocker{}, false
}

// blockersVerdict is the tab's one-line verdict: can it end, how many acts
// are the person's, the counts by who acts and what was repaired.
func blockersVerdict(lang string, out api.IncrementBlockersOutput, byKind map[string][]api.IncrementBlocker, n func(...string) int) node {
	repaired := node(render.Badge(fmt.Sprintf("%s %d", blockerWord(lang, "repaired"), len(out.Repaired)), "green"))
	if out.Total == 0 {
		return node{"kind": "box", "callout": "green", "children": []node{{"kind": "hbox", "gap": "md", "wrap": true, "align": "center", "children": []node{
			{"kind": "text", "content": blockerWord(lang, "verdict_free"), "fw": 700}, repaired,
		}}}}
	}
	// Yours: your answers and acts, and the incidents only you clear.
	mine := n(api.BlockerAnswer, api.BlockerAct)
	for _, b := range byKind[api.BlockerIncident] {
		if b.ActorKind == api.ActorPerson {
			mine++
		}
	}
	say := blockerWord(lang, "mine_none")
	switch {
	case mine == 1:
		say = fmt.Sprintf(blockerWord(lang, "mine_one"), mine)
	case mine > 1:
		say = fmt.Sprintf(blockerWord(lang, "mine_many"), mine)
	}
	counts := []node{
		node(render.Badge(fmt.Sprintf("%s %d", blockerWord(lang, "for_you"), mine), "yellow")),
		node(render.Badge(fmt.Sprintf("%s %d", blockerWord(lang, "agents"), n(api.BlockerReady, api.BlockerHeld)), "orange")),
		node(render.Badge(fmt.Sprintf("%s %d", blockerWord(lang, "outside"), n(api.BlockerOutside)), "grape")),
		node(render.Badge(fmt.Sprintf("%s %d", blockerWord(lang, "incidents"), n(api.BlockerIncident)), "red")),
		repaired,
	}
	line := []node{
		{"kind": "text", "content": blockerWord(lang, "verdict_blocked"), "fw": 700},
		{"kind": "text", "content": say, "size": "sm"},
		{"kind": "hbox", "gap": 6, "wrap": true, "children": counts},
	}
	return node{"kind": "box", "callout": "red", "children": []node{{"kind": "hbox", "gap": "md", "wrap": true, "align": "center", "children": line}}}
}

// blockersRepaired is what the reconciler already repaired on the
// increment's work, listed last and folded, for trust.
func blockersRepaired(lang string, repaired []api.IncrementRepair) node {
	lines := []render.Node{}
	for _, r := range repaired {
		head := []node{}
		if r.Title != "" {
			head = append(head, node{"kind": "text", "content": r.Title, "fw": 600, "size": "sm"})
		}
		head = append(head, node{"kind": "text", "content": r.Work, "size": "xs", "mono": true, "color": "dimmed"})
		why := blockerSay(lang, r.Action) + " - " + blockerSay(lang, r.Reason)
		if at := shortAt(r.At); at != "" {
			why += " · " + at
		}
		lines = append(lines, render.Node{"kind": "vbox", "gap": 0, "id": "blocker-repaired-" + r.Work, "children": []node{
			{"kind": "hbox", "gap": 6, "wrap": true, "align": "baseline", "children": head},
			{"kind": "text", "content": why, "size": "sm", "color": "dimmed"},
		}})
	}
	content := render.VBox(lines...)
	if len(lines) == 0 {
		content = render.Dimmed(blockerWord(lang, "empty"))
	}
	fold := render.Fold{ID: "blockers-repaired", Label: fmt.Sprintf(blockerWord(lang, "repaired_fold"), len(repaired)), Color: "green",
		Data: map[string]string{"data-repaired": fmt.Sprint(len(repaired))}, Content: content}
	return node(render.Accordion([]render.Fold{fold}).With("id", "blockers-repaired-fold"))
}

// shortAt is a stored time as day and hour (UTC), "" when it cannot be read.
func shortAt(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return ""
	}
	return t.UTC().Format("02/01 15:04")
}

// blockerRow is one root: a card striped in its kind's colour, its title and
// id, why and since when, the chain it heads, who acts - and the act it
// calls for.
func blockerRow(lang, inc, class, kind string, b api.IncrementBlocker) node {
	head := []node{{"kind": "text", "content": b.Title, "fw": 600, "size": "sm"}, {"kind": "text", "content": b.Work, "size": "xs", "mono": true, "color": "dimmed"}}
	if b.Title == "" {
		head = head[1:]
	}
	why := blockerSay(lang, b.Reason)
	if b.Age != "" {
		why += " · " + fmt.Sprintf(blockerWord(lang, "since"), b.Age)
	}
	if len(b.Holds) > 0 {
		why += " · " + fmt.Sprintf(blockerWord(lang, "holds"), len(b.Holds))
	}
	what := []node{
		{"kind": "hbox", "gap": 6, "wrap": true, "align": "baseline", "children": head},
		{"kind": "text", "content": why, "size": "sm", "color": "dimmed"},
	}
	if b.Outside != "" {
		what = append(what, node{"kind": "text", "content": blockerSay(lang, b.Outside), "size": "xs", "color": "dimmed"})
	}
	if len(b.Chain) > 0 {
		var chain []node
		for k, c := range append([]string{b.Work}, b.Chain...) {
			if k > 0 {
				chain = append(chain, node{"kind": "text", "content": "→", "size": "xs", "color": "dimmed"})
			}
			tone := "gray"
			if k == 0 {
				tone = "orange"
			}
			chain = append(chain, node{"kind": "badge", "text": c, "size": "xs", "variant": "outline", "color": tone})
		}
		what = append(what, node{"kind": "hbox", "gap": 4, "wrap": true, "align": "center", "children": chain})
	}
	if b.Incident != "" {
		what = append(what, node{"kind": "text", "content": fmt.Sprintf(blockerWord(lang, "incident"), b.Incident, b.Sweeps), "size": "xs", "color": "red"})
	}
	what = append(what, node{"kind": "hbox", "gap": 4, "wrap": true, "children": []node{
		{"kind": "text", "content": blockerWord(lang, "who"), "size": "xs", "mono": true, "color": "dimmed"},
		{"kind": "text", "content": blockerWho(lang, b), "size": "xs"},
	}})
	line := render.Node{"kind": "hbox", "gap": "sm", "wrap": false, "align": "flex-start", "justify": "space-between", "children": []node{
		{"kind": "vbox", "gap": 2, "children": what},
		{"kind": "vbox", "gap": 4, "align": "flex-end", "children": blockerActs(lang, inc, kind, b)},
	}}
	return node(render.Card(line).With("id", "blocker-"+class+"-"+b.Work).With("p", "sm").With("gap", 4).Stripe(blockerColors[kind]))
}

// readyUntaken is how long work stays ready, untaken, before the tab says
// no loop running takes it.
const readyUntaken = time.Hour

// incrementUntargetURI takes a goal or a feature out of its increment.
const incrementUntargetURI = "ktw-agents:increments/untarget"

// planUnwaitURI removes a waiting to-do's wait (plans unwait).
const planUnwaitURI = "ktw-agents:plans/unwait"

// blockerActs are the act a root calls for, named as the act - its button
// `blocker-open-<work>` - and, for an incident, the other way out
// (`blocker-alt-<work>`): an answer, the person's act by its kind, a loop
// to start when ready work waits untaken, the chain, the request outside,
// the request to file or the plan to make for an incident.
func blockerActs(lang, inc, kind string, b api.IncrementBlocker) []node {
	w := func(k string) string { return blockerWord(lang, k) }
	open := func(label string) render.Action { return render.NavigateTo(label, b.Work, "") }
	variant := "default"
	if b.ActorKind == api.ActorPerson {
		variant = "filled"
	}
	var main render.Action
	var alt *render.Action
	switch kind {
	case api.BlockerAnswer:
		main = open(w("act_answer"))
	case api.BlockerAct:
		main = open(w(personActKey(b.Blocker)))
	case api.BlockerReady:
		main = open(w("act_open"))
		if cmd, scope := loopCommand(b.Actor); cmd != "" && waitedOver(b.Since, readyUntaken) {
			// Ready an hour and untaken: no loop running takes it.
			main = render.Action{Label: fmt.Sprintf(w("act_loop"), scope), Modal: &render.Modal{Title: w("loop_title"),
				Node: render.VBox(render.Text(w("loop_open")), render.Text(cmd).Mono().With("fw", 600))}}
			variant = "filled"
		}
	case api.BlockerHeld:
		main = open(w("act_open"))
		if len(b.Chain) > 0 {
			main = open(w("act_chain"))
		}
	case api.BlockerOutside:
		main = open(w("act_request"))
	case api.BlockerIncident:
		var a render.Action
		main, a = incidentActs(w, inc, b, open)
		alt = &a
	}
	acts := []node{node(render.Button(main.Label, main).With("id", "blocker-open-"+b.Work).With("variant", variant))}
	if alt != nil {
		acts = append(acts, node(render.Button(alt.Label, *alt).With("id", "blocker-alt-"+b.Work).With("variant", "default")))
	}
	return acts
}

// incidentActs are an incident's two ways out (K6), each named as its act:
// a cycle is broken - or the work taken out of the increment; a wait on
// work nobody works on has its request filed - or the wait removed; work
// with no work is planned - or taken out; a wait on nothing live (an
// unreadable or ended dependency, a step no longer open, a wait no rule
// reads) is planned again - or the wait removed; any other incident of the
// work's plan (no step, a blocked plan offering its step, an answer not
// resumed, a result never collected, a step the queue does not offer, a
// lifecycle ended under an open plan, two increments taking it) is planned
// again - or the work taken out.
func incidentActs(w func(string) string, inc string, b api.IncrementBlocker, open func(string) render.Action) (main, alt render.Action) {
	plan := render.RunURI(w("act_plan"), deckPlanURI, map[string]any{"id": b.Work, "by": "${person}", "note": "planned again from the increment's Blockers tab"})
	takeOut := render.RunURI(w("act_take_out"), incrementUntargetURI, map[string]any{"id": b.Work})
	takeOut.Confirm = fmt.Sprintf(w("take_out_confirm"), b.Work)
	unwait := open(w("act_unwait"))
	if b.Unwait {
		// On a to-do Unwait accepts, the wait is removed
		// elsewhere the work is opened to act on it.
		unwait = render.RunURI(w("act_unwait"), planUnwaitURI, map[string]any{"todo": b.Todo, "by": "${person}"})
		unwait.Confirm = fmt.Sprintf(w("unwait_confirm"), b.Work)
	}
	switch b.Cause {
	case api.CauseCycle:
		return open(w("act_break_cycle")), takeOut
	case api.CauseNobodyWorks:
		if b.On != "" {
			return render.RunURI(fmt.Sprintf(w("act_file_on"), b.On), requestFileURI, map[string]any{"on": b.On, "increment": inc, "by": "${person}"}), unwait
		}
	case api.CauseNoWork:
		return render.RunURI(w("act_plan"), requestFileURI, map[string]any{"on": b.Work, "increment": inc, "by": "${person}"}), takeOut
	case api.CauseUnreadable, api.CauseWaitNotCleared, api.CauseStepClosed, api.CauseUnclassified, api.CauseUnknownKind:
		return plan, unwait
	case api.CauseTakenTwice:
		// Two increments take it: aim it at one in its pane - or give it up here.
		return open(w("act_plan")), takeOut
	}
	return plan, takeOut
}

// personActKey is the word of the person's act a K2 blocker calls for, by
// its act (api.Blocker.Act).
func personActKey(b api.Blocker) string {
	switch b.Act {
	case api.ActAccept, api.ActPlan, api.ActUnblock, api.InteractionValidate, api.InteractionChallenge, api.InteractionDecide, "confirm":
		return "act_" + b.Act
	}
	return "act_act"
}

// loopCommand is the Claude Code command starting a loop on the scope a
// ready root's actor names (« an agent: a loop on increment:<id> »), and
// that scope's id; "" when it names none.
func loopCommand(actor string) (cmd, scope string) {
	on, ok := strings.CutPrefix(actor, "an agent: a loop on ")
	if !ok {
		return "", ""
	}
	kind, id, _ := strings.Cut(on, ":")
	switch kind {
	case "increment":
		return "/k-loop --increment " + id, id
	case "release":
		return "/k-loop --release " + id, id
	case "module":
		return "/k-loop " + id, id
	}
	return "", ""
}

// waitedOver says whether since (RFC 3339) is longer ago than d.
func waitedOver(since string, d time.Duration) bool {
	at, err := time.Parse(time.RFC3339Nano, since)
	return err == nil && time.Since(at) > d
}

// blockerWho is who acts, in the person's words: « You » for the person
// (with what is theirs to do), else the actor the reader named, after the
// kind of actor.
func blockerWho(lang string, b api.IncrementBlocker) string {
	kind := b.ActorKind
	if kind == api.ActorOutside {
		kind = "outside_who"
	}
	word := blockerWord(lang, kind)
	switch {
	case b.ActorKind == api.ActorPerson:
		if b.Actor == "" || b.Actor == "the person" {
			return word
		}
		if rest, ok := strings.CutPrefix(b.Actor, "the person: "); ok {
			sep := ": "
			if lang == "fr" {
				sep = " : "
			}
			return word + sep + blockerSay(lang, rest)
		}
		return word + " (" + blockerSay(lang, b.Actor) + ")"
	case b.Actor == "":
		return word
	case word == "":
		return blockerSay(lang, b.Actor)
	}
	return word + " · " + blockerSay(lang, b.Actor)
}
