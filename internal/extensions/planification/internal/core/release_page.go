// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// incrementWaits says why an increment is not delivered: what it waits on, one
// line each - its hold, an increment it depends on still open, a request
// blocked or waiting on a person, a goal or feature no work serves - or, when
// its work recorded it done (realized, assumed), awaiting a person's
// confirmation. Nothing when it is done or dropped, or when its work runs.
func incrementWaits(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) []string {
	return incrementWaitsIn(ctx, cwm, o, "")
}

// incrementWaitsIn is incrementWaits in lang (waitWord).
func incrementWaitsIn(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, lang string) []string {
	inc := toIncrement(ctx, o)
	if inc.State != api.IncrementOpen {
		return nil
	}
	var out []string
	if inc.Held != nil {
		out = append(out, waitWord(lang, "on_hold")+inc.Held.Reason)
	}
	for _, d := range inc.DependsOn {
		if do, _, err := loadAny(ctx, cwm, d); err == nil && releaseState(do) == api.IncrementOpen {
			out = append(out, waitWord(lang, "waits_on")+titleOf(do))
		}
	}
	if hasRequest(o) {
		if w := requestWaitsIn(ctx, o, waitWord(lang, "its_proposal"), lang); w != "" {
			out = append(out, w)
		}
	}
	scope, err := scopeOf(ctx, cwm, o)
	if err != nil {
		return out
	}
	for _, t := range scope.Targets {
		if t.Done || uncounted(t.State) {
			continue
		}
		serving, _ := incoming(ctx, cwm, t.ID, api.EdgeServes)
		serving = append([]string{t.ID}, serving...)
		worked := false
		for _, r := range serving {
			ro, err := loadObject(ctx, cwm, r)
			if err != nil || !hasRequest(ro) || planEnded(stateOf(ro)) {
				continue
			}
			worked = true
			if w := requestWaitsIn(ctx, ro, titleOf(ro), lang); w != "" {
				out = append(out, w)
			}
		}
		switch {
		case worked:
		case confirmable[t.Type].state != "" && t.State == confirmable[t.Type].state:
			// Recorded done by its work: plan-work files nothing for it,
			// only the person's confirm moves it on.
			out = append(out, t.Title+waitWord(lang, "awaits_confirm"))
		default:
			out = append(out, t.Title+waitWord(lang, "no_work"))
		}
	}
	return out
}

// requestWaits says what a request waits on, named as `name`: blocked (its
// plan's note, then what the one reader says holds it), or a person's step of
// it ready (their answer, in the Inbox); empty while an agent's step is next
// or runs.
func requestWaits(ctx context.Context, o cwmapi.Object, name string) string {
	return requestWaitsIn(ctx, o, name, "")
}

// requestWaitsIn is requestWaits in lang.
func requestWaitsIn(ctx context.Context, o cwmapi.Object, name, lang string) string {
	p, _ := planOf(o)
	if p.State == api.PlanBlocked {
		why := p.Note
		if why == "" {
			why = waitWord(lang, "step_failed")
		}
		if b, ok := holdOf(ctx, o); ok {
			// The block's own words, then what holds it as planning check
			// names it.
			held := holdWords(b)
			if deliveryLocal(lang) {
				held = blockerSay(lang, held)
			}
			why += " - " + held
		}
		return name + waitWord(lang, "blocked") + why
	}
	todos, _ := todosOf(ctx, o.ID)
	for _, t := range todos {
		if t.For == schedapi.ForPerson && t.State == schedapi.StateReady {
			ask := t.Packet.Interaction
			if ask == "" {
				ask = "answer"
			}
			if deliveryLocal(lang) {
				if w, ok := waitWords[lang]["ask_"+ask]; ok {
					return name + w
				}
				return name + fmt.Sprintf(waitWords[lang]["ask"], ask)
			}
			return name + " awaits a person's " + ask
		}
	}
	return ""
}

