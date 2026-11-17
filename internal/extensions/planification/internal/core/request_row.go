// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ONE REQUEST ROW (section 3 of the mock-up
// modules/research-delivery-overview/request-rows-mockup.html): every list of
// requests - an increment's open requests, Delivery's Running cards and the
// Planned delivery panel, an increment's goals in detail - draws a request as
// this one row of four fixed columns: a state stripe (its colour the row's
// group: running, on you, blocked, waiting, to do), its id with its kind tag
// (never wrapping), its title (two lines at most, one dense) over a meta line
// (module · open step · who holds it, or « waits on » and every chip,
// wrapping), and its step dots with « n/m · step » under them. A click opens it.

// The row's columns in pixels, 0 the title sharing what is left; the dense
// row's in a Running card and a goal; the columns with a line on their left.
var (
	rowWidths      = []int{4, 150, 0, 210}
	denseRowWidths = []int{4, 140, 0, 190}
	rowDividers    = []int{2, 3}
)

// The groups of an increment's open requests, in their order (data-group).
const (
	groupRunning = "running"
	groupOnYou   = "on-you"
	groupBlocked = "blocked"
	groupWaiting = "waiting"
	groupToDo    = "to-do"
	groupDone    = "done"
)

var rowGroups = []string{groupRunning, groupOnYou, groupBlocked, groupWaiting, groupToDo}

// rowStripe is a group's stripe colour, the mock-up's.
var rowStripe = map[string]string{
	groupRunning: "orange.6", groupOnYou: "grape.6", groupBlocked: "red.6",
	groupWaiting: "violet.7", groupToDo: "dark.2", groupDone: "green.6",
}

// rowTag is a group's tag colour on its fold.
var rowTag = map[string]string{
	groupRunning: "orange", groupOnYou: "grape", groupBlocked: "red", groupWaiting: "violet", groupToDo: "gray",
}

// rowWords are the row's words, per language.
var rowWords = map[string]map[string]string{
	"en": {
		groupRunning: "Running", groupOnYou: "On you", groupBlocked: "Blocked", groupWaiting: "Waiting", groupToDo: "To do",
		"held_by": "held by %s", "since": "since %s", "ready": "ready", "not_started": "not started", "waiting_step": "waiting",
		"waits": "waits", "waits_on": "waits on",
	},
	"fr": {
		groupRunning: "En cours", groupOnYou: "À vous", groupBlocked: "Bloquées", groupWaiting: "En attente", groupToDo: "À faire",
		"held_by": "prise par %s", "since": "depuis %s", "ready": "prête", "not_started": "pas commencée", "waiting_step": "en attente",
		"waits": "attend", "waits_on": "attend",
	},
}

func rowWord(lang, key string) string {
	if l, ok := rowWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return rowWords["en"][key]
}

// rowGroup is the group a request is listed in: its bucket, a step of it
// failed or blocked counting blocked (runningRank).
func rowGroup(rq paneRequest) string {
	switch rq.Bucket {
	case "done":
		return groupDone
	case "run":
		return groupRunning
	case "wait":
		return groupOnYou
	}
	if runningRank(rq) == runningOrder["block"] {
		return groupBlocked
	}
	if rq.Bucket == "waits" {
		return groupWaiting
	}
	return groupToDo
}

// openStep is a request's open step, read from its to-dos: the step, who
// holds it and since when, whether it is ready (a person's: on you) or
// waiting; Label is its words in one line (the row's data-step-who).
type openStep struct {
	Step, Who, Since string
	Ready, OnYou     bool
	Waits            bool
	Label            string
}

// rowTime is an instant as the row says it, in the local time.
var rowTime = func(at time.Time) string { return at.Local().Format("15:04") }

// openStepOf is a request's open step: the step a worker holds and who holds
// it, else the first step ready - a person's « on you » -, else the first
// waiting; nothing open, « ready » (the mock-up's « implement · claude-d-a »,
// « verify · on you »).
func openStepOf(ctx context.Context, lang string, rq paneRequest) openStep {
	todos, err := todosOf(ctx, rq.ID)
	if err != nil {
		return openStep{}
	}
	var ready, waiting *api.WorkTodo
	for i := range todos {
		t := &todos[i]
		step, _ := t.Work["step"].(string)
		switch t.State {
		case schedapi.StateClaimed:
			st := openStep{Step: step, Label: step}
			if t.Claim != nil && t.Claim.By.ID != "" {
				st.Who, st.Label = t.Claim.By.ID, step+" · "+t.Claim.By.ID
				if at, err := time.Parse(time.RFC3339Nano, t.Claim.At); err == nil {
					st.Since = rowTime(at)
				}
			}
			return st
		case schedapi.StateReady:
			if ready == nil {
				ready = t
			}
		case schedapi.StateWaiting:
			if waiting == nil {
				waiting = t
			}
		}
	}
	switch {
	case ready != nil:
		step, _ := ready.Work["step"].(string)
		if doer, _ := ready.Work["doer"].(string); doer == "person" {
			return openStep{Step: step, Ready: true, OnYou: true, Label: step + " · " + pageWord(lang, "on_you")}
		}
		return openStep{Step: step, Ready: true, Label: step}
	case waiting != nil:
		step, _ := waiting.Work["step"].(string)
		return openStep{Step: step, Waits: true, Label: step + " · " + pageWord(lang, "waits")}
	}
	return openStep{Label: pageWord(lang, "ready")}
}

