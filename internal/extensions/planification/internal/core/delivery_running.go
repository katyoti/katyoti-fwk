// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DELIVERY'S RUNNING TAB (view 2 « Running increments »
// variant B « every increment unfolded », of the mock-up
// modules/research-delivery-overview/delivery-views-v2-mockup.html) - only
// the increments being built (standing running: never defined, delivering, on
// hold or done), by priority then id; each one the planification card
// - id, title, its state badge (running / on you
// ready), done of total, its release and stream - then its OPEN requests
// only, the first runningRows of them, each with its id, title and step dots
// (the increment pane's stepDots: one component), « + n more » for the rest;
// done requests are only counted. A legend on top; a click on an increment's
// id or on a request opens it.

// runningRows is how many open requests an increment shows before « + n more ».
const runningRows = 3

// The state badges of a running increment.
const (
	runningBadgeRunning = "running" // a step of it runs now
	runningBadgeOnYou   = "on-you"  // a step of it waits on a person
	runningBadgeReady   = "ready"   // its work waits for an agent to take it
)

// runningIncrement is one increment of the Running tab, read once.
type runningIncrement struct {
	ID, Title, Priority, Badge, Release, Stream string
	Done, Total                                 int
	// Open are its open requests, in the tab's order: those running, those
	// waiting on a person, those blocked, then the rest, by id.
	Open []paneRequest
	// Steps are the open steps of the first runningRows of them, by id.
	Steps map[string]openStep
}

// runningOrder is an open request's place by its bucket.
var runningOrder = map[string]int{"run": 0, "wait": 1, "block": 2, "todo": 3, "waits": 4, "idle": 5}

// runningRank is an open request's place: running, waiting on a person, then
// blocked - its bucket, or a step of it failed or blocked and back in the
// queue - then the rest.
func runningRank(rq paneRequest) int {
	r := runningOrder[rq.Bucket]
	if r <= runningOrder["block"] {
		return r
	}
	for _, st := range rq.Steps {
		if st.State == "failed" || st.State == "blocked" {
			return runningOrder["block"]
		}
	}
	return r
}

// openRequests are the increment's open requests - neither done nor ended -
// running, waiting on a person, blocked, then the rest, by id: the Running
// tab's and the Planned delivery panel's one order.
func (p paneData) openRequests() []paneRequest {
	var open []paneRequest
	for _, id := range p.order {
		if rq := p.requests[id]; rq.Bucket != "" && rq.Bucket != "done" {
			open = append(open, rq)
		}
	}
	sort.SliceStable(open, func(i, j int) bool {
		a, b := runningRank(open[i]), runningRank(open[j])
		if a != b {
			return a < b
		}
		return idNumber(open[i].ID) < idNumber(open[j].ID)
	})
	return open
}

// deliveryRunningCount is how many increments are being built: the Running
// tab's badge, read from their summaries alone.
func deliveryRunningCount(ctx context.Context) int {
	incs := NewIncrements()
	list, err := incs.List(ctx, api.ListIncrementsInput{State: api.IncrementOpen})
	if err != nil {
		return 0
	}
	n := 0
	for _, inc := range list.Increments {
		if sum, err := incs.Summary(ctx, api.GetInput{ID: inc.ID}); err == nil && sum.Standing == api.StandingRunning {
			n++
		}
	}
	return n
}

// deliveryRunning are the increments being built, each with its open requests.
func deliveryRunning(ctx context.Context, cwm cwmapi.CWMService) []runningIncrement {
	incs := NewIncrements()
	list, err := incs.List(ctx, api.ListIncrementsInput{State: api.IncrementOpen})
	if err != nil {
		return nil
	}
	var out []runningIncrement
	for _, inc := range list.Increments {
		sum, err := incs.Summary(ctx, api.GetInput{ID: inc.ID})
		if err != nil || sum.Standing != api.StandingRunning {
			continue
		}
		p, err := incs.paneData(ctx, cwm, inc.ID)
		if err != nil {
			continue
		}
		r := runningIncrement{ID: inc.ID, Title: inc.Title, Priority: incrementRowPriority(inc),
			Done: sum.RequestsDone, Total: sum.Requests, Badge: runningBadgeReady}
		switch {
		case sum.Running > 0:
			r.Badge = runningBadgeRunning
		case sum.Awaiting > 0:
			r.Badge = runningBadgeOnYou
		}
		// Its release: the one it ships in, else the one it is planned for.
		rel := inc.ShipsIn
		if rel == "" {
			rel = inc.PlannedFor
		}
		if rel != "" {
			r.Release = releaseLabelOf(ctx, cwm, rel)
		}
		if inc.Stream != "" {
			r.Stream = inc.Stream
			if o, _, err := loadAny(ctx, cwm, inc.Stream); err == nil && o.Name != "" {
				r.Stream = o.Name
			}
		}
		r.Open = p.openRequests()
		r.Steps = openStepsOf(ctx, p.lang, r.Open, runningRows)
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := priorityRank(out[i].Priority), priorityRank(out[j].Priority)
		if a != b {
			return a < b
		}
		return idNumber(out[i].ID) < idNumber(out[j].ID)
	})
	return out
}