// Page answers a release's pane: its overview (
// modules/portfolio/release-overview.md) - can it ship, what is left, what
// needs a person, what runs now. Its header gives its stage, its verdict and
// its next action; its gauges, its checklist (the gates), its increments by
// priority with what is left and what they wait on, the increments of other
// streams they need, and the work running or blocked follow. Every answer is
// planification's own: releases gates, the increments' summaries and waits,
// the to-dos of its requests. The header and its actions are drawn at once and the rest is a section loaded on its own
// (F-098): its work, read in one computation.
func (r *Releases) Page(ctx context.Context, in api.GetInput) (render.Def, error) {
	ctx = withReadCache(ctx) // every part reads the same listing and queue, once
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	p, err := readReleasePage(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	children := p.head(ctx, cwm)
	children = append(children, loadedSection("release-work", releasePageSectionURI,
		node{"id": in.ID, "section": api.ReleasePageWork}, "", render.PlaceholderRows, 8))
	return render.Def{Kind: "page", Title: titleOf(p.rel), DSLTree: node{"kind": "vbox", "gap": "lg", "children": children}}, nil
}

// PageSection answers one section of a release's pane, loaded on its own
// : its work -
// what the pane built at once drew below its actions, its three reads (the
// increments' summaries, its requests' to-dos, « À faire » for it) run side
// by side, each once.
func (r *Releases) PageSection(ctx context.Context, in api.ReleasePageSectionInput) (render.Def, error) {
	if in.Section != api.ReleasePageWork {
		return render.Def{}, invalid("PageSection: no section %q (work)", in.Section)
	}
	ctx = withReadCache(ctx)
	cwm, err := cwmOf(ctx)
	if err != nil {
		return render.Def{}, err
	}
	p, err := readReleasePage(ctx, cwm, in.ID)
	if err != nil {
		return render.Def{}, err
	}
	return render.Def{Kind: "page", DSLTree: node{"kind": "vbox", "gap": "lg", "children": p.work(ctx, cwm, p.readWork(ctx, cwm))}}, nil
}

// releasePageSectionURI loads one section of a release's pane.
const releasePageSectionURI = "ktw-agents:releases/page-section"

// releasePage is what every part of a release's pane reads first: the
// release, its gates, its stream, what it carries - the done ones last, then
// by priority - and what rides along.
type releasePage struct {
	rel     cwmapi.Object
	rr      api.Release
	state   string
	gates   api.GatesOutput
	sm      string
	carried []cwmapi.Object
	along   []cwmapi.Object
}

func readReleasePage(ctx context.Context, cwm cwmapi.CWMService, id string) (releasePage, error) {
	rel, err := loadRelease(ctx, cwm, id)
	if err != nil {
		return releasePage{}, err
	}
	p := releasePage{rel: rel, rr: toRelease(rel), state: releaseState(rel)}
	if p.gates, err = gatesOf(ctx, cwm, rel, ""); err != nil {
		return releasePage{}, err
	}
	p.sm = edgeTo(rel, api.EdgeInStream)
	p.carried = carriedIncrements(ctx, cwm, rel)
	sort.SliceStable(p.carried, func(i, j int) bool {
		if di, dj := releaseState(p.carried[i]) == api.IncrementDone, releaseState(p.carried[j]) == api.IncrementDone; di != dj {
			return dj // the done ones fold to the bottom
		}
		pi, _ := planOf(p.carried[i])
		pj, _ := planOf(p.carried[j])
		if pi.Priority != pj.Priority {
			return priorityRank(pi.Priority) < priorityRank(pj.Priority)
		}
		return p.carried[i].ID < p.carried[j].ID
	})
	if p.state == api.ReleasePlanned && p.sm != "" {
		if next, ok := nextPlannedRelease(ctx, cwm, p.sm); ok && next == rel.ID {
			p.along = ridingAlong(ctx, cwm, p.sm)
		}
	}
	return p, nil
}

// head is the pane's header - stage, verdict, next action - and what its
// stage allows.
func (p releasePage) head(ctx context.Context, cwm cwmapi.CWMService) []node {
	rel, rr, state, gates, sm := p.rel, p.rr, p.state, p.gates, p.sm
	// The verdict follows the stage: ready to freeze (what it must carry is
	// done), ready for a candidate (and no open P0 fix), ready to promote
	// (every gate on its candidate).
	asked, ask := stageGates(gates.Stage)
	var open []string
	for _, g := range gates.Gates {
		if !g.Passed && slices.Contains(asked, g.Name) {
			open = append(open, g.Name+" ("+g.Why+")")
		}
	}
	verdict, color := "READY "+ask, "green"
	switch {
	case state == api.ReleaseReleased:
		verdict, open = "RELEASED", nil
	case state == api.ReleaseDropped:
		verdict, color, open = "DROPPED", "gray", nil
	case len(open) > 0:
		verdict, color = "NOT READY "+ask, "orange"
	}
	line := []string{"stage " + gates.Stage}
	if rr.Version != "" {
		line = append(line, "version "+rr.Version)
	}
	if rr.Date != "" {
		line = append(line, "target "+rr.Date+daysLeft(rr.Date))
	}
	if sm != "" {
		line = append(line, "stream "+titleOfID(ctx, cwm, sm))
	}
	if gates.Candidate != "" {
		line = append(line, "candidate "+gates.Candidate+buildOf(gates.Build))
	}
	verdictText := verdict
	if len(open) > 0 {
		verdictText += fmt.Sprintf(" - %d open: %s", len(open), strings.Join(open, "; "))
	}
	head := []node{
		{"kind": "hbox", "gap": 6, "wrap": true, "children": []node{
			{"kind": "badge", "text": verdictText, "variant": "filled", "color": color, "tooltip": rel.ID},
			{"kind": "badge", "text": rel.ID, "variant": "outline", "color": "gray", "size": "xs", "keepCase": true, "mono": true},
		}},
		{"kind": "text", "content": strings.Join(line, " · "), "size": "sm"},
		{"kind": "text", "content": "Next: " + nextAction(state, gates), "size": "sm", "fw": 600},
	}
	if sm != "" {
		name := titleOfID(ctx, cwm, sm)
		head = append(head, render.Link("Stream "+name, render.NavigateTo("Stream "+name, sm, "")))
	}
	children := []node{section(titleOf(rel), "package", "indigo", "", node{"kind": "vbox", "gap": "xs", "children": head})}
	if acts := releaseActions(rr, state, gates); acts != nil {
		children = append(children, section("Act on it", "player-play", "grape", "What its stage allows, as you: each refused with its reason when a gate does not allow it", acts))
	}
	return children
}

// releaseWork is what the pane's work counts and lists, read once: the
// increments' summaries, its requests with their to-dos, and what awaits a
// person - « À faire » for this release.
type releaseWork struct {
	sums                        map[string]api.IncrementSummaryOutput
	sumErr                      map[string]error
	feats, featsTotal, incsDone int
	work                        []requestWork
	reqs                        workSummary // each request once, counted as an increment counts its own
	needs                       []node
	queueErr                    error
}

// readWork reads the pane's work: its three reads side by side - they share
// only the view's read cache, which is safe to share.
func (p releasePage) readWork(ctx context.Context, cwm cwmapi.CWMService) releaseWork {
	w := releaseWork{sums: map[string]api.IncrementSummaryOutput{}, sumErr: map[string]error{}}
	summaries := func() {
		incs := NewIncrements()
		for _, o := range p.carried {
			s, err := incs.Summary(ctx, api.GetInput{ID: o.ID})
			if err != nil {
				w.sumErr[o.ID] = err
				continue
			}
			w.sums[o.ID] = s
			w.feats, w.featsTotal = w.feats+s.Done, w.featsTotal+s.Total
			if s.State == api.IncrementDone {
				w.incsDone++
			}
		}
	}
	// Needs a person, running, blocked: the to-dos of its requests.
	var todosErr error
	requests := func() {
		for _, id := range releaseRequests(ctx, cwm, p.rel, p.carried) {
			ro, err := loadObject(ctx, cwm, id)
			if err != nil {
				continue
			}
			w.reqs.add(ctx, ro)
			todos, err := todosOf(ctx, id)
			if err != nil && todosErr == nil {
				todosErr = err
			}
			pl, _ := planOf(ro)
			w.work = append(w.work, requestWork{id: id, title: titleOf(ro), plan: pl, todos: todos})
		}
	}
	// What awaits a person: « À faire » scoped to this release - the same
	// items through the same function, read from the
	// release's demands only.
	var queueErr error
	awaiting := func() {
		items, err := ObjectQueueItems(ctx, p.rel.ID, "")
		if err != nil {
			queueErr = err
			return
		}
		for _, it := range items {
			if it.Held != nil || it.Action == api.ActionNotice {
				continue
			}
			meta := it.Asks
			if it.IncrementTitle != "" {
				meta = it.IncrementTitle + " · " + meta
			}
			w.needs = append(w.needs, node{"id": it.Object, "title": it.Action + " · " + it.Title, "state": "awaiting a person", "meta": meta})
		}
	}
	var wg sync.WaitGroup
	for _, read := range []func(){summaries, requests, awaiting} {
		wg.Add(1)
		go func() { defer wg.Done(); read() }()
	}
	wg.Wait()
	w.queueErr = todosErr
	if queueErr != nil {
		w.queueErr = queueErr
	}
	return w
}

// work is the pane below its actions: its gauges, its gates, what awaits a
// person, its increments (or, released, what it delivered, its candidates
// and its change log), the other streams' increments it needs, and what runs
// and what is blocked.
func (p releasePage) work(ctx context.Context, cwm cwmapi.CWMService, w releaseWork) []node {
	rel, state, gates, sm := p.rel, p.state, p.gates, p.sm
	carried := p.carried
	var children []node
	depsOpen, depsTotal := dependenciesOf(ctx, cwm, rel, carried)
	_, running, blocked := workRows(w.work)
	reqs, needs, queueErr := w.reqs, w.needs, w.queueErr

	bar := func(label string, done, total int, color string) node {
		return node{"kind": "gauge", "label": label, "value": done, "max": max(1, total), "valueText": fmt.Sprintf("%d/%d", done, total), "color": color, "size": "sm"}
	}
	children = append(children, node{"kind": "grid", "cols": 5, "gap": "md", "children": []node{
		bar("Increments", w.incsDone, len(carried), "teal"),
		bar("Dependencies", depsTotal-depsOpen, depsTotal, "indigo"),
		bar("Features", w.feats, w.featsTotal, "green"),
		{"kind": "gauge", "label": "Requests", "legend": true, "size": "sm",
			"valueText": fmt.Sprintf("%d/%d", reqs.done, reqs.total()),
			"segments": []node{
				{"value": reqs.done, "color": "green", "label": "done"},
				{"value": reqs.running, "color": "blue", "label": "in progress"},
				{"value": reqs.todo, "color": "cyan", "label": "to do"},
				{"value": reqs.awaiting, "color": "grape", "label": "awaiting a person"},
				{"value": reqs.blocked, "color": "red", "label": "blocked"},
			}},
		kpi("Awaiting a person", len(needs), "steps a person answers", "grape", "user-check"),
	}})

	// Ready to ship? The gates, each with why.
	var checks []string
	for _, g := range gates.Gates {
		mark := "✓"
		if !g.Passed {
			mark = "✗"
		}
		l := "- " + mark + " **" + g.Name + "**"
		if g.Why != "" {
			l += " - " + g.Why
		}
		checks = append(checks, l)
	}
	if len(gates.Filed) > 0 {
		var filed []string
		for _, id := range gates.Filed {
			filed = append(filed, titleOfID(ctx, cwm, id))
		}
		checks = append(checks, "", "Filed since the candidate: "+strings.Join(filed, ", "))
	}
	children = append(children, section("Ready to ship?", "checklist", "teal", "Every gate of the verdict, on its last candidate", node{"kind": "markdown", "content": strings.Join(checks, "\n")}))

	openItem := node{"label": "Open", "navigate": node{"object": "${item.id}"}}
	list := func(key string, rows []node, empty string) node {
		if len(rows) == 0 {
			return render.Dimmed(empty)
		}
		return node{"kind": "tree", "data": rows, "item": itemTitles(key), "badgeField": "state", "open": openItem}
	}
	nothing := func(empty string) string { return unknownOr(queueErr, empty) }
	children = append(children, section("Awaiting a person", "user-check", "grape", "What « To do » holds for this release, by action then oldest first", list("awaiting", needs, nothing("Nothing awaits a person."))))

	var rows []node
	for _, o := range carried {
		if err := w.sumErr[o.ID]; err != nil {
			rows = append(rows, node{"id": o.ID, "title": titleOf(o), "state": releaseState(o), "meta": "its summary could not be read: " + err.Error()})
			continue
		}
		s := w.sums[o.ID]
		meta := fmt.Sprintf("%s · %d/%d held done · requests %d done, %d in progress, %d to do, %d waiting, %d awaiting, %d blocked, %d with nothing left", s.Priority, s.Done, s.Total, s.RequestsDone, s.Running, s.ToDo, s.WaitingOn, s.Awaiting, s.Blocked, s.Idle)
		if left := leftOf(ctx, cwm, o); len(left) > 0 {
			meta += " · left: " + strings.Join(left, ", ")
		}
		if len(s.Waits) > 0 {
			meta += " · waits: " + strings.Join(s.Waits, "; ")
		}
		standing, _ := api.StandingLabel(s.Standing, false)
		row := node{"id": o.ID, "title": titleOf(o), "state": standing, "meta": meta}
		if releaseState(o) == api.IncrementOpen {
			row["open_increment"] = true
			for _, pr := range api.Priorities {
				if pr != s.Priority {
					row["priority_not_"+pr] = true
				}
			}
		}
		rows = append(rows, row)
	}
	var alongRows []node
	for _, o := range p.along {
		alongRows = append(alongRows, node{"id": o.ID, "title": titleOf(o), "state": releaseState(o), "meta": "done, in no release: its freeze takes it in"})
	}
	if state == api.ReleaseReleased || state == api.ReleaseDropped {
		children = append(children, section("What it delivered", "circle-check", "green", "The increments it shipped", list("delivered", rows, "It shipped no increment.")))
		var cands []node
		for _, c := range releaseFields(rel).Candidates {
			meta := "delivered " + c.At + buildOf(c.Build)
			if c.Promoted {
				meta += " · promoted: the official version"
			}
			cands = append(cands, node{"id": rel.ID, "title": c.Name, "state": map[bool]string{true: "promoted", false: "candidate"}[c.Promoted], "meta": meta})
		}
		children = append(children, section("Its candidates", "versions", "indigo", "Its deliveries, in order", list("candidates", cands, "It had no candidate.")))
		if note, err := noteOf(ctx, cwm, rel); err != nil {
			children = append(children, section("Its change log", "file-text", "teal", "The change entries of what it carried (releases note)",
				node{"kind": "text", "content": "Its change log could not be read: " + err.Error(), "color": "red", "size": "sm"}))
		} else {
			var lines []string
			for _, in := range note.Increments {
				for _, c := range in.Changes {
					lines = append(lines, "- "+c.Title+" ("+titleOfID(ctx, cwm, in.ID)+")")
				}
			}
			for _, c := range note.Fixes {
				lines = append(lines, "- "+c.Title+" (correction)")
			}
			body := node(render.Dimmed("No change entry."))
			if len(lines) > 0 {
				body = node{"kind": "markdown", "content": strings.Join(lines, "\n")}
			}
			children = append(children, section("Its change log", "file-text", "teal", "The change entries of what it carried (releases note)", body))
		}
	} else {
		incList := list("increments", rows, "It carries no increment yet.")
		if len(rows) > 0 {
			incList["actions"] = incrementRowActions(ctx, cwm, rel, sm)
		}
		children = append(children, section("Increments of this release", "stack-2", "teal", "By priority: what each holds done, its requests and what it waits on - and, on each, change its release, take it out, hold it, set its priority", incList))
		if len(alongRows) > 0 {
			children = append(children, section("Riding along", "arrow-right", "gray", "Done increments of its stream in no release: its freeze takes them in", list("riding-along", alongRows, "")))
		}
		if sec := plannedSection(ctx, cwm, rel, state, list); sec != nil {
			children = append(children, sec)
		}
	}
	children = append(children, section("Dependencies (other streams)", "git-merge", "indigo", "Increments of other streams one of its increments depends on", list("dependencies", dependencyRows(ctx, cwm, rel, carried), "It depends on no other stream.")))
	children = append(children, node{"kind": "grid", "cols": 2, "gap": "md", "children": []node{
		section("Running now", "player-play", "blue", "Steps claimed now, by whom and since when", list("running", running, nothing("Nothing runs."))),
		section("Blocked", "alert-triangle", "red", "Requests blocked or failed, and why", list("blocked", blocked, nothing("Nothing is blocked."))),
	}})
	return children
}

// stageGates is the gates a release's stage asks, and what it is ready for.
func stageGates(stage string) ([]string, string) {
	switch stage {
	case api.ReleasePlanned:
		return []string{api.GateIncrements, api.GateDependencies}, "TO FREEZE"
	case api.ReleaseFrozen:
		return []string{api.GateIncrements, api.GateDependencies, api.GateNoOpenP0Fix}, "FOR A CANDIDATE"
	}
	return []string{api.GateIncrements, api.GateDependencies, api.GateNoOpenP0Fix, api.GateEndToEnd, api.GateSignOff, api.GateChangeLog}, "TO PROMOTE"
}

// leftOf is what is left of an increment: its open requests, each with its
// next step.
func leftOf(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) []string {
	var out []string
	for _, id := range incrementRequests(ctx, cwm, inc) {
		o, err := loadObject(ctx, cwm, id)
		if err != nil || !hasRequest(o) || planEnded(planState(o)) || id == inc.ID {
			continue
		}
		step := planState(o)
		if t := currentTodo(ctx, id); t != nil && t.Packet.Step != "" {
			step = t.Packet.Step
		}
		out = append(out, titleOf(o)+" ("+step+")")
	}
	return out
}

// nextAction is what a person does next on a release, by its stage.
func nextAction(state string, g api.GatesOutput) string {
	switch {
	case state == api.ReleasePlanned:
		return "freeze it once what it must carry is in (Act on it)"
	case state == api.ReleaseFrozen && g.Candidate == "":
		return "deliver a candidate (Act on it)"
	case state == api.ReleaseFrozen && g.Ready:
		return "promote " + g.Candidate + " (Act on it)"
	case state == api.ReleaseFrozen && onlyChangeLogOpen(g):
		// Promote is what starts the change log: the release's guard refuses
		// it, its change-log work is filed, then the person confirms it.
		return "promote " + g.Candidate + " (Act on it) - its change log is written first, then you confirm it in the Inbox, which promotes it"
	case state == api.ReleaseFrozen:
		return "close its open gates, then deliver another candidate or promote " + g.Candidate
	}
	return "nothing - it is " + state
}

// onlyChangeLogOpen is whether every gate passes on the candidate but the
// release's change log.
func onlyChangeLogOpen(g api.GatesOutput) bool {
	open := false
	for _, l := range g.Gates {
		if !l.Passed {
			if l.Name != api.GateChangeLog {
				return false
			}
			open = true
		}
	}
	return open && g.Candidate != ""
}

// daysLeft says how many days remain to a date (YYYY-MM-DD).
func daysLeft(date string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	n := int(time.Until(d).Hours() / 24)
	if n < 0 {
		return fmt.Sprintf(" (%d days late)", -n)
	}
	return fmt.Sprintf(" (%d days left)", n)
}

func buildOf(b string) string {
	if b == "" {
		return ""
	}
	return " (build " + b + ")"
}

// releaseRequests is every request about a release: its own and its direct
// work, and those of the increments it carries (their own and what serves
// what they hold).
func releaseRequests(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, carried []cwmapi.Object) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if hasRequest(rel) {
		add(rel.ID)
	}
	direct, _ := workOf(ctx, cwm, rel.ID)
	for _, id := range direct {
		add(id)
	}
	for _, inc := range carried {
		for _, id := range incrementRequests(ctx, cwm, inc) {
			add(id)
		}
	}
	return out
}