// rowOpts are a row's variants: dense in a Running card and a goal (no kind
// tag, one title line), named to say the open step under the dots of a dense
// row.
type rowOpts struct{ dense, named bool }

// rowKind is a request's kind tag and its colour: a request's kind (build
// when unset), else its object's type (feature, goal).
func rowKind(rq paneRequest) (string, string) {
	k := rq.Kind
	if k == "" {
		return "", ""
	}
	switch k {
	case api.KindBuild, api.KindFix:
		return k, "blue"
	}
	return k, "gray"
}

// rowStepCount is « n/m »: the steps done, the one running counted, of all.
func rowStepCount(steps []api.StepChip) string {
	n := 0
	for _, s := range steps {
		switch stepStatus(s.State) {
		case "done", "running":
			n++
		}
	}
	return fmt.Sprintf("%d/%d", n, len(steps))
}

// requestRow is a request as the one row of the mock-up: stripe, id and
// kind, title over its meta line, step dots and its open step.
func requestRow(lang string, rq paneRequest, st openStep, o rowOpts) node {
	_, status, _ := rq.words()
	group := rowGroup(rq)
	widths, pad, clamp := rowWidths, "8px", 2
	if o.dense {
		widths, pad, clamp = denseRowWidths, "6px", 1
	}

	// The id, and the kind tag under it (a full row).
	idc := []node{{"kind": "text", "content": rq.ID, "size": "xs", "mono": true, "data-row": "id"}}
	if k, c := rowKind(rq); k != "" && !o.dense {
		idc = append(idc, node{"kind": "hbox", "gap": 0, "children": []node{{"kind": "badge", "text": k, "size": "xs", "variant": "light", "color": c, "data-kind": k}}})
	}

	// The open step's words: what the meta line and data-step-who say.
	label := st.Label
	meta := rowMeta(lang, rq, st, o.dense)
	switch {
	case rq.Bucket == "waits" && len(rq.On) > 0:
		label = rowWord(lang, "waits_on") + " " + strings.Join(rq.On, " ") // as the row reads: the word, then each chip
	case rq.Bucket == "idle":
		label = pageWord(lang, "nothing_left")
	}

	// The steps column: the dots, then « n/m · step » (a dense row: the step
	// alone when named, else nothing).
	steps := []node{stepDots(rq.Steps)}
	if name := rowStepName(lang, rq, st, o); !o.dense || o.named {
		var under []node
		if !o.dense {
			sep := rowStepCount(rq.Steps)
			if name != "" {
				sep += " ·"
			}
			under = append(under, node{"kind": "text", "content": sep, "size": "xs", "color": "dimmed"})
		}
		if name != "" {
			under = append(under, node{"kind": "text", "content": name, "size": "xs", "fw": 600})
		}
		if o.dense && st.Step != "" && (st.Waits || waitsOnOthers(rq)) {
			under = append(under, node{"kind": "text", "content": rowWord(lang, "waits"), "size": "xs", "color": "dimmed"})
		}
		steps = append(steps, node{"kind": "hbox", "gap": 4, "wrap": false, "justify": "flex-end", "children": under})
	}

	cells := []node{
		{"kind": "box", "bg": rowStripe[group], "data-stripe": group},
		{"kind": "vbox", "gap": 4, "justify": "center", "p": pad + " 10px", "children": idc},
		{"kind": "vbox", "gap": 3, "justify": "center", "p": pad + " 12px", "children": []node{
			{"kind": "text", "content": rq.Title, "size": "sm", "fw": 500, "lineClamp": clamp},
			meta,
		}},
		{"kind": "vbox", "gap": 4, "justify": "center", "align": "flex-end", "p": pad + " 12px", "children": steps},
	}
	cols := make([]render.Node, len(cells))
	for i, c := range cells {
		cols[i] = render.Node(c)
	}
	grid := render.Grid(widths, cols...).Dividers(rowDividers...).With("gap", 0)
	row := node{"kind": "list", "variant": "cards", "cardPadding": 0, "data": []node{{"id": rq.ID}},
		"data-request": rq.ID, "data-state": status, "data-step-who": label, "data-group": group,
		"open": render.NavigateTo(pageWord(lang, "open_it")+" "+rq.ID, rq.ID, ""),
		"item": node(grid)}
	if o.dense {
		row["data-dense"] = true
	}
	return row
}

