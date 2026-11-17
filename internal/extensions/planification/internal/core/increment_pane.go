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

// AN INCREMENT'S PANE: its tabs - Open requests, Goals
// Increment, Information, History, and Blockers while IN-056's switch admits
// it - each a panel of the increment's type; PanePage answers the
// planification's: Open requests and its section, Goals - each delivery goal
// in detail, each request with its open step and every step named -,
// Increment, Blockers and its section. A click on a request, a feature or a
// goal opens it in its own pane.

// The increment verbs its pane's acts run.
const (
	incrementUnshipURI = "ktw-agents:increments/unship"
	incrementDoneURI   = "ktw-agents:increments/done"
	incrementDropURI   = "ktw-agents:increments/drop"
)

// The pane's tabs: the panels' inputs.
const (
	paneGoals = "goals"
	// incrementPanePageURI answers every tab.
	incrementPanePageURI = "ktw-agents:increments/pane-page"
)

// The request buckets, in the bar's order, with their row group and words:
// their colour is the group's stripe (rowStripe, request-rows-mockup.html's
// one state palette); nothing left, a bucket no row group lists, is pink.
var paneBuckets = []struct{ key, group, word, status string }{
	{"done", groupDone, "done", "done"},
	{"run", groupRunning, "running", "running"},
	{"wait", groupOnYou, "on you", "waiting"},
	{"waits", groupWaiting, "waiting", "waiting-on"},
	{"todo", groupToDo, "to do", "todo"},
	{"block", groupBlocked, "blocked", "blocked"},
	{"idle", "", "nothing left", "idle"},
}

// bucketTone is a bucket's colour: its row group's stripe.
func bucketTone(group string) string {
	if c, ok := rowStripe[group]; ok {
		return c
	}
	return "pink"
}

// paneRequest is a request as the pane draws it.
type paneRequest struct {
	ID, Title, Bucket, Where string
	Steps                    []api.StepChip
	// On is what it waits on (the requests named, each a link).
	On []string
	// Ended is how a request counted nowhere ended: dropped or split.
	Ended string
	// Kind is its row's kind tag: a request's kind (build when unset), else
	// its object's type (feature, goal).
	Kind string
}

// words are a request's state word and status: its bucket's, or how it
// ended when it is counted nowhere.
func (r paneRequest) words() (word, status, color string) {
	if r.Bucket == "" {
		e := r.Ended
		if e == "" {
			e = api.PlanDropped
		}
		return e, e, "dark"
	}
	word, status = bucketWords(r.Bucket)
	return word, status, bucketColor(r.Bucket)
}

// newPaneRequest is a request as the pane draws it: its bucket, and - when it
// waits or has nothing left - what it waits on in place of its open step.
func newPaneRequest(ctx context.Context, cwm cwmapi.CWMService, id, title, where string, steps []api.StepChip) paneRequest {
	b, why, on := requestBucket(ctx, cwm, id)
	if b == "" {
		// Ended without shipping (dropped, split): no open step to say.
		return paneRequest{ID: id, Title: title, Steps: steps, Ended: why}
	}
	if why != "" {
		where = why
	}
	return paneRequest{ID: id, Title: title, Where: where, Steps: steps, Bucket: b, On: on, Kind: requestKindOf(ctx, cwm, id)}
}

// requestKindOf is a request's kind tag: its kind (build when unset) when it
// is a request, else its object's type (a feature, a goal carrying one).
func requestKindOf(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return ""
	}
	if o.Type != api.CapRequest {
		return o.Type
	}
	if k := fieldsOf(o).Kind; k != "" {
		return k
	}
	return api.KindBuild
}

// paneGoalLine is a delivery goal: its key, its words, what it became and the
// requests serving it.
type paneGoalLine struct {
	Key, Text  string
	Constraint bool // the proposal made nothing of it: checked when the increment ends
	Analysed   bool
	Became     []api.HeldLine
	Requests   []string
}