// incrementRequests is the requests of one increment: those targeting it, the
// increment itself when it carries one, and those on its goals and features
// (its scope) or serving them.
func incrementRequests(ctx context.Context, cwm cwmapi.CWMService, inc cwmapi.Object) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	targeting, _ := workOf(ctx, cwm, inc.ID)
	for _, id := range targeting {
		add(id)
	}
	if hasRequest(inc) {
		add(inc.ID)
	}
	scope, err := scopeOf(ctx, cwm, inc)
	if err != nil {
		return out
	}
	for _, t := range scope.Targets {
		serving, _ := incoming(ctx, cwm, t.ID, api.EdgeServes)
		for _, r := range append([]string{t.ID}, serving...) {
			if o, err := loadObject(ctx, cwm, r); err == nil && hasRequest(o) {
				add(r)
			}
		}
	}
	return out
}

// requestWork is what the overview reads of one request: its plan and its
// to-dos.
type requestWork struct {
	id, title string
	plan      api.PlanFields
	todos     []api.WorkTodo
}

// workRows sorts a release's requests into what needs a person (oldest
// first), what runs and what is blocked or failed - a request once under
// Blocked, its plan's block first.
func workRows(work []requestWork) (needs, running, blocked []node) {
	for _, w := range work {
		why := ""
		if w.plan.State == api.PlanBlocked {
			why = w.plan.Note
			blocked = append(blocked, node{"id": w.id, "title": w.title, "state": "blocked", "meta": why})
		}
		failed := false
		for _, t := range w.todos {
			switch {
			case t.For == schedapi.ForPerson && t.State == schedapi.StateReady:
				needs = append(needs, node{"id": w.id, "title": t.Packet.Interaction + " · " + w.title, "state": "awaiting a person", "meta": "step " + t.Packet.Step + " - since " + t.Since, "since": t.Since})
			case t.State == schedapi.StateClaimed && t.Claim != nil:
				running = append(running, node{"id": w.id, "title": w.title, "state": "running", "meta": t.Packet.Step + " · " + t.Claim.By.ID + " · since " + t.Claim.At})
			case stopStatus(t) == schedapi.StatusFail && w.plan.State != api.PlanBlocked && !failed:
				failed = true
				stop, summary := stopOf(t)
				blocked = append(blocked, node{"id": w.id, "title": w.title, "state": "failed", "meta": t.Packet.Step + " - " + stop + ": " + summary})
			}
		}
	}
	sort.SliceStable(needs, func(i, j int) bool { return before(needs[i]["since"].(string), needs[j]["since"].(string)) })
	return needs, running, blocked
}