// waitsOnOthers says a request waits on other requests (its chips).
func waitsOnOthers(rq paneRequest) bool { return rq.Bucket == "waits" && len(rq.On) > 0 }

// rowStepName is the step named under the dots: the open step; for one
// waiting on others « waiting » in a full row (« n/m · waiting ») and its open
// step in a dense named row (« implement waits », the goal's); else the
// first step not done.
func rowStepName(lang string, rq paneRequest, st openStep, o rowOpts) string {
	if waitsOnOthers(rq) && (!o.dense || st.Step == "") {
		return rowWord(lang, "waiting_step")
	}
	if st.Step != "" {
		return st.Step
	}
	for _, s := range rq.Steps {
		if stepStatus(s.State) != "done" {
			return s.Name
		}
	}
	return ""
}

// rowMeta is the line under the title, case by case: held « module · step ·
// held by who since HH:MM »; a person's « module · step · on you »; ready
// « module · step ready » (none done yet: « not started · intake ready »);
// waiting on others « module · waits on » and every chip; nothing left
// « module · nothing left - it should end ». A dense row leaves the module
// out.
func rowMeta(lang string, rq paneRequest, st openStep, dense bool) node {
	var kids []node
	dim := func(s string) node { return node{"kind": "text", "content": s, "size": "xs", "color": "dimmed"} }
	strong := func(s string) node { return node{"kind": "text", "content": s, "size": "xs", "fw": 600} }
	sep := func() { kids = append(kids, dim("·")) }
	if !dense {
		if m, err := cwmapi.ModuleOfObjectID(rq.ID); err == nil {
			kids = append(kids, dim(m))
			sep()
		}
	}
	switch {
	case rq.Bucket == "waits" && len(rq.On) > 0:
		kids = append(kids, dim(rowWord(lang, "waits_on")))
		for _, on := range rq.On {
			kids = append(kids, idChip(on))
		}
	case rq.Bucket == "idle":
		kids = append(kids, dim(pageWord(lang, "nothing_left")))
	case st.Who != "":
		kids = append(kids, strong(st.Step))
		sep()
		who := st.Who
		if !dense {
			who = fmt.Sprintf(rowWord(lang, "held_by"), st.Who)
			if st.Since != "" {
				who += " " + fmt.Sprintf(rowWord(lang, "since"), st.Since)
			}
		}
		kids = append(kids, dim(who))
	case st.OnYou:
		kids = append(kids, strong(st.Step))
		sep()
		kids = append(kids, dim(pageWord(lang, "on_you")))
	case st.Ready:
		if !dense && rowStepCount(rq.Steps) == fmt.Sprintf("0/%d", len(rq.Steps)) {
			kids = append(kids, dim(rowWord(lang, "not_started")))
			sep()
		}
		kids = append(kids, strong(st.Step), dim(rowWord(lang, "ready")))
	case st.Waits:
		kids = append(kids, strong(st.Step), dim(rowWord(lang, "waits")))
	case st.Step != "":
		kids = append(kids, strong(st.Step))
	default:
		if len(kids) > 0 {
			kids = kids[:len(kids)-1] // the module alone: no separator after it
		}
	}
	return node{"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "data-row": "meta", "children": kids}
}

// requestGroups is an increment's open requests in their groups - Running,
// On you, Blocked, Waiting, To do, in that order, each a rule fold with its
// tag, its count and data-group, every one open at first; a group with none
// is left out.
func requestGroups(ctx context.Context, lang string, open []paneRequest) node {
	by := map[string][]node{}
	for _, rq := range open {
		g := rowGroup(rq)
		by[g] = append(by[g], requestRow(lang, rq, openStepOf(ctx, lang, rq), rowOpts{}))
	}
	var folds []render.Fold
	var ids []string
	for _, g := range rowGroups {
		rows := by[g]
		if len(rows) == 0 {
			continue
		}
		id := "group-" + g
		folds = append(folds, render.Fold{ID: id, Label: rowWord(lang, g), Badge: len(rows), Color: rowTag[g],
			Data: map[string]string{"data-group": g}, Content: node{"kind": "vbox", "gap": 6, "children": rows}})
		ids = append(ids, id)
	}
	return node(render.Accordion(folds, ids...).RuleLook().With("data-view", "increment-open-groups"))
}