// paneData is what every tab reads.
type paneData struct {
	inc      api.Increment
	detail   api.IncrementDetail
	goals    []paneGoalLine
	requests map[string]paneRequest
	order    []string // every request once, in the goals' order, then the filed ones
	filed    []string // the requests it counts that serve none of its goals
	holders  []string
	lang     string // the person's language (en or fr): IN-062's page words
}

// PanePage answers an increment's pane: the tab its panel names, or the goal
// region of its Overview.
func (i *Increments) PanePage(ctx context.Context, in api.IncrementPaneInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	if in.Tab == paneBlockerList {
		// The Blockers tab's section: the verb's answer, drawn.
		b, err := i.Blockers(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			return render.Def{}, err
		}
		return render.Def{Kind: "page", DSLTree: blockersBody(in.Language, b, in.Round)}, nil
	}
	p, err := i.paneData(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	p.lang = in.Language
	title := p.detail.Row.Title
	var tree node
	var data any
	switch in.Tab {
	case "", paneOpenRequests:
		tree = p.openRequestsTab(ctx, cwm)
	case paneOpenList:
		// The Open requests tab's section.
		tree = p.openRequestList(ctx)
	case paneGoals:
		tree = p.goalsTab(ctx, cwm)
	case panePage:
		tree = p.incrementPage(ctx, cwm)
	case paneBlockers:
		tree = p.blockersTab(ctx, cwm)
		data = map[string]any{"selection": blockersSelection()}
	default:
		return render.Def{}, invalid("PanePage: no tab %q (open-requests, open-request-list, goals, page, blockers, blocker-list)", in.Tab)
	}
	def := render.Def{Kind: "page", Title: title, DSLTree: tree, Data: data}
	return def, nil
}

// paneData reads the increment once: its detail, its goals and their requests.
func (i *Increments) paneData(ctx context.Context, cwm cwmapi.CWMService, id string) (paneData, error) {
	d, err := i.Detail(ctx, api.IncrementDetailInput{ID: id})
	if err != nil {
		return paneData{}, err
	}
	o, err := loadIncrement(ctx, cwm, id)
	if err != nil {
		return paneData{}, err
	}
	p := paneData{inc: toIncrement(ctx, o), detail: d, requests: map[string]paneRequest{}}
	// The requests it counts, as its summary does: a goal's requests are
	// among them, every other one is filed in it.
	var counts []cwmapi.Object
	if scope, err := scopeOf(ctx, cwm, o); err == nil {
		counts = incrementRequestObjects(ctx, cwm, o, scope)
	}
	countsIt := map[string]bool{}
	for _, ro := range counts {
		countsIt[ro.ID] = true
	}
	analysed := d.Row.Standing != api.StandingDefined
	for k, g := range d.Goals {
		gl := paneGoalLine{Key: fmt.Sprintf("G%d", k+1), Text: g.Goal, Analysed: analysed, Became: g.Became}
		seen := map[string]bool{}
		for _, h := range g.Became {
			for _, rq := range h.Requests {
				if seen[rq.ID] || !countsIt[rq.ID] {
					// Counted once; one the increment no longer holds (its
					// feature untargeted) is counted nowhere.
					continue
				}
				seen[rq.ID] = true
				gl.Requests = append(gl.Requests, rq.ID)
				if _, ok := p.requests[rq.ID]; !ok {
					p.requests[rq.ID] = newPaneRequest(ctx, cwm, rq.ID, rq.Title, rq.Where, rq.Steps)
					p.order = append(p.order, rq.ID)
				}
			}
		}
		gl.Constraint = analysed && len(g.Became) == 0
		p.goals = append(p.goals, gl)
	}
	// Every other request the increment counts - filed straight into it, its
	// proposal - in its own group, so the pane counts what the summary does.
	for _, ro := range counts {
		if _, ok := p.requests[ro.ID]; ok {
			continue
		}
		p.requests[ro.ID] = newPaneRequest(ctx, cwm, ro.ID, titleOf(ro), whereInWork(ctx, ro.ID), stepsOf(ctx, ro))
		p.order = append(p.order, ro.ID)
		p.filed = append(p.filed, ro.ID)
	}
	if r, err := i.Readiness(ctx, api.GetInput{ID: id}); err == nil {
		p.holders = r.HeldBy
	}
	return p, nil
}

// requestBucket is where a request stands in the bar: the increment
// summary's own counting (workSummary.add) - and, when it waits or has
// nothing left, why and on what.
func requestBucket(ctx context.Context, cwm cwmapi.CWMService, id string) (bucket, why string, on []string) {
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return "todo", "", nil
	}
	var w workSummary
	w.add(ctx, o)
	switch {
	case w.total() == 0:
		// Dropped or split: listed, counted nowhere, as the summary - and
		// said so, never « to do ».
		return "", planState(o), nil
	case w.done > 0:
		return "done", "", nil
	case w.blocked > 0:
		return "block", "", nil
	case w.awaiting > 0:
		return "wait", "", nil
	case w.running > 0:
		return "run", "", nil
	case w.waiting > 0:
		_, why, on := openState(ctx, o)
		return "waits", why, on
	case w.idle > 0:
		_, why, _ := openState(ctx, o)
		return "idle", why, nil
	}
	return "todo", "", nil
}