// unknownOr is a list's empty text, or that it is unknown when the work queue
// could not be read: an empty list is never shown as if it were true.
func unknownOr(queueErr error, empty string) string {
	if queueErr != nil {
		return "Unknown - the work queue could not be read: " + queueErr.Error()
	}
	return empty
}

// dependenciesOf counts the increments of other streams the carried ones depend
// on: open, and in all.
func dependenciesOf(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, carried []cwmapi.Object) (open, total int) {
	for _, r := range dependencyRows(ctx, cwm, rel, carried) {
		total++
		if r["state"] != api.IncrementDone && r["state"] != api.IncrementDropped {
			open++
		}
	}
	return open, total
}

// dependencyRows is the increments of other streams the carried ones depend
// on, each with its stream.
func dependencyRows(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, carried []cwmapi.Object) []node {
	sm := edgeTo(rel, api.EdgeInStream)
	seen := map[string]bool{}
	var rows []node
	for _, inc := range carried {
		for _, d := range edgesTo(inc, api.EdgeDependsOn) {
			do, _, err := loadAny(ctx, cwm, d)
			if err != nil || do.Type != api.IncrementType || seen[d] || (sm != "" && edgeTo(do, api.EdgeInStream) == sm) {
				continue
			}
			seen[d] = true
			meta := "stream " + titleOfID(ctx, cwm, edgeTo(do, api.EdgeInStream)) + " · needed by " + titleOf(inc)
			if r := edgeTo(do, api.EdgeShipsIn); r != "" {
				meta += " · ships in " + titleOfID(ctx, cwm, r)
			}
			rows = append(rows, node{"id": d, "title": titleOf(do), "state": releaseState(do), "meta": meta})
		}
	}
	return rows
}

