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

// AN INCREMENT'S PAGE OPENS ON ITS OPEN REQUESTS (view 4
// « Increment page: open requests first » of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html). Its tabs
// are Open requests, Goals, Increment, Information and History; every tab of the planification draws the page's head: its id, its
// title with its standing, its priority and its release, one bar - done,
// running, on you - and « n done · n running · n on you · n to do ». The Open
// requests tab is that head, then a section loaded on its own: « Open
// requests (n) », the open requests as the shared request row (request_row.go)
// grouped by state, in the Running tab's order (openRequests) inside each
// group, then « n done » folded underneath.

// The page's tabs and section (the panels' inputs).
const (
	paneOpenRequests = "open-requests"
	paneOpenList     = "open-request-list"
	panePage         = "page"
)

// The page's element ids, read by its GUI test.
const (
	pageHeadView      = "increment-page-head"
	pageCountsView    = "increment-page-counts"
	openRequestsView  = "increment-open-requests"
	openListView      = "increment-open-list"
	doneRequestsFold  = "increment-done-requests"
	incrementPageView = "increment-page"
	dependenciesView  = "increment-dependencies"
)

// OpenRequestCount is the Open requests tab's badge:
// the open requests its page's « Open requests (n) » counts - running, on
// you and to do (counts) - in orange.
func (i *Increments) OpenRequestCount(ctx context.Context, in api.IncrementPaneInput) (api.IncrementCountOutput, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementCountOutput{}, err
	}
	p, err := i.paneData(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementCountOutput{}, err
	}
	return api.IncrementCountOutput{Count: p.counts().open(), Color: "orange"}, nil
}

// pageWords are the page's words, per language.
var pageWords = map[string]map[string]string{
	"en": {
		"counts": "%d done · %d running · %d on you · %d to do", "open": "Open requests (%d)",
		"done_one": "%d done", "done_many": "%d done", "empty": "No open request.",
		"ships": "ships in %s", "planned": "planned for %s", "on_you": "on you", "ready": "ready", "waits": "waits",
		"bar_done": "done", "bar_running": "running", "bar_on_you": "on you", "bar_todo": "to do",
		"proposal": "Its proposal", "no_proposal": "No proposal yet: running the increment files it.",
		"dependencies": "What it depends on", "open_it": "Open", "no_links": "No dependency between its items.",
		"waiting_on": "waiting on", "nothing_left": "nothing left - it should end",
		"review": "Its delivery review", "review_approved": "Passed - by %s", "review_rejected": "Rejected - by %s",
		"demo": "Demo: %s", "finding": "Missing: %s", "follow_up": "followed up in %s", "test_delivery": "A test delivery: %s turns its display switch on",
	},
	"fr": {
		"counts": "%d faites · %d en cours · %d à vous · %d à faire", "open": "Demandes ouvertes (%d)",
		"done_one": "%d faite", "done_many": "%d faites", "empty": "Aucune demande ouverte.",
		"ships": "livré dans %s", "planned": "prévu pour %s", "on_you": "à vous", "ready": "prête", "waits": "attend",
		"bar_done": "faites", "bar_running": "en cours", "bar_on_you": "à vous", "bar_todo": "à faire",
		"proposal": "Sa proposition", "no_proposal": "Pas encore de proposition : lancer l'incrément la dépose.",
		"dependencies": "Ce dont il dépend", "open_it": "Ouvrir", "no_links": "Aucune dépendance entre ses éléments.",
		"waiting_on": "attend", "nothing_left": "plus rien à faire - elle devrait se terminer",
		"review": "Sa revue de livraison", "review_approved": "Passée - par %s", "review_rejected": "Refusée - par %s",
		"demo": "Démo : %s", "finding": "Manque : %s", "follow_up": "suivi dans %s", "test_delivery": "Une livraison de test : %s allume son interrupteur d'affichage",
	},
}

func pageWord(lang, key string) string {
	if l, ok := pageWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return pageWords["en"][key]
}

// pageCounts are the head's counts: done, running, waiting on a person, and
// every other open request to do; open is the tab's count.
type pageCounts struct{ done, running, onYou, toDo int }

func (c pageCounts) open() int { return c.running + c.onYou + c.toDo }

// counts reads the head's counts from the requests' buckets - the ones
// counted nowhere (dropped, split) left out, as the increment's summary.
func (p paneData) counts() pageCounts {
	var c pageCounts
	for _, id := range p.order {
		switch p.requests[id].Bucket {
		case "":
		case "done":
			c.done++
		case "run":
			c.running++
		case "wait":
			c.onYou++
		default:
			c.toDo++
		}
	}
	return c
}