// bar is a goal's progress (request-rows-mockup.html section 4): its
// requests split by bucket in the shared palette, each segment listing its
// requests on hover (each opening its object), its legend swatches - and no
// count of its own (the card says it once, above the bar).
func (p paneData) bar(ids []string) node {
	var segs []render.Segment
	for _, b := range paneBuckets {
		var items []render.SegmentItem
		for _, id := range ids {
			if r := p.requests[id]; r.Bucket == b.key {
				items = append(items, render.SegmentItem{ID: id, Title: r.Title})
			}
		}
		if len(items) > 0 {
			segs = append(segs, render.Segment{Value: float64(len(items)), Color: bucketTone(b.group), Label: goalWord(p.lang, "b_"+b.key), Items: items})
		}
	}
	if len(segs) == 0 {
		// No request yet: an empty bar.
		return node{"kind": "gauge", "size": "md", "legend": false, "value": 0, "max": 1, "valueText": ""}
	}
	return node(render.Stacked("", segs...).Swatches().With("size", "md").With("valueText", ""))
}

// counted is ids less the requests counted nowhere (dropped, split), as the
// increment's summary counts them.
func (p paneData) counted(ids []string) []string {
	var out []string
	for _, id := range ids {
		if p.requests[id].Bucket != "" {
			out = append(out, id)
		}
	}
	return out
}

// doneOf counts the requests done among ids.
func (p paneData) doneOf(ids []string) int {
	n := 0
	for _, id := range ids {
		if p.requests[id].Bucket == "done" {
			n++
		}
	}
	return n
}

// header is the pane's head (pageHead), its actions on
// its Increment tab.
func (p paneData) header(ctx context.Context, cwm cwmapi.CWMService) node {
	return p.pageHead(ctx, cwm)
}