// titleOfID is what an object is read by on a page: its name, a release's
// version and name; its id only when it cannot be read.
func titleOfID(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	if id == "" {
		return ""
	}
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil || o.Name == "" {
		return id
	}
	if v := releaseFields(o).Version; v != "" && o.Type == api.ReleaseType {
		return api.ReleaseLabel(v, o.Name)
	}
	return o.Name
}

// itemTitles is the template of a tree row on a release's pane: the title and
// a dimmed line - the id only opens it. key names the tree; its note binds the
// row's id, so each row's toggle is its own: text:<key>-meta:<item id>:toggle
func itemTitles(key string) node {
	meta := noteText(key+"-meta", "", "xs", "dimmed")
	meta["dataField"] = "$.item.meta"
	meta["idField"] = "$.item.id"
	return node{"kind": "vbox", "gap": 2, "children": []node{
		{"kind": "text", "content": "", "dataField": "$.item.title", "size": "sm", "fw": 500},
		{"kind": "text", "content": "", "dataField": "$.item.id", "size": "xs", "color": "gray", "ff": "monospace"},
		// A block or refusal note can be long: three lines, the rest behind
		// « Afficher plus » (text:<key>-meta:<item id>:toggle).
		meta,
	}}
}

// noteText is a note that can be long - why a request is blocked, why its end
// was refused: clamped to three lines, with a toggle « Afficher plus » /
// « Afficher moins » drawn only when it overflows, its element id
// text:<id>:toggle.
func noteText(id, content, size, color string) node {
	t := render.Text(content).Clamp(3).Expandable().With("id", id).With("size", size)
	if color != "" {
		t = t.With("color", color)
	}
	return node(t)
}

