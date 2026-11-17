// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE PLANNED DELIVERY PANEL ON ITS OPEN REQUESTS (view 3
// « Planned delivery: side panel » of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html), drawn by
// PanelPage. The increment picked in Planned delivery shows on the
// right its OPEN requests, as the Running tab draws them (runningRequest: id,
// title, step dots - one component), never its goals: its id, priority and
// release on one line, its title, « n open · n done · n waits on you », the
// first panelRows open requests in the Running tab's order, « + n more »
// opening its Requests tab, then « Open the increment › » and « Put on hold »
// (Resume when it is held). Goals and details stay on its own page.

// panelRows is how many open requests the panel shows before « + n more ».
const panelRows = 3

// The panel's element ids, read by its GUI test.
const (
	panelView         = "increment-panel"
	panelRequestsView = "increment-panel-requests"
)

// panelWords are the panel's words, per language.
var panelWords = map[string]map[string]string{
	"en": {
		"open_one": "open", "open_many": "open", "done_one": "done", "done_many": "done", "on_you": "waits on you", "held": "on hold: ", "empty": "No open request.",
		"open": "Open the increment ›", "open_action": "Open the increment",
		"hold": "Put on hold", "hold_title": "Put it on hold", "hold_reason": "Why it waits", "resume": "Resume",
		"pick": "Pick an increment on the left to see its open requests.",
	},
	"fr": {
		"open_one": "ouverte", "open_many": "ouvertes", "done_one": "faite", "done_many": "faites", "on_you": "à vous", "held": "suspendu : ", "empty": "Aucune demande ouverte.",
		"open": "Ouvrir l'incrément ›", "open_action": "Ouvrir l'incrément",
		"hold": "Suspendre", "hold_title": "Le suspendre", "hold_reason": "Pourquoi il attend", "resume": "Reprendre",
		"pick": "Choisissez un incrément à gauche pour voir ses demandes ouvertes.",
	},
}

func panelWord(lang, key string) string {
	if l, ok := panelWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return panelWords["en"][key]
}

// panelCounts is « n open · n done · n waits on you » in lang, each word in
// its singular or plural form (French: plural from 2 on).
func panelCounts(lang string, open, done, onYou int) string {
	form := func(key string, n int) string {
		if n > 1 {
			return panelWord(lang, key+"_many")
		}
		return panelWord(lang, key+"_one")
	}
	return fmt.Sprintf("%d %s · %d %s · %d %s", open, form("open", open), done, form("done", done), onYou, panelWord(lang, "on_you"))
}

// holdFormSchema is the « Put it on hold » form, its one field labelled and
// described in the person's words (reason).
func holdFormSchema(reason string) map[string]any {
	s := render.InlineSchema[incrementHoldForm]("HoldIncrement", map[string]string{"reason": reason})
	if t, ok := s["types"].(map[string]any)["HoldIncrement"].(map[string]any); ok {
		if f, ok := t["fields"].(map[string]any)["reason"].(map[string]any); ok {
			f["description"] = reason
		}
	}
	return s
}

// openRequestsPanel is the Planned delivery panel of the increment p reads, in its
// person's language (p.lang).
func openRequestsPanel(ctx context.Context, cwm cwmapi.CWMService, p paneData) node {
	inc, lang := p.inc, p.lang
	head := []string{inc.ID, incrementRowPriority(inc)}
	rel := inc.ShipsIn
	if rel == "" {
		rel = inc.PlannedFor
	}
	if rel != "" {
		head = append(head, releaseLabelOf(ctx, cwm, rel))
	}
	open := p.openRequests()
	done, onYou := 0, 0
	for _, id := range p.order {
		if p.requests[id].Bucket == "done" {
			done++
		}
	}
	for _, rq := range open {
		if rq.Bucket == "wait" {
			onYou++
		}
	}
	kids := []node{
		{"kind": "text", "content": strings.Join(head, " · "), "size": "xs", "mono": true, "color": "dimmed", "data-view": "increment-panel-head"},
		{"kind": "text", "content": inc.Title, "size": "md", "fw": 700},
		{"kind": "text", "content": panelCounts(lang, len(open), done, onYou), "size": "xs", "mono": true, "color": "dimmed",
			"data-view": "increment-panel-counts", "data-open": len(open), "data-done": done, "data-on-you": onYou},
	}
	if inc.Held != nil {
		kids = append(kids, node{"kind": "text", "content": panelWord(lang, "held") + inc.Held.Reason, "size": "xs", "color": "yellow"})
	}
	rows := []node{}
	steps := openStepsOf(ctx, lang, open, panelRows)
	for n, rq := range open {
		if n == panelRows {
			break
		}
		rows = append(rows, runningRequest(lang, rq, steps[rq.ID]))
	}
	if len(open) == 0 {
		rows = append(rows, render.Dimmed(panelWord(lang, "empty")))
	}
	kids = append(kids, node{"kind": "vbox", "gap": 4, "data-view": panelRequestsView, "children": rows})
	if more := len(open) - panelRows; more > 0 {
		label := fmt.Sprintf(runningWord(lang, "more"), more)
		kids = append(kids, render.Link(label, render.NavigateTo(label, inc.ID, "tab:increment-open-requests")).With("data-more", inc.ID).With("data-hidden", more))
	}
	acts := []node{render.Button(panelWord(lang, "open"), render.NavigateTo(panelWord(lang, "open_action"), inc.ID, "")).
		With("variant", "filled").With("data-control", "open-increment")}
	if inc.State == api.IncrementOpen {
		if inc.Held != nil {
			acts = append(acts, render.Button(panelWord(lang, "resume"), render.RunURI(panelWord(lang, "resume"), incrementResumeURI, map[string]any{"id": inc.ID, "by": "${person}"})).
				With("variant", "subtle").With("data-control", "resume"))
		} else {
			hold := drawerButton(panelWord(lang, "hold"), panelWord(lang, "hold_title"), render.AnswerForm(holdFormSchema(panelWord(lang, "hold_reason")),
				render.RunURI(panelWord(lang, "hold_title"), boardMoveURI, withLanguage(map[string]any{"id": inc.ID, "column": heldColumn, "reason": "${value.reason}", "by": "${person}"}, lang)), "player-pause", incrementHoldForm{}))
			hold["variant"] = "subtle"
			hold["data-control"] = "hold"
			acts = append(acts, hold)
		}
		if whyNotRunnable(inc) == "" {
			acts = append(acts, runButton(inc.ID, p.actWord("start"), p.actWord("start_title")))
		}
	}
	kids = append(kids, node{"kind": "hbox", "gap": "xs", "wrap": true, "data-view": "panel-actions", "children": acts})
	return node{"kind": "vbox", "gap": "sm", "data-view": panelView, "data-increment": inc.ID, "children": kids}
}