// runningWords are the tab's words, per language.
var runningWords = map[string]map[string]string{
	"en": {
		"tab": "Running", "running": "running", "on-you": "on you", "ready": "ready",
		"legend_done": "step done", "legend_running": "step running", "legend_on_you": "waits on you",
		"legend_failed": "failed / blocked", "legend_todo": "to come", "more": "+ %d more",
		"empty": "No increment is being built.", "open": "Open",
	},
	"fr": {
		"tab": "En cours", "running": "en cours", "on-you": "à vous", "ready": "prêt",
		"legend_done": "étape faite", "legend_running": "étape en cours", "legend_on_you": "vous attend",
		"legend_failed": "échouée / bloquée", "legend_todo": "à venir", "more": "+ %d de plus",
		"empty": "Aucun incrément n'est en cours de construction.", "open": "Ouvrir",
	},
}

func runningWord(lang, key string) string {
	if l, ok := runningWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return runningWords["en"][key]
}

// runningBadgeColor is a state badge's colour, as the mock-up draws it.
func runningBadgeColor(badge string) string {
	switch badge {
	case runningBadgeRunning:
		return "orange"
	case runningBadgeOnYou:
		return "grape"
	}
	return "blue"
}

// runningTab is the « Running » tab: the legend, then every running increment
// unfolded, each the planification card.
func runningTab(lang string, incs []runningIncrement) node {
	kids := []node{runningLegend(lang)}
	if len(incs) == 0 {
		kids = append(kids, render.Dimmed(runningWord(lang, "empty")))
	} else {
		cards := make([]card, 0, len(incs))
		for _, r := range incs {
			cards = append(cards, runningCard(lang, r))
		}
		kids = append(kids, cardList(cards, 2, nil, node{"data-view": "delivery-running-cards"}))
	}
	return node{"kind": "vbox", "gap": "sm", "data-view": "delivery-running", "children": kids}
}

// runningCard is a running increment as the planification card: its id
// (opening it), title, state badge, done of total, release and stream, then
// its first open requests as compact rows with their step dots and « + n
// more » for the rest.
func runningCard(lang string, r runningIncrement) card {
	meta := []string{fmt.Sprintf("%d/%d", r.Done, r.Total)}
	for _, s := range []string{r.Release, r.Stream} {
		if s != "" {
			meta = append(meta, s)
		}
	}
	var body []node
	for n, rq := range r.Open {
		if n == runningRows {
			break
		}
		body = append(body, runningRequest(lang, rq, r.Steps[rq.ID]))
	}
	if more := len(r.Open) - runningRows; more > 0 {
		label := fmt.Sprintf(runningWord(lang, "more"), more)
		body = append(body, node(render.Link(label, render.NavigateTo(label, r.ID, "tab:increment-open-requests")).With("data-more", r.ID)))
	}
	color := runningBadgeColor(r.Badge)
	return card{ID: r.ID, Title: r.Title, State: runningWord(lang, r.Badge), StateColor: color, Meta: strings.Join(meta, " · "),
		Body: body, Tone: color, Attrs: node{"data-increment": r.ID, "data-state": r.Badge}}
}

// runningLegend is one dot per step state and its word - the same dots.
func runningLegend(lang string) node {
	var kids []node
	for _, l := range []struct{ state, key string }{
		{"done", "legend_done"}, {"running", "legend_running"}, {"on-you", "legend_on_you"}, {"failed", "legend_failed"}, {"todo", "legend_todo"},
	} {
		word := runningWord(lang, l.key)
		kids = append(kids, node{"kind": "hbox", "gap": 4, "wrap": false, "align": "center", "children": []node{
			stepDots([]api.StepChip{{Name: word, State: l.state}}),
			{"kind": "text", "content": word, "size": "xs", "color": "dimmed"},
		}})
	}
	return node{"kind": "hbox", "gap": 14, "wrap": true, "data-view": "delivery-running-legend", "children": kids}
}

// runningRequest is an open request as the shared request row, dense
// : its stripe, id, title over its step and who, its
// step dots; a click opens it.
func runningRequest(lang string, rq paneRequest, st openStep) node {
	return requestRow(lang, rq, st, rowOpts{dense: true})
}

// openStepsOf are the open steps of the first n requests, by id.
func openStepsOf(ctx context.Context, lang string, rqs []paneRequest, n int) map[string]openStep {
	out := map[string]openStep{}
	for i, rq := range rqs {
		if i == n {
			break
		}
		out[rq.ID] = openStepOf(ctx, lang, rq)
	}
	return out
}