// actionWords are the words of the increment page's actions, per language
// : its header's and its More menu's.
var actionWords = map[string]map[string]string{
	"en": {
		"release": "Release", "change_release": "Change release", "priority": "Priority", "ships_in": "ships in", "planned_for": "planned for",
		"resume": "Resume", "hold": "Put on hold", "hold_title": "Put it on hold", "hold_reason": "Why it waits",
		"start": "Run", "start_title": "Run it: its analysis and proposal are filed",
		"run": "Run in Claude Code", "run_title": "Run it in Claude Code", "run_open": "Open Claude Code in the project and run:",
		"run_note": "Each session works the increment's to-dos one by one and stops on a step a person answers.",
		"more":     "More", "edit_goals": "Edit goals", "goals_title": "Its delivery goals", "goals_field": "Delivery goals", "goals_save": "Save the delivery goals",
		"add_dependency": "Add a dependency", "dependency_title": "The increment it waits on", "dependency_field": "The increment it waits on (its id)", "dependency_save": "Wait on it",
		"take_out": "Take out", "take_out_confirm": "Take it out of its release? It waits for another.", "mark_done": "Mark it done",
		"force_done": "Force it done", "force_done_prompt": "Its delivery is not proven usable (%s). Close it anyway: your note says why",
		"drop": "Drop it", "drop_confirm": "Drop this increment? Its work stops being pulled.",
		"goals_help": "Its delivery goals - one per line - in the user's words",
	},
	"fr": {
		"release": "Version", "change_release": "Changer de version", "priority": "Priorité", "ships_in": "livré dans", "planned_for": "prévu pour",
		"resume": "Reprendre", "hold": "Suspendre", "hold_title": "Le suspendre", "hold_reason": "Pourquoi il attend",
		"start": "Lancer", "start_title": "Le lancer : son analyse et sa proposition sont déposées",
		"run": "Lancer dans Claude Code", "run_title": "Le lancer dans Claude Code", "run_open": "Ouvrez Claude Code dans le projet et lancez :",
		"run_note": "Chaque session traite les tâches de l'incrément une à une et s'arrête sur une étape à laquelle une personne répond.",
		"more":     "Plus", "edit_goals": "Modifier les objectifs", "goals_title": "Ses objectifs de livraison", "goals_field": "Objectifs de livraison", "goals_save": "Enregistrer les objectifs de livraison",
		"add_dependency": "Ajouter une dépendance", "dependency_title": "L'incrément qu'il attend", "dependency_field": "L'incrément qu'il attend (son id)", "dependency_save": "L'attendre",
		"take_out": "Retirer", "take_out_confirm": "Le retirer de sa version ? Il en attend une autre.", "mark_done": "Marquer terminé",
		"force_done": "Forcer la clôture", "force_done_prompt": "Sa livraison n'est pas prouvée utilisable (%s). Le clore quand même : votre note dit pourquoi",
		"drop": "Abandonner", "drop_confirm": "Abandonner cet incrément ? Son travail n'est plus pris.",
		"goals_help": "Ses objectifs de livraison - un par ligne - dans les mots de l'utilisateur",
	},
}

// helped gives a form's field its help (its description) in the pane's
// language.
func (p paneData) helped(schema map[string]any, root, field, key string) map[string]any {
	if t, ok := schema["types"].(map[string]any)[root].(map[string]any); ok {
		if f, ok := t["fields"].(map[string]any)[field].(map[string]any); ok {
			f["description"] = p.actWord(key)
		}
	}
	return schema
}