// incrementRow is an increment's tree row: its title, state and progress.
func incrementRow(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) node {
	meta := ""
	if scope, err := scopeOf(ctx, cwm, o); err == nil {
		meta = fmt.Sprintf("%d of %d done", scope.Done, scope.Total)
	}
	if p, ok := planOf(o); ok && p.Priority != "" {
		meta = p.Priority + " · " + meta
	}
	return node{"id": o.ID, "title": titleOf(o), "state": releaseState(o), "meta": meta}
}

// plannedSection lists the increments planned for a release, apart from those
// shipping in it - those held from it among them, with Resume; once it is its stream's next planned release, one action
// ships them all. Nil when none is planned for it.
func plannedSection(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, state string, list func(string, []node, string) node) node {
	planned, err := plannedFor(ctx, cwm, rel.ID)
	if err != nil {
		return section("Planned for this release", "calendar-plus", "violet", "Increments aimed at it: they ship in it once it is its stream's next release",
			node{"kind": "text", "content": "What is planned for it could not be read: " + err.Error(), "color": "red", "size": "sm"})
	}
	if len(planned) == 0 {
		return nil
	}
	var rows []node
	for _, o := range planned {
		inc := toIncrement(ctx, o)
		label, _ := api.StandingLabel(api.IncrementStanding(api.IncrementStandingInput{Increment: inc}), false)
		row := node{"id": o.ID, "title": titleOf(o), "state": label, "meta": "planned for it: ships in it once it is its stream's next release"}
		if inc.Held != nil {
			row["held"] = true
			row["meta"] = "on hold: " + inc.Held.Reason + " - planned for it, it ships in it once resumed"
		}
		rows = append(rows, row)
	}
	tree := list("planned", rows, "")
	tree["actions"] = []node{
		{"id": "resume", "label": "Resume", "icon": "player-play", "when": "held", "uri": incrementResumeURI, "args": node{"id": "${item.id}", "by": "${person}"}},
		{"id": "unplan", "label": "Unplan", "icon": "minus", "uri": incrementUnplanURI, "args": node{"id": "${item.id}"},
			"confirm": "Take it off this release? It waits in its stream's backlog."},
	}
	kids := []node{tree}
	if next, ok := nextPlannedRelease(ctx, cwm, edgeTo(rel, api.EdgeInStream)); state == api.ReleasePlanned && ok && next == rel.ID {
		label := fmt.Sprintf("Ship the %d planned increment(s)", len(planned))
		kids = append(kids, render.Button(label, render.RunURI(label, releaseShipPlannedURI, map[string]any{"id": rel.ID, "by": "${person}"})))
	}
	return section("Planned for this release", "calendar-plus", "violet", "Increments aimed at it: they ship in it once it is its stream's next release", node{"kind": "vbox", "gap": "xs", "children": kids})
}