// pageHead is the page's head, the mock-up's: id; title, standing,
// priority, release; one bar; the counts.
func (p paneData) pageHead(ctx context.Context, cwm cwmapi.CWMService) node {
	inc, row, lang := p.inc, p.detail.Row, p.lang
	title := []node{
		{"kind": "text", "content": row.Title, "size": "lg", "fw": 700},
		render.Badge(row.Standing, statusTone(row.Standing)).With("data-standing", row.Standing),
		render.Badge(incrementRowPriority(inc), "gray").With("variant", "outline"),
	}
	if rel := inc.ShipsIn + inc.PlannedFor; rel != "" {
		key := "ships"
		if inc.PlannedFor != "" {
			key = "planned"
		}
		title = append(title, render.Badge(fmt.Sprintf(pageWord(lang, key), releaseLabelOf(ctx, cwm, rel)), "gray").With("variant", "outline"))
	}
	c := p.counts()
	segs := []node{
		{"value": c.done, "color": "green", "label": pageWord(lang, "bar_done")},
		{"value": c.running, "color": "orange", "label": pageWord(lang, "bar_running")},
		{"value": c.onYou, "color": "grape", "label": pageWord(lang, "bar_on_you")},
		{"value": c.toDo, "color": "gray", "label": pageWord(lang, "bar_todo")},
	}
	bar := node{"kind": "gauge", "size": "sm", "legend": false, "segments": segs, "maw": 520}
	if c.done+c.open() == 0 {
		bar = node{"kind": "gauge", "size": "sm", "legend": false, "value": 0, "max": 1, "maw": 520}
	}
	return node{"kind": "vbox", "gap": 4, "data-view": pageHeadView, "data-increment": inc.ID, "children": []node{
		{"kind": "text", "content": inc.ID, "size": "xs", "mono": true, "color": "dimmed"},
		{"kind": "hbox", "gap": 8, "wrap": true, "align": "center", "children": title},
		bar,
		{"kind": "text", "content": fmt.Sprintf(pageWord(lang, "counts"), c.done, c.running, c.onYou, c.toDo), "size": "xs", "mono": true, "color": "dimmed",
			"data-view": pageCountsView, "data-done": c.done, "data-running": c.running, "data-on-you": c.onYou, "data-to-do": c.toDo},
	}}
}

// openRequestsTab is the Open requests tab: the head, then its section.
func (p paneData) openRequestsTab(ctx context.Context, cwm cwmapi.CWMService) node {
	args := node{"id": p.inc.ID, "tab": paneOpenList}
	if p.lang != "" {
		args["language"] = p.lang
	}
	n := p.counts().open()
	if n == 0 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	list := loadedSection("open-requests", incrementPanePageURI, args, "", render.PlaceholderRows, n)
	return node{"kind": "vbox", "gap": "md", "data-view": openRequestsView, "data-increment": p.inc.ID, "children": []node{p.pageHead(ctx, cwm), list}}
}

// openRequestList is the Open requests tab's section: « Open requests (n) »,
// the open requests as the shared request row in their groups - Running, On
// you, Blocked, Waiting, To do -, then the done ones
// folded into one quiet line.
func (p paneData) openRequestList(ctx context.Context) node {
	lang := p.lang
	open := p.openRequests()
	kids := []node{render.SectionLabel(fmt.Sprintf(pageWord(lang, "open"), len(open))).With("data-count", len(open))}
	if len(open) == 0 {
		kids = append(kids, render.Dimmed(pageWord(lang, "empty")))
	} else {
		kids = append(kids, requestGroups(ctx, lang, open))
	}
	var done []node
	for _, id := range p.order {
		if p.requests[id].Bucket == "done" {
			chip := idChip(id)
			chip["data-done-request"] = id
			done = append(done, chip)
		}
	}
	if len(done) > 0 {
		key := "done_many"
		if len(done) == 1 {
			key = "done_one"
		}
		fold := render.Accordion([]render.Fold{{ID: doneRequestsFold, Label: fmt.Sprintf(pageWord(lang, key), len(done)),
			Content: node{"kind": "hbox", "gap": 4, "wrap": true, "children": done}}}).RuleLook()
		fold["data-view"] = doneRequestsFold
		fold["data-count"] = len(done)
		kids = append(kids, fold)
	}
	return node{"kind": "vbox", "gap": 4, "data-view": openListView, "children": kids}
}