// actWord is an action's word in the pane's language.
func (p paneData) actWord(key string) string {
	if l, ok := actionWords[p.lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return actionWords["en"][key]
}

// modeWord is how a release option is reached - ships in or planned for.
func (p paneData) modeWord(mode string) string {
	switch mode {
	case "ships-in":
		return p.actWord("ships_in")
	case "planned-for":
		return p.actWord("planned_for")
	}
	return mode
}

// headActions are the header's actions: its release, its priority, Put on
// hold or Resume, Run while it can run (the verb's own predicate,
// ), and Run in Claude Code once it is run - before
// there is nothing planned for a session to work.
func (p paneData) headActions(ctx context.Context, cwm cwmapi.CWMService) node {
	inc, row := p.inc, p.detail.Row
	w := p.actWord
	var acts []node
	if inc.State == api.IncrementOpen {
		current := inc.ShipsIn + inc.PlannedFor
		var options []map[string]any
		if current != "" {
			options = append(options, map[string]any{"value": current, "label": releaseLabelOf(ctx, cwm, current)})
		}
		for _, o := range p.detail.Options {
			if o.ID != "" && o.ID != current {
				options = append(options, map[string]any{"value": o.ID, "label": o.Label + " — " + p.modeWord(o.Mode)})
			}
		}
		if len(options) > 0 {
			sel := node{"kind": "select", "aria-label": w("release"), "size": "xs", "options": options,
				"action": render.RunURI(w("change_release"), incrementPlanForURI, map[string]any{"id": inc.ID, "in": "${value}", "by": "${person}"})}
			if current != "" {
				sel["value"] = current
			}
			acts = append(acts, sel)
		}
		pr := render.InlineSelect(row.Priority, "P0|P1|P2|P3", render.RunURI(w("priority"), requestAssessURI, map[string]any{"id": inc.ID, "priority": "${value}", "by": "${person}"}))
		pr["size"], pr["aria-label"] = "xs", w("priority")
		acts = append(acts, pr)
		if inc.Held != nil {
			acts = append(acts, render.Button(w("resume"), render.RunURI(w("resume"), incrementResumeURI, map[string]any{"id": inc.ID, "by": "${person}"})).With("variant", "default"))
		} else {
			acts = append(acts, drawerButton(w("hold"), w("hold_title"), render.AnswerForm(p.helped(render.InlineSchema[incrementHoldForm]("HoldIncrement", map[string]string{"reason": w("hold_reason")}), "HoldIncrement", "reason", "hold_reason"),
				render.RunURI(w("hold_title"), incrementHoldURI, map[string]any{"id": inc.ID, "reason": "${value.reason}", "by": "${person}"}), "player-pause", incrementHoldForm{})))
		}
		// What the mock-up's head leaves out stays one menu away: its goals,
		// a dependency, taking it out, marking it done, dropping it.
		acts = append(acts, p.moreMenu(ctx, cwm))
		// Claude Code runs it from a terminal in the project: the command to
		// run, in a modal (no session is started from here).
		if whyNotRunnable(inc) == "" {
			acts = append(acts, runButton(inc.ID, w("start"), w("start_title")))
		}
		if inc.Running {
			cmd := "/k-loop --increment " + inc.ID
			acts = append(acts, render.Button(w("run"), render.Action{Label: w("run"), Modal: &render.Modal{Title: w("run_title"),
				Node: render.VBox(render.Text(w("run_open")), node{"kind": "text", "content": cmd, "mono": true, "fw": 600},
					render.Dimmed(w("run_note")))}}).With("variant", "filled"))
		}
	}
	return node{"kind": "hbox", "gap": "xs", "wrap": true, "align": "center", "data-view": "increment-actions", "children": acts}
}

// runButton is « Run » / « Lancer »: the person's go (increments run),
// primary; data-control="run-increment" names it for a walk.
func runButton(id, label, title string) node {
	b := render.Button(label, render.RunURI(title, incrementRunURI, map[string]any{"id": id, "by": "${person}"})).With("variant", "filled")
	b["data-control"] = "run-increment"
	return node(b)
}

// moreMenu holds the increment's other acts: Edit goals, Add a dependency,
// Take out (its release still planned), Mark it done (all it holds done),
// Drop it.
func (p paneData) moreMenu(ctx context.Context, cwm cwmapi.CWMService) node {
	inc := p.inc
	w := p.actWord
	goals := inc.Delivery
	if goals == nil {
		goals = []string{}
	}
	entries := []map[string]any{
		render.MenuEntry(render.Action{ID: "edit-goals", Label: w("edit_goals"), Drawer: &render.Drawer{Title: w("goals_title"), Node: render.AnswerForm(p.helped(render.InlineSchema[goalsForm]("DeliveryGoals", map[string]string{"goals": w("goals_field")}), "DeliveryGoals", "goals", "goals_help"),
			render.RunURI(w("goals_save"), incrementSetGoalsURI, map[string]any{"id": inc.ID, "goals": "${value.goals}", "by": "${person}"}), "check", goalsForm{Goals: goals})}}, "", ""),
		render.MenuEntry(render.Action{ID: "add-dependency", Label: w("add_dependency"), Drawer: &render.Drawer{Title: w("dependency_title"), Node: render.AnswerForm(p.helped(render.InlineSchema[dependForm]("IncrementDepend", map[string]string{"on": w("dependency_field")}), "IncrementDepend", "on", "dependency_field"),
			render.RunURI(w("dependency_save"), requestDependURI, map[string]any{"id": inc.ID, "on": "${value.on}"}), "git-merge", dependForm{})}}, "", ""),
	}
	if inc.ShipsIn != "" {
		if r, err := loadRelease(ctx, cwm, inc.ShipsIn); err == nil && releaseState(r) == api.ReleasePlanned {
			entries = append(entries, render.MenuEntry(render.Action{ID: "take-out", Label: w("take_out"), URI: incrementUnshipURI, Args: map[string]any{"id": inc.ID, "by": "${person}"},
				Confirm: w("take_out_confirm")}, "", ""))
		}
	}
	if sc, err := NewIncrements().Scope(ctx, api.GetInput{ID: inc.ID}); err == nil && sc.Total > 0 && sc.Done == sc.Total {
		entries = append(entries, doneEntry(ctx, cwm, inc.ID, w))
	}
	drop := render.RunURI(w("drop"), incrementDropURI, map[string]any{"id": inc.ID, "by": "${person}"})
	drop.ID, drop.Confirm, drop.Dangerous = "drop", w("drop_confirm"), true
	entries = append(entries, render.MenuEntry(drop, "", ""))
	m := render.Menu(w("more"), entries...)
	m["data-view"] = "increment-more"
	return m
}

// doneEntry is the More menu's close once everything the increment holds is
// done: « Mark it done » while usable-delivered
// accepts it; while the gate refuses, « Force it done » in its place - it
// names why the gate refuses, asks the person's note and runs the done
// forced with it, so the pane never offers a close the gate refuses.
func doneEntry(ctx context.Context, cwm cwmapi.CWMService, id string, w func(string) string) map[string]any {
	gate := api.CheckReadyOutput{Accept: true}
	if o, err := loadIncrement(ctx, cwm, id); err == nil {
		gate = usableDelivered(ctx, cwm, o)
	}
	if gate.Accept {
		done := render.RunURI(w("mark_done"), incrementDoneURI, map[string]any{"id": id, "by": "${person}"})
		done.ID = "mark-done"
		return render.MenuEntry(done, "", "")
	}
	force := render.RunURI(w("force_done"), incrementDoneURI, map[string]any{"id": id, "by": "${person}", "force": true, "note": "${value}"})
	force.ID, force.Dangerous = "force-done", true
	return render.MenuEntry(force, fmt.Sprintf(w("force_done_prompt"), gate.Reason), "")
}

// becameLabel names what a goal became: Features when only features.
func becameLabel(hs []api.HeldLine) string {
	for _, h := range hs {
		if !strings.Contains(h.ID, ".F-") {
			return "What it became"
		}
	}
	if len(hs) == 1 {
		return "Feature"
	}
	return "Features"
}

// stepDots are a request's steps as dots, each named on hover.
func stepDots(steps []api.StepChip) node {
	s := []node{}
	for _, st := range steps {
		s = append(s, node{"id": st.Name, "label": st.Name, "status": stepStatus(st.State)})
	}
	return node{"kind": "stepper", "variant": "dots", "steps": s}
}

func stepStatus(state string) string {
	switch state {
	case "done":
		return "done"
	case "running", "current":
		return "running"
	case "waiting":
		return "waiting"
	case "on-you":
		return "on-you" // a person's step: the dot is violet
	case "blocked":
		return "blocked"
	case "failed":
		return "failed"
	}
	return "todo"
}

func bucketWords(b string) (word, status string) {
	for _, x := range paneBuckets {
		if x.key == b {
			return x.word, x.status
		}
	}
	return "to do", "todo"
}

func bucketColor(b string) string {
	for _, x := range paneBuckets {
		if x.key == b {
			return bucketTone(x.group)
		}
	}
	return bucketTone(groupToDo)
}

// statusTone colours an increment's standing.
func statusTone(standing string) string {
	switch standing {
	case api.StandingDone, api.StandingShipped:
		return "green"
	case api.StandingOnHold:
		return "yellow"
	case api.StandingDropped:
		return "red"
	case api.StandingDefined:
		return "gray"
	}
	return "orange"
}

// goalWords are the goal card's words, per language.
var goalWords = map[string]map[string]string{
	"en": {
		"label":    "Every delivery goal - its progress, what it became, its open requests, the done ones folded",
		"requests": "requests", "of_done": " / %d done", "no_request": "no request yet", "became": "Became",
		"constraint": "delivery constraint", "checked": "checked when the increment ends", "done": "%d done", "ended": "%d %s", "no_open": "no request open",
		"e_dropped": "dropped", "e_split": "split",
		"b_done": "done", "b_run": "running", "b_wait": "on you", "b_waits": "waiting", "b_todo": "to do", "b_block": "blocked", "b_idle": "nothing left",
	},
	"fr": {
		"label":    "Chaque objectif de livraison - son avancement, ce qu'il est devenu, ses demandes ouvertes, les faites repliées",
		"requests": "demandes", "of_done": " / %d faites", "no_request": "pas encore de demande", "became": "Devenu",
		"constraint": "contrainte de livraison", "checked": "vérifiée à la fin de l'incrément", "done": "%d faites", "ended": "%d %s", "no_open": "aucune demande ouverte",
		"e_dropped": "abandonnées", "e_split": "scindées",
		"b_done": "faites", "b_run": "en cours", "b_wait": "à vous", "b_waits": "en attente", "b_todo": "à faire", "b_block": "bloquées", "b_idle": "plus rien à faire",
	},
}

func goalWord(lang, key string) string {
	if l, ok := goalWords[lang]; ok {
		if s, ok := l[key]; ok {
			return s
		}
	}
	return goalWords["en"][key]
}

// The goal card's header columns (request-rows-mockup.html .goal .gh): the
// key, the goal's words sharing what is left, the progress; and the indent of
// what is under the header - the text column's left edge.
var (
	goalHeadWidths = []int{44, 0, 260}
	goalIndent     = "74px"
)

// goalsTab is the detailed view: each delivery goal as the mock-up's goal
// card (request-rows-mockup.html section 4).
func (p paneData) goalsTab(ctx context.Context, cwm cwmapi.CWMService) node {
	kids := []node{p.header(ctx, cwm), render.SectionLabel(goalWord(p.lang, "label"))}
	for _, g := range p.goals {
		kids = append(kids, p.goalCard(ctx, g))
	}
	return node{"kind": "vbox", "gap": "sm", "data-view": "increment-goals-detail", "children": kids}
}

// goalCard is a delivery goal as the mock-up draws it: a header - its key, its
// words in full, « requests n / m done » over its bar split by state and the
// bar's swatch legend -; « Became » and its chips; its open requests as the
// shared dense row, running first, then on you, blocked, waiting, to do; the
// done ones folded in one line, their chips inside. A delivery constraint (a
// goal that became nothing) is a dashed card with no bar: its tag and
// « checked when the increment ends ».
func (p paneData) goalCard(ctx context.Context, g paneGoalLine) node {
	keyBg, keyColor := "var(--mantine-color-blue-light)", "var(--mantine-color-blue-light-color)"
	if g.Constraint {
		keyBg, keyColor = "var(--mantine-color-gray-light)", "dimmed"
	}
	key := node{"kind": "box", "bg": keyBg, "radius": 6, "p": "4px 0", "data-goal-key": g.Key, "children": []node{
		{"kind": "text", "content": g.Key, "size": "sm", "mono": true, "fw": 700, "ta": "center", "color": keyColor},
	}}
	text := node{"kind": "text", "content": g.Text, "size": "sm", "fw": 600}
	var prog node
	if g.Constraint {
		prog = node{"kind": "vbox", "gap": 5, "align": "flex-end", "data-goal-progress": "constraint", "children": []node{
			{"kind": "badge", "text": goalWord(p.lang, "constraint"), "size": "sm", "variant": "light", "color": "gray"},
			{"kind": "text", "content": goalWord(p.lang, "checked"), "size": "xs", "color": "dimmed", "ta": "right"},
		}}
	} else {
		counted := p.counted(g.Requests)
		nums := []node{render.Dimmed(goalWord(p.lang, "no_request"))}
		if len(counted) == 0 && len(g.Requests) > 0 {
			// Every request ended without shipping (dropped, split): the
			// fold says how; no empty bar.
			nums = []node{render.Dimmed(goalWord(p.lang, "no_open"))}
		}
		if len(counted) > 0 {
			nums = []node{
				render.Dimmed(goalWord(p.lang, "requests")),
				{"kind": "hbox", "gap": 0, "wrap": false, "align": "baseline", "children": []node{
					{"kind": "text", "content": fmt.Sprint(p.doneOf(g.Requests)), "size": "sm", "fw": 700},
					render.Dimmed(fmt.Sprintf(goalWord(p.lang, "of_done"), len(counted))),
				}},
			}
		}
		parts := []node{{"kind": "hbox", "gap": 6, "wrap": false, "justify": "space-between", "align": "baseline", "children": nums}}
		if len(counted) > 0 || len(g.Requests) == 0 {
			parts = append(parts, p.bar(g.Requests))
		}
		prog = node{"kind": "vbox", "gap": 5, "data-goal-progress": "bar", "children": parts}
	}
	head := render.Grid(goalHeadWidths, render.Node(key), render.Node(text), render.Node(prog)).With("gap", 14)
	sec := []node{{"kind": "vbox", "gap": 0, "p": "14px 16px 10px", "children": []node{node(head)}}}

	if len(g.Became) > 0 {
		chips := []node{render.Dimmed(goalWord(p.lang, "became"))}
		for _, h := range g.Became {
			chips = append(chips, idChip(h.ID))
		}
		sec = append(sec, node{"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "p": "0 16px 10px " + goalIndent, "data-goal-became": g.Key, "children": chips})
	}

	// The open requests in the groups' order; the done ones - and those
	// ended without shipping (dropped, split) - in the fold.
	var open []paneRequest
	var closed []string
	done := 0
	endedBy := map[string]int{}
	var endedWords []string
	for _, id := range g.Requests {
		r := p.requests[id]
		switch r.Bucket {
		case "done":
			done++
			closed = append(closed, id)
		case "":
			w, _, _ := r.words()
			if endedBy[w] == 0 {
				endedWords = append(endedWords, w)
			}
			endedBy[w]++
			closed = append(closed, id)
		default:
			open = append(open, r)
		}
	}
	rank := map[string]int{}
	for i, gr := range rowGroups {
		rank[gr] = i
	}
	sort.SliceStable(open, func(a, b int) bool { return rank[rowGroup(open[a])] < rank[rowGroup(open[b])] })
	var body []node
	for _, r := range open {
		body = append(body, requestRow(p.lang, r, openStepOf(ctx, p.lang, r), rowOpts{dense: true, named: true}))
	}
	if len(closed) > 0 {
		label := fmt.Sprintf(goalWord(p.lang, "done"), done)
		for _, w := range endedWords {
			ew := goalWord(p.lang, "e_"+w)
			if ew == "" {
				ew = w
			}
			label += " · " + fmt.Sprintf(goalWord(p.lang, "ended"), endedBy[w], ew)
		}
		var chips []node
		for _, id := range closed {
			chips = append(chips, idChip(id))
		}
		fold := render.Fold{ID: "done-" + g.Key, Label: label, Data: map[string]string{"data-goal-done": g.Key},
			Content: node{"kind": "hbox", "gap": 6, "wrap": true, "align": "center", "children": chips}}
		body = append(body, node(render.Accordion([]render.Fold{fold}).RuleLook()))
	}
	if len(body) > 0 {
		sec = append(sec, node{"kind": "vbox", "gap": 6, "p": "4px 16px 14px " + goalIndent, "children": body})
	}

	card := render.Card(nodesOf(sec)...).With("p", 0).With("gap", 0).With("radius", "md").With("data-goal", g.Key)
	if g.Constraint {
		card = card.Dashed()
	}
	return node(card)
}

// nodesOf are page nodes as render nodes.
func nodesOf(ns []node) []render.Node {
	out := make([]render.Node, len(ns))
	for i, n := range ns {
		out[i] = render.Node(n)
	}
	return out
}