// incrementRowActions are what a person does to an increment from its
// release's pane, each on its row as the person: move it
// to another planned release of the stream (its next one ships it in, a later
// one plans it), take it out, put it on hold, set its priority. An action
// shows only on a row it applies to (its `when` field).
func incrementRowActions(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, stream string) []node {
	var acts []node
	if stream != "" {
		next, _ := nextPlannedRelease(ctx, cwm, stream)
		rels, _ := inStream(ctx, cwm, stream, api.ReleaseType)
		sort.Slice(rels, func(i, j int) bool { return releaseFields(rels[i]).Date < releaseFields(rels[j]).Date })
		for _, r := range rels {
			if r.ID == rel.ID || releaseState(r) != api.ReleasePlanned {
				continue
			}
			label := "Plan for " + titleOf(r)
			if r.ID == next {
				label = "Ship in " + titleOf(r)
			}
			acts = append(acts, node{"id": "move-" + r.ID, "label": label, "icon": "arrows-exchange", "when": "open_increment",
				"uri": incrementPlanForURI, "args": node{"id": "${item.id}", "in": r.ID, "by": "${person}"}})
		}
	}
	acts = append(acts,
		node{"id": "take-out", "label": "Take out", "icon": "minus", "when": "open_increment", "uri": incrementUnshipURI,
			"args": node{"id": "${item.id}"}, "confirm": "Take it out of this release? It waits in its stream's backlog."},
		node{"id": "hold", "label": "Hold", "icon": "player-pause", "when": "open_increment", "uri": incrementHoldURI,
			"args":    node{"id": "${item.id}", "reason": "held from the pane of " + titleOf(rel), "by": "${person}"},
			"confirm": "Put it on hold? Its work is not pulled and it leaves this release until resumed."})
	for _, p := range api.Priorities {
		acts = append(acts, node{"id": "priority-" + p, "label": "Priority " + p, "icon": "arrows-sort", "when": "priority_not_" + p,
			"uri": requestAssessURI, "args": node{"id": "${item.id}", "priority": p, "by": "${person}"}})
	}
	return acts
}