// incrementPage is the Increment tab: the head, what a person manages - its
// release, priority, hold, the More menu -, its intent, its proposal and
// what it depends on.
// deliveryReview is the page's delivery section: its
// last delivery review - the verdict, the demo path, what it found missing -
// and the increment that turns its switch on when it is a test delivery.
func (p paneData) deliveryReview() []node {
	rv, td := p.inc.DeliveryReview, p.inc.TestDelivery
	if rv == nil && td == nil {
		return nil
	}
	lang := p.lang
	kids := []node{render.SectionLabel(pageWord(lang, "review"))}
	if rv != nil {
		verdict := "review_rejected"
		if rv.Verdict == api.VerdictApproved {
			verdict = "review_approved"
		}
		kids = append(kids, node{"kind": "text", "content": fmt.Sprintf(pageWord(lang, verdict), rv.By.ID), "size": "sm", "data-review": rv.Verdict})
		if rv.Demo != "" {
			kids = append(kids, node{"kind": "text", "content": fmt.Sprintf(pageWord(lang, "demo"), rv.Demo), "size": "sm", "data-demo": "true"})
		}
		for _, fd := range rv.Findings {
			line := fmt.Sprintf(pageWord(lang, "finding"), fd.What)
			if fd.FollowUp != "" {
				line += " - " + fmt.Sprintf(pageWord(lang, "follow_up"), fd.FollowUp)
			}
			kids = append(kids, node{"kind": "text", "content": line, "size": "sm"})
		}
	}
	if td != nil {
		kids = append(kids, node{"kind": "text", "content": fmt.Sprintf(pageWord(lang, "test_delivery"), td.TurnedOnBy), "size": "sm"})
	}
	return kids
}

func (p paneData) incrementPage(ctx context.Context, cwm cwmapi.CWMService) node {
	lang := p.lang
	kids := []node{p.pageHead(ctx, cwm), p.headActions(ctx, cwm)}
	if p.inc.Description != "" {
		kids = append(kids, node{"kind": "text", "content": p.inc.Description, "size": "sm"})
	}
	kids = append(kids, render.SectionLabel(pageWord(lang, "proposal")))
	if p.detail.Proposal != "" {
		kids = append(kids, node{"kind": "text", "content": p.detail.Proposal, "size": "sm"})
	} else {
		kids = append(kids, render.Dimmed(pageWord(lang, "no_proposal")))
	}
	kids = append(kids, p.deliveryReview()...)
	if o, err := loadIncrement(ctx, cwm, p.inc.ID); err == nil {
		kids = append(kids, render.SectionLabel(pageWord(lang, "dependencies")))
		kids = append(kids, incrementDependencies(dependencyGraph(ctx, cwm, []cwmapi.Object{o}), lang)...)
	}
	return node{"kind": "vbox", "gap": "sm", "data-view": incrementPageView, "data-increment": p.inc.ID, "children": kids}
}

// incrementDependencies is the « What it depends on » section: the graph draws
// only the boxes a depends-on arrow touches (either end), with the frame
// holding them; every other work item sits below as a grid of item cards. No
// arrow at all: a dimmed line in place of the graph. The release's
// Dependencies view keeps the whole network.
func incrementDependencies(g depGraph, lang string) []node {
	linked := map[string]bool{}
	for _, e := range g.edges {
		linked[e["from"].(string)] = true
		linked[e["to"].(string)] = true
	}
	held := map[string]bool{} // the frames a linked box sits in
	for _, b := range g.boxes {
		if grp, _ := b["group"].(string); grp != "" && linked[b["id"].(string)] {
			held[grp] = true
		}
	}
	var boxes []any
	var cards []node
	for _, b := range g.boxes {
		id := b["id"].(string)
		grp, _ := b["group"].(string)
		switch {
		case linked[id] || held[id]:
			boxes = append(boxes, b)
		case grp != "" && !strings.HasSuffix(id, "#empty"):
			status, _ := b["status"].(string)
			cards = append(cards, node{"id": id, "title": b["title"], "state": boxLabel(status), "meta": b["meta"]})
		}
	}
	open := render.NavigateTo(pageWord(lang, "open_it"), "${item.id}", "")
	var out []node
	if len(g.edges) == 0 {
		out = append(out, render.Dimmed(pageWord(lang, "no_links")))
	} else {
		edges := make([]any, len(g.edges))
		for i, e := range g.edges {
			edges[i] = e
		}
		out = append(out, render.Node{"kind": "graph", "id": dependenciesView, "data-view": dependenciesView, "data": map[string]any{"nodes": boxes, "edges": edges}, "open": open})
	}
	if len(cards) > 0 {
		out = append(out, node{"kind": "list", "layout": "grid", "columns": 3, "data": cards, "item": itemCard(),
			"toneField": "state", "toneColors": labelColors(), "open": open, "data-view": "increment-unlinked"})
	}
	return out
}

// boxLabel is a dependency box's status as the label an item card colours:
// work not started reads ready.
func boxLabel(status string) string {
	if status == "todo" {
		return stateLabels[api.PlanReady]
	}
	return status
}
