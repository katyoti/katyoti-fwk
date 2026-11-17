// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// THE PLANNING DECK — one page over what planification knows: the plans, the
// demands, the task groups, the releases. It is composed ONLY of the page
// DSL's generic nodes (tabs, kpi, lanes, list, tree, gauge, gantt, graph,
// button), so nothing in the GUI is planification's; its actions are
// declarative, run by the panel's host with `${item.*}`, `${lane}` and
// `${person}` — or, a `tab` action, by the page itself. Declared as a global panel (its target the Global selector's)
// and on every module (its id).

// The deck's action URIs: the verbs it offers, each a published method.
const (
	deckPlanURI     = "ktw-agents:plans/plan"
	deckFinishURI   = "ktw-agents:plans/finish"
	deckTargetURI   = "ktw-agents:releases/target"
	deckUntargetURI = "ktw-agents:releases/untarget"
	deckRetryURI    = "ktw-agents:work/retry"
	deckPauseURI    = "ktw-agents:plans/pause"
	deckResumeURI   = "ktw-agents:plans/resume"
	deckBacklog     = "backlog"
)

type node = map[string]any

// standRunning is where a request stands while one of its steps runs (a to-do
// pulled): read from its to-dos, never a plan state.
const standRunning = "running"

// standLapsed is where a request stands while its pulled step's claim is past
// its until: its holder went silent - never shown as running.
const standLapsed = "lapsed"

// stateColors are the plan states' badge colors — and running's — one palette
// everywhere.
var stateColors = map[string]string{
	api.PlanReady: "teal", standRunning: "blue", standLapsed: "yellow", api.PlanIdle: "orange", api.PlanBlocked: "red",
	api.PlanSplit: "violet", api.PlanDone: "green", api.PlanDropped: "gray",
}

// stateLabels are what a person reads for a plan state — and for running.
var stateLabels = map[string]string{
	api.PlanReady: "ready", standRunning: "running", standLapsed: "lapsed", api.PlanIdle: "waiting", api.PlanBlocked: "blocked",
	api.PlanSplit: "split", api.PlanDone: "done", api.PlanDropped: "dropped",
}

// Deck answers the planning deck of a module (target, else id) or of every
// module.
func (v *Views) Deck(ctx context.Context, in api.DeckInput) (render.Def, error) {
	ctx = withReadCache(ctx) // every part reads the same listing and queue, once
	module, focus, scope := deckScope(ctx, in.Target, in.ID)
	// The tabs a person opens less often - History, Deliveries, Task groups -
	// are sections loaded when they open: the deck reads
	// neither the ended demands nor the task groups to open.
	plans, err := NewPlans().List(ctx, api.ListPlansInput{Module: module})
	if err != nil {
		return render.Def{}, err
	}
	svc := New()
	open, err := svc.Demands(ctx, api.DemandsInput{Module: module})
	if err != nil {
		return render.Def{}, err
	}
	if scope != nil {
		plans.Plans = inScopePlans(plans.Plans, scope)
		open.Demands = inScopeDemands(open.Demands, scope)
	}
	asked := map[string]bool{}
	questioned := openQuestions(ctx, nil)
	var walkAsked func(ds []api.Demand)
	walkAsked = func(ds []api.Demand) {
		for _, d := range ds {
			if (questioned[d.ID] || len(d.AskedIn) > 0) && planIdle(d) {
				asked[d.ID] = true
			}
			walkAsked(d.Parts)
		}
	}
	walkAsked(open.Demands)
	stands, err := standsOf(ctx, plans.Plans, asked)
	if err != nil {
		return render.Def{}, err
	}
	releases := deckReleases(ctx, module, focus)

	lang := in.Language
	title := deckWord(lang, "title_all")
	switch {
	case focus != nil:
		title = deckWordf(lang, "title_of", focus.Name)
	case module != "":
		title = deckWordf(lang, "title_of", module)
	}
	byID := map[string]api.Demand{}
	var walkDemands func(ds []api.Demand)
	walkDemands = func(ds []api.Demand) {
		for _, d := range ds {
			byID[d.ID] = d
			walkDemands(d.Parts)
		}
	}
	walkDemands(open.Demands)
	// Needs you is To do's waiting work, read from the same source - the
	// person queue - its held items left out: read once, the
	// computation the rail's count shares, so the Needs you tile, the Needs
	// tab's badge and « À faire » show one number.
	base, queueErr := sharedQueueBase(ctx, in.Language, true)
	var waiting map[string]bool
	if queueErr == nil {
		waiting = personWaiting(base)
	}
	needs, needing := needsTab(stands, byID, waiting)
	period := in.Period
	if period == "" {
		period = api.PeriodWeek
	}
	if periodStart(period, time.Now()).IsZero() {
		return render.Def{}, invalid("Deck: period %q is day, week or month", in.Period)
	}
	doneWeek := doneIn(plans.Plans, period, time.Now())
	counts := map[string]int{}
	for _, p := range []string{api.PeriodDay, api.PeriodWeek, api.PeriodMonth} {
		counts[p] = len(doneIn(plans.Plans, p, time.Now()))
	}
	var inScope map[string]bool // a release deck: its plans, split parts included
	if scope != nil {
		inScope = map[string]bool{}
		for _, p := range plans.Plans {
			inScope[p.ID] = true
		}
	}
	current := currentReleases(ctx, lang, focus) // the current releases' increments
	byModule := modulesOf(ctx, stands, inScope)
	you := needsYouIn(base, queueErr, lang, deckFocus(module, focus), needing)
	page := node{"kind": "tabs", "variant": "pills", "tabs": []node{
		{"id": tabOverview, "label": deckWord(lang, "tab_overview"), "icon": "layout-dashboard", "content": overviewTab(lang, stands, doneWeek, period, counts, you, current)},
		{"id": tabByState, "label": deckWord(lang, "tab_by_state"), "icon": "list-check", "badge": len(stands), "content": byStateTab(stands, doneWeek, period)},
		{"id": tabByPhase, "label": deckWord(lang, "tab_by_phase"), "icon": "arrows-right", "content": byPhaseTab(stands)},
		{"id": tabByModule, "label": deckWord(lang, "tab_by_module"), "icon": "folders", "badge": len(byModule), "content": byModuleTab(byModule)},
		{"id": "requests", "label": deckWord(lang, "tab_requests"), "icon": "sitemap", "badge": len(open.Demands), "content": requestsTab(open.Demands, stands)},
		{"id": tabNeeds, "label": deckWord(lang, "tab_needs"), "icon": "hand-stop", "badge": you.count, "content": needs},
		{"id": "deliveries", "label": deckWord(lang, "tab_deliveries"), "icon": "rocket", "badge": countLive(releases)},
		{"id": "history", "label": deckWord(lang, "tab_history"), "icon": "archive"},
		{"id": "groups", "label": deckWord(lang, "tab_groups"), "icon": "gauge"},
	}}
	tabs := page["tabs"].([]node)
	for _, t := range tabs[len(tabs)-3:] {
		t["content"] = deckTabSection(in.Target, in.ID, t["id"].(string))
	}
	return render.Def{Kind: "page", Title: title, DSLTree: page}, nil
}

// deckTabURI loads one tab of the deck.
const deckTabURI = "ktw-agents:views/deck-tab"

// deckTabSection is a tab's content loaded when the tab opens: a section the
// host loads from DeckTab, rows drawn meanwhile.
func deckTabSection(target, id, tab string) node {
	args := node{"tab": tab}
	if target != "" {
		args["target"] = target
	}
	if id != "" {
		args["id"] = id
	}
	return loadedSection("deck:"+tab, deckTabURI, args, "", render.PlaceholderRows, 4)
}

// DeckTab answers one tab of the deck, loaded when it opens
// : its history, its deliveries or its task groups, of
// what the deck is about - as Deck draws them.
func (v *Views) DeckTab(ctx context.Context, in api.DeckTabInput) (render.Def, error) {
	ctx = withReadCache(ctx)
	module, focus, scope := deckScope(ctx, in.Target, in.ID)
	var content node
	switch in.Tab {
	case api.DeckTabHistory:
		history, err := New().Demands(ctx, api.DemandsInput{Module: module, View: api.DemandsHistory})
		if err != nil {
			return render.Def{}, err
		}
		if scope != nil {
			history.Demands = inScopeDemands(history.Demands, scope)
		}
		content = historyTab(history.Demands)
	case api.DeckTabDeliveries:
		open, err := New().Demands(ctx, api.DemandsInput{Module: module})
		if err != nil {
			return render.Def{}, err
		}
		if scope != nil {
			open.Demands = inScopeDemands(open.Demands, scope)
		}
		content = deliveriesTab(ctx, open.Demands, deckReleases(ctx, module, focus))
	case api.DeckTabGroups:
		content = groupsTab(taskGroups(ctx))
	default:
		return render.Def{}, invalid("DeckTab: tab %q is history, deliveries or groups", in.Tab)
	}
	return render.Def{Kind: "page", DSLTree: content}, nil
}

// deckScope is what a deck is about: the module of its target (else id;
// "" every module) - a release target's module, its deck kept to that
// release (focus) and to its targets and their parts (scope).
func deckScope(ctx context.Context, target, id string) (module string, focus *api.Release, scope map[string]bool) {
	module = target
	if module == "" {
		module = id
	}
	if module == "" {
		return "", nil, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return module, nil, nil
	}
	o, err := loadObject(ctx, cwm, module)
	if err != nil || o.Type != api.ReleaseType {
		return module, nil, nil
	}
	r := toRelease(o)
	focus, scope = &r, map[string]bool{}
	if sc, err := NewReleases().Scope(ctx, api.GetInput{ID: r.ID}); err == nil {
		for _, t := range sc.Targets {
			scope[t.ID] = true
		}
	}
	module = r.Module
	if r.Project {
		module = ""
	}
	return module, focus, scope
}

// deckReleases are the releases a deck shows: its module's, or its focus.
func deckReleases(ctx context.Context, module string, focus *api.Release) []api.Release {
	if focus != nil {
		return []api.Release{*focus}
	}
	releases, _ := NewReleases().List(ctx, api.ListInput{Module: module})
	return releases.Releases
}

// taskGroups are the task groups' loads; none without a task service.
func taskGroups(ctx context.Context) []taskapi.GroupLoad {
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return nil
	}
	g, err := tasks.Groups(ctx, taskapi.GroupsRequest{})
	if err != nil {
		return nil
	}
	return g.Groups
}

// ---------------------------------------------------------------- overview

// The tabs a click on the Overview opens.
const (
	tabOverview = "overview"
	tabByState  = "by-state"
	tabByPhase  = "by-phase"
	tabNeeds    = "needs"
	tabByModule = "by-module"
)

// fillHeight: a board takes the window's height below it, each lane scrolling
// inside itself.
const fillHeight = "fill"

// The Overview's Needs you tile and its button (its action opens the rail's
// To do, « À faire »): their element ids.
const (
	needsTileView  = "deck-needs-tile"
	needsOpenView  = "deck-needs-open"
	needsTileWidth = 320 // the tile's column in the By phase header
)

// doneLane is the state lanes' last lane: the requests ended done in the
// period asked.
const doneLane = "done"

// The Done tile's words, by period: its switch's label, its hint and its lane.
var periodWords = map[string][3]string{
	api.PeriodDay:   {"Day", "ended done today", "Done today"},
	api.PeriodWeek:  {"Week", "ended done this week", "Done this week"},
	api.PeriodMonth: {"Month", "ended done this month", "Done this month"},
}

// overviewTab is where the work stands at a glance: the six state tiles (a
// click opens By state), the slim By phase header - the open requests by
// phase as one bar (a click opens By phase) with the Needs you counter on its
// right (its button opens « À faire ») -, then what runs now and the current
// releases' increments side by side.
func overviewTab(lang string, stands []stand, doneWeek []node, period string, counts map[string]int, needs needsYou, current node) node {
	count := map[string]int{}
	for _, st := range stands {
		count[st.state]++
	}
	byState := render.ShowTab(deckWord(lang, "tab_by_state"), tabByState)
	tile := func(label string, value int, hint, color, icon string) node {
		t := kpi(label, value, hint, color, icon)
		t["action"] = byState
		return t
	}
	tiles := node{"kind": "grid", "cols": 6, "gap": "md", "children": []node{
		tile(deckWord(lang, "tile_open"), len(stands), deckWord(lang, "tile_open_hint"), "indigo", "stack-2"),
		tile(deckWord(lang, "tile_ready"), count[stateLabels[api.PlanReady]], deckWord(lang, "tile_ready_hint"), stateColors[api.PlanReady], "player-play"),
		tile(deckWord(lang, "tile_running"), count[stateLabels[standRunning]], deckWord(lang, "tile_running_hint"), stateColors[standRunning], "loader-2"),
		tile(deckWord(lang, "tile_waiting"), count[stateLabels[api.PlanIdle]], deckWord(lang, "tile_waiting_hint"), stateColors[api.PlanIdle], "hourglass"),
		tile(deckWord(lang, "tile_blocked"), count[stateLabels[api.PlanBlocked]], deckWord(lang, "tile_blocked_hint"), stateColors[api.PlanBlocked], "alert-octagon"),
		doneTile(lang, counts, stateColors[api.PlanDone], byState),
	}}

	inPhase := map[string]int{}
	for _, st := range stands {
		inPhase[st.lane()]++
	}
	var segments []node
	for _, ph := range api.Phases {
		segments = append(segments, node{"label": deckPhase(lang, ph), "value": inPhase[ph], "color": phaseColors[ph]})
	}
	gauge := node{"kind": "gauge", "segments": segments, "legend": true, "size": "sm", "valueText": deckWord(lang, "phase_open"), "action": render.ShowTab(deckWord(lang, "tab_by_phase"), tabByPhase)}

	return node{"kind": "vbox", "gap": "lg", "children": []node{
		tiles,
		phaseHeader(lang, gauge, needsTile(lang, needs)),
		{"kind": "grid", "cols": 2, "gap": "lg", "data-view": overviewBoxesView, "children": []node{fillBox(runningNow(lang, stands)), fillBox(current)}},
	}}
}

// fillBox makes an Overview box take the window's height below it, its
// body - the card's last child - scrolling inside it, so Running now and the
// current releases each scroll their own list and the page does not (
// render.Node.FillHeight's mark, set here on the deck's plain map).
func fillBox(box node) node {
	box["height"] = fillHeight
	return box
}

// The Overview's slim By phase header and the row of its two boxes, Running
// now and the current releases: their element ids.
const (
	phaseHeaderView   = "deck-phase-header"
	overviewBoxesView = "deck-overview-boxes"
	phaseTitleWidth   = 120 // the header's title column
)

// phaseHeader is By phase as one slim row: its title
// the phase bar with its chips - a click opens By phase - and on its right the
// Needs you counter, whose button opens « À faire »; no tall box, so Running
// now and the current releases take the page below.
func phaseHeader(lang string, gauge, counter node) node {
	title := node{"kind": "heading", "text": deckWord(lang, "tab_by_phase"), "level": 6, "icon": "arrows-right", "color": "blue"}
	return node{"kind": "card", "withBorder": true, "p": "sm", "radius": "md", "data-view": phaseHeaderView, "children": []node{
		{"kind": "grid", "cols": 3, "gap": "md", "widths": []int{phaseTitleWidth, 0, needsTileWidth}, "children": []node{
			{"kind": "vbox", "justify": "center", "children": []node{title}},
			{"kind": "vbox", "justify": "center", "children": []node{gauge}},
			counter,
		}},
	}}
}

// needsYou is what the Needs you tile counts: the person queue's waiting
// items in the deck's scope - « À faire »'s count, what the rail's To do
// shows - and how many ask each action.
type needsYou struct {
	count    int
	byAction map[string]int
}

// deckFocus is the scope the deck's person queue is read in: its release,
// else its module, else everything.
func deckFocus(module string, focus *api.Release) string {
	if focus != nil {
		return focus.ID
	}
	return module
}

// needsYouIn is the person queue in scope - its count is the Needs you
// tile's and the Needs tab's badge; unread (err), the Needs a person cards
// are counted, by no action.
func needsYouIn(base queueBase, err error, lang, scope string, needing []node) needsYou {
	if err != nil {
		return needsYou{count: len(needing)}
	}
	q := scopedQueue(base, api.PersonQueueInput{Target: scope, Language: lang})
	by := map[string]int{}
	for a, n := range q.ByAction {
		if a != api.ActionNotice && a != api.ActionConversation && n > 0 {
			by[a] = n
		}
	}
	return needsYou{count: q.Count, byAction: by}
}

// needsActions are the actions the tile's line names, in « À faire »'s order.
var needsActions = []string{api.ActionUnblock, api.ActionDecide, api.ActionValidate, api.ActionAnswer, api.ActionTest}

// needsTile is the Needs you tile: how many items wait on the person, one
// line of what each asks, and « Ouvrir À faire » opening the rail's To do -
// a call to act, so the kpi's inline variant (bold label and count on one
// line, no icon, a soft ring) and an outlined button, as
// request-rows-mockup.html draws it.
func needsTile(lang string, n needsYou) node {
	var parts []string
	for _, a := range needsActions {
		if c := n.byAction[a]; c > 0 {
			parts = append(parts, deckWordf(lang, "needs_"+a, c))
		}
	}
	hint := strings.Join(parts, " · ")
	color := "grape"
	if n.count == 0 {
		color, hint = "gray", deckWord(lang, "needs_none")
	}
	label := deckWord(lang, "needs_open")
	t := node{"kind": "kpi", "variant": "inline", "label": deckWord(lang, "needs_tile"), "value": n.count, "description": hint, "color": color, "data-view": needsTileView}
	t["children"] = []node{
		render.Button(label, render.OpenScreen(label, render.ScreenTodo)).With("data-view", needsOpenView).With("color", "grape").With("variant", "outline").With("icon", "arrow-right"),
	}
	return t
}

// doneTile is the Done tile: the requests ended done today, this week and this
// month at once, "Day/Week/Month" over "d/w/m".
func doneTile(lang string, counts map[string]int, color string, action render.Action) node {
	t := node{"kind": "kpi", "label": deckWord(lang, "tile_done"),
		"value":       fmt.Sprintf("%d/%d/%d", counts[api.PeriodDay], counts[api.PeriodWeek], counts[api.PeriodMonth]),
		"description": deckWord(lang, "tile_done_hint"), "color": color, "icon": "circle-check"}
	t["action"] = action
	return t
}

// periodStart is when a period began: today at midnight, the week's Monday,
// the month's first day (UTC); zero for an unknown period.
func periodStart(period string, now time.Time) time.Time {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch period {
	case api.PeriodDay:
		return day
	case api.PeriodWeek:
		return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	case api.PeriodMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}
}

// doneIn is the requests ended done in a period, as cards.
func doneIn(plans []api.PlannedObject, period string, now time.Time) []node {
	out := []node{}
	since := periodStart(period, now).Format(time.RFC3339)
	for _, p := range plans {
		if p.State == api.PlanDone && len(p.Trail) > 0 && p.Trail[len(p.Trail)-1].At >= since {
			ended := p.Trail[len(p.Trail)-1].At
			if len(ended) >= 10 {
				ended = ended[:10]
			}
			out = append(out, node{"id": p.ID, "title": p.Title, "column": doneLane, "state": stateLabels[api.PlanDone], "meta": p.Module + " · " + ended})
		}
	}
	return out
}

// currentReleases is the Overview's right-hand box:
// the increments of each stream's current release - Team priorities' data,
// read through TeamPriorities (CurrentOnly: no other group is read) - drawn
// as Team priorities draws a group, in
// the compact « overview » row: the priority, the title with « id · release »
// under it, the progress; by priority then rank. With several streams, a
// separator names each stream's current release; a release deck shows only
// its release's group. A click on a row opens the increment.
func currentReleases(ctx context.Context, lang string, focus *api.Release) node {
	var groups []api.TeamGroup
	if d, err := NewIncrements().TeamPriorities(ctx, api.TeamPrioritiesInput{HideDelivered: true, CurrentOnly: focus == nil}); err == nil {
		for _, g := range d.Groups {
			if len(g.Releases) != 1 {
				continue
			}
			if focus != nil {
				if g.Releases[0].ID == focus.ID {
					groups = append(groups, g)
				}
				continue
			}
			if g.Kind == api.TeamGroupCurrent {
				groups = append(groups, g)
			}
		}
	}
	rows := []node{}
	if len(groups) == 0 {
		rows = append(rows, render.Dimmed(deckWord(lang, "current_none")))
	} else if cwm, err := cwmOf(ctx); err == nil {
		names := map[string]string{}
		if len(groups) > 1 {
			if streams, err := orderedStreams(ctx, cwm); err == nil {
				for _, so := range streams {
					st := toStream(so)
					names[st.ID] = st.Name
				}
			}
		}
		for _, g := range groups {
			if len(groups) > 1 {
				rows = append(rows, teamSeparator(lang, g, names[g.Stream]))
			}
			rows = append(rows, teamGroupTable(ctx, cwm, lang, g, overviewRowVariant))
		}
	}
	return section(deckWord(lang, "current_releases"), "rocket", "teal", deckWord(lang, "current_releases_hint"),
		node{"kind": "vbox", "gap": "xs", "data-view": currentReleasesView, "children": rows})
}

// The Overview's right-hand box: its element id and its rows' variant of the
// increment-row data type.
const (
	currentReleasesView = "deck-current-releases"
	overviewRowVariant  = "overview"
)

// stepOfTodo is a to-do's step in words: the step and who does it.
func stepOfTodo(t api.WorkTodo) string {
	who := t.Packet.Task
	if t.Packet.Doer == api.DoerPerson {
		who = "a person to " + t.Packet.Interaction
	}
	if who == "" {
		return t.Packet.Step
	}
	return t.Packet.Step + " (" + who + ")"
}

func orEmpty(n []node) []node {
	if n == nil {
		return []node{}
	}
	return n
}

// ---------------------------------------------------------------- by module

// modulesOf is one card per user module with open work: how many of its
// requests are ready, running, waiting and blocked, what is held and by whom,
// and its next takeable step — what needs attention first, first.
func modulesOf(ctx context.Context, stands []stand, scope map[string]bool) []node {
	type tally struct {
		counts map[string]int
		held   []string
		next   string
	}
	per := map[string]*tally{}
	of := func(m string) *tally {
		if per[m] == nil {
			per[m] = &tally{counts: map[string]int{}}
		}
		return per[m]
	}
	for _, st := range stands {
		if st.ended {
			continue
		}
		m := st.plan.Module
		if m == "" {
			m, _ = cwmapi.ModuleOfObjectID(st.plan.ID)
		}
		t := of(m)
		t.counts[st.state]++
		if st.step != nil && st.step.Claim != nil {
			t.held = append(t.held, st.step.Claim.By.ID+" ("+st.step.Packet.Step+" on "+st.plan.ID+")")
		}
	}
	if q, err := queueOf(ctx, ""); err == nil {
		for _, e := range q.Queue {
			t := decode(e.Todo)
			if !e.Takeable || per[t.Module] == nil || per[t.Module].next != "" || (scope != nil && !scope[t.Request]) {
				continue
			}
			per[t.Module].next = stepOfTodo(t) + " — " + t.Request
		}
	}
	user := map[string]bool{}
	if cwm, err := cwmOf(ctx); err == nil {
		if mods, err := cwmapi.UserModules(ctx, cwm); err == nil {
			for _, m := range mods {
				user[m.ID] = true
			}
		}
	}
	var ids []string
	for m := range per {
		if len(user) == 0 || user[m] {
			ids = append(ids, m)
		}
	}
	rank := func(m string) []int {
		c := per[m].counts
		return []int{c[stateLabels[api.PlanBlocked]], c[stateLabels[api.PlanIdle]], c[stateLabels[api.PlanReady]], c[stateLabels[standRunning]]}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := rank(ids[i]), rank(ids[j])
		for k := range a {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return ids[i] < ids[j]
	})
	out := []node{}
	for _, m := range ids {
		t := per[m]
		c := t.counts
		meta := fmt.Sprintf("%d blocked · %d waiting · %d ready · %d running", c[stateLabels[api.PlanBlocked]], c[stateLabels[api.PlanIdle]], c[stateLabels[api.PlanReady]], c[stateLabels[standRunning]])
		if len(t.held) > 0 {
			meta += " · held: " + strings.Join(t.held, ", ")
		}
		if t.next != "" {
			meta += " · next: " + t.next
		}
		state := stateLabels[api.PlanReady]
		switch {
		case c[stateLabels[api.PlanBlocked]] > 0:
			state = stateLabels[api.PlanBlocked]
		case c[stateLabels[api.PlanIdle]] > 0:
			state = stateLabels[api.PlanIdle]
		case c[stateLabels[standRunning]] > 0 && c[stateLabels[api.PlanReady]] == 0:
			state = stateLabels[standRunning]
		}
		out = append(out, node{"id": m, "title": m, "state": state, "meta": meta})
	}
	return out
}

// byModuleTab lists the modules with open work; a click opens the module on
// its own planning deck.
func byModuleTab(modules []node) node {
	return section("By module", "folders", "indigo", "Each module with open work: where its requests stand, what is held and its next step — what needs attention first, first",
		node{"kind": "list", "data": orEmpty(modules), "empty": "No module has open work.", "item": itemCard(), "toneField": "state", "toneColors": labelColors(),
			"open": node{"label": "Focus on the module", "navigate": node{"object": "${item.id}", "panel": "planning-deck"}}})
}

// ---------------------------------------------------------------- by state

// byStateTab is the board of where each open request stands, read from its
// to-dos, and what ended done this week.
func byStateTab(stands []stand, doneWeek []node, period string) node {
	lanes := []node{}
	for _, st := range []string{api.PlanReady, standRunning, standLapsed, api.PlanIdle, api.PlanBlocked} {
		lanes = append(lanes, node{"id": stateLabels[st], "title": phaseTitle(stateLabels[st]), "color": stateColors[st]})
	}
	lanes = append(lanes, node{"id": doneLane, "title": periodWords[period][2], "color": stateColors[api.PlanDone]})
	cards := []node{}
	for _, st := range stands {
		cards = append(cards, node{"id": st.plan.ID, "title": st.plan.Title, "column": st.state, "state": st.state, "meta": st.meta()})
	}
	cards = append(cards, doneWeek...)
	return section("The work by state", "list-check", "teal", "Each open request where it stands, read from its to-dos, and what ended done this week",
		node{"kind": "lanes", "lanes": lanes, "data": cards, "laneField": "column", "laneWidth": 220, "laneHeight": fillHeight, "item": itemCard(), "toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard()})
}

// ---------------------------------------------------------------- by phase

// byPhaseTab is each open request in the phase of the step it is on — intake's
// waiting and running requests in the Requests lane.
func byPhaseTab(stands []stand) node {
	lanes := make([]node, 0, len(api.Phases))
	for _, ph := range api.Phases {
		lanes = append(lanes, node{"id": ph, "title": phaseTitle(ph), "color": phaseColors[ph]})
	}
	cards := []node{}
	for _, st := range stands {
		cards = append(cards, node{"id": st.plan.ID, "title": st.plan.Title, "lane": st.lane(), "state": st.state, "meta": st.meta()})
	}
	return section("The work by phase", "arrows-right", "blue", "Each open request in the phase of the step it is on; intake's in Requests",
		node{"kind": "lanes", "lanes": lanes, "data": cards, "laneWidth": 220, "laneHeight": fillHeight, "item": itemCard(), "toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard()})
}

// runningNow lists the steps pulled now: who holds each, since when. A step
// whose claim lapsed reads lapsed, with its holder and when it lapsed, and
// offers to release it.
func runningNow(lang string, stands []stand) node {
	running := []node{}
	for _, st := range stands {
		switch st.state {
		case stateLabels[standRunning]:
			running = append(running, node{"id": st.plan.ID, "title": st.plan.Title, "state": st.state, "meta": st.heldText()})
		case stateLabels[standLapsed]:
			running = append(running, withExpected(node{"id": st.plan.ID, "title": st.plan.Title, "state": st.state}, expected(st, nil, nil)))
		}
	}
	return section(deckWord(lang, "running_now"), "loader-2", "blue", deckWord(lang, "running_now_hint"),
		node{"kind": "list", "data": running, "empty": deckWord(lang, "running_none"), "item": itemCard(), "toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard(), "actions": expectedActions(true)})
}

// ---------------------------------------------------------------- needs a person

// needsTab is what needs a person — one board of three lanes: failed steps to
// try again, ends refused, what waits for an answer — and its cards.
func needsTab(stands []stand, demands map[string]api.Demand, waiting map[string]bool) (node, []node) {
	const failedLane, refusedLane, waitingLane = "failed", "refused", "waiting"
	cards := []node{}
	for _, st := range stands {
		// Only what To do lists, never what is on hold there (waiting nil:
		// the person queue could not be read - every case shows).
		if waiting != nil && !waiting[st.plan.ID] {
			continue
		}
		d := demands[st.plan.ID]
		e := expected(st, d.Deliverables, d.AskedIn)
		c := withExpected(node{"id": st.plan.ID, "title": st.plan.Title, "state": st.state, "meta": st.meta()}, e)
		switch e.Case {
		case expectHeld:
			continue // stopped on what another moves: never the person's card
		case expectFailed:
			c["column"] = failedLane
		case expectRefused, expectBlocked:
			c["column"] = refusedLane
		case expectAnswer:
			c["column"] = waitingLane
		default:
			if waiting == nil {
				continue
			}
			c["column"] = waitingLane // a person's step To do lists (decide, test, validate)
		}
		cards = append(cards, c)
	}
	lanes := []node{
		{"id": failedLane, "title": "A step failed", "meta": "Try its step again, or drop the request", "color": "red"},
		{"id": refusedLane, "title": "Its end refused, or blocked", "meta": "A deliverable not verified, or another cause to settle", "color": "red"},
		{"id": waitingLane, "title": "Waits for an answer", "meta": "A person's step is up: answer it in the Inbox", "color": "orange"},
	}
	return section("Needs a person", "hand-stop", "orange", "What only a person can move on: each card says what is expected; a click opens the request",
		node{"kind": "lanes", "lanes": lanes, "data": cards, "laneField": "column", "laneWidth": 280, "laneHeight": fillHeight, "item": itemCard(),
			"toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard(), "actions": expectedActions(true)}), cards
}

// ---------------------------------------------------------------- task groups

func groupsTab(groups []taskapi.GroupLoad) node {
	var gauges []node
	for _, g := range groups {
		gauges = append(gauges, node{"kind": "gauge", "label": g.Name, "value": g.Running, "max": max(1, g.MaxRunning),
			"hint": fmt.Sprintf("{value} of {max} running · %d waiting · %d queued%s", g.Waiting, g.Queued, paused(g))})
		if g.Name == api.PlanGroup {
			label, uri, icon := "Pause the planning group", deckPauseURI, "player-pause"
			if !g.On {
				label, uri, icon = "Resume the planning group", deckResumeURI, "player-play"
			}
			gauges = append(gauges, node{"kind": "button", "label": label, "variant": "light", "size": "xs",
				"action": node{"uri": uri, "icon": icon, "label": label, "args": node{"by": "${person}"}}})
		}
	}
	if len(gauges) == 0 {
		gauges = append(gauges, node{"kind": "text", "content": "No task groups (the task service is not reachable).", "size": "sm", "color": "dimmed"})
	}
	return section("Task groups", "gauge", "violet", "How many runs of each group run at once — the internal scheduler runs each step in its group", node{"kind": "vbox", "gap": "md", "children": gauges})
}

// ---------------------------------------------------------------- requests

func requestsTab(open []api.Demand, stands []stand) node {
	at := map[string]stand{}
	for _, st := range stands {
		at[st.plan.ID] = st
	}
	var rows []node
	var walk func(ds []api.Demand, parent string)
	walk = func(ds []api.Demand, parent string) {
		for _, d := range ds {
			state, meta := stateLabels[d.State], demandMeta(d)
			if st, ok := at[d.ID]; ok {
				// Where it stands, from its to-dos: not the step written when it was filed.
				state, meta = st.state, st.meta()
			}
			rows = append(rows, node{"id": d.ID, "parent": parent, "title": d.Title, "state": state, "meta": meta})
			walk(d.Parts, d.ID)
		}
	}
	colors := map[string]string{}
	for st, label := range stateLabels {
		colors[label] = stateColors[st]
	}
	// What people asked apart from the modules' work.
	var asked, work []api.Demand
	for _, d := range open {
		if d.Kind == api.KindDemand {
			asked = append(asked, d)
		} else {
			work = append(work, d)
		}
	}
	tree := func(ds []api.Demand, empty string) node {
		rows = nil
		walk(ds, "")
		return node{"kind": "tree", "data": rows, "badgeField": "state", "badgeColors": colors, "item": itemLines(), "open": openRequestCard(),
			"toneField": "state", "toneColors": colors, "pulse": livePulse, "empty": empty}
	}
	return node{"kind": "vbox", "gap": "lg", "children": []node{
		section(fmt.Sprintf("Demands — %d", len(asked)), "message-question", "grape", "What people asked: each reconciles into the goals and features and increments it changes - where it stands and why it waits",
			tree(asked, "No open demand.")),
		section(fmt.Sprintf("Module work — %d", len(work)), "sitemap", "indigo", "The modules' work - release work and direct work - with its parts in any module: where it stands, the step it is on and why it waits",
			tree(work, "No open work.")),
	}}
}

// ---------------------------------------------------------------- deliveries

func deliveriesTab(ctx context.Context, open []api.Demand, releases []api.Release) node {
	var live []api.Release
	for _, r := range releases {
		if r.State == api.ReleasePlanned || r.State == api.ReleaseFrozen {
			live = append(live, r)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		if live[i].Date != live[j].Date {
			return live[i].Date < live[j].Date
		}
		return live[i].ID < live[j].ID
	})
	in := map[string]string{} // demand → the release it targets
	lanes := []node{{"id": deckBacklog, "title": "Backlog", "meta": "Targets no release", "color": "gray",
		"move": node{"uri": deckUntargetURI, "label": "Untarget", "args": node{"id": "${item.id}"}}}}
	rel := NewReleases()
	for _, r := range live {
		meta := r.State
		if r.Date != "" {
			meta += " · due " + r.Date
		}
		if scope, err := rel.Scope(ctx, api.GetInput{ID: r.ID}); err == nil {
			meta += fmt.Sprintf(" · %d / %d done", scope.Done, scope.Total)
			for _, t := range scope.Targets {
				in[t.ID] = r.ID
			}
		}
		color := "blue"
		if r.State == api.ReleaseFrozen {
			color = "violet"
		}
		lanes = append(lanes, node{"id": r.ID, "title": r.Name, "meta": meta, "color": color})
	}
	var cards []node
	for _, d := range open {
		lane := in[d.ID]
		if lane == "" {
			lane = deckBacklog
		}
		cards = append(cards, node{"id": d.ID, "title": d.Title, "lane": lane, "state": stateLabels[d.State], "meta": d.Module})
	}
	board := section("Releases and the backlog", "rocket", "violet", "Drag a demand onto a release to target it, back to the backlog to untarget it",
		node{"kind": "lanes", "lanes": lanes, "data": cards, "item": itemCard(), "laneWidth": 260, "laneHeight": fillHeight, "toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard(),
			"move": node{"uri": deckTargetURI, "label": "Target", "args": node{"id": "${item.id}", "release": "${lane}"}}})

	children := []node{board}
	if len(live) > 0 {
		r := live[0]
		if net, err := NewPlanning().Network(ctx, api.GetInput{ID: r.ID}); err == nil {
			children = append(children, networkViews(r, net))
		}
	}
	return node{"kind": "vbox", "gap": "lg", "children": children}
}

// networkViews draws a release's network as a Gantt and as a graph.
func networkViews(r api.Release, net api.NetworkOutput) node {
	var tasks, boxes, edges []node
	for _, n := range net.Nodes {
		if n.Ended {
			continue
		}
		tasks = append(tasks, node{"id": n.ID, "title": n.Title, "start": n.EarliestStart, "length": maxf(n.Weight, 0.1), "critical": n.Critical})
		boxes = append(boxes, node{"id": n.ID, "title": n.Title, "meta": fmt.Sprintf("slack %g", n.Slack), "critical": n.Critical})
		for _, a := range n.After {
			edges = append(edges, node{"from": a, "to": n.ID})
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i]["start"].(float64) < tasks[j]["start"].(float64) })
	if len(tasks) == 0 {
		return section(r.Name+" — its network", "chart-dots-3", "blue", "", node{"kind": "text", "content": "Nothing left to do in it.", "size": "sm", "color": "dimmed"})
	}
	return node{"kind": "grid", "cols": 2, "gap": "lg", "children": []node{
		section(r.Name+" — Gantt", "timeline", "blue", fmt.Sprintf("Earliest start and weight; the critical path in dark (%g units)", net.Duration),
			node{"kind": "gantt", "data": tasks, "unit": "h"}),
		section(r.Name+" — network", "chart-dots-3", "grape", "Each box after what it waits on; slack under each",
			node{"kind": "graph", "data": node{"nodes": boxes, "edges": edges}}),
	}}
}

// ---------------------------------------------------------------- history

func historyTab(history []api.Demand) node {
	var rows []node
	for _, d := range history {
		ended := d.EndedAt
		if len(ended) >= 10 {
			ended = ended[:10]
		}
		meta := d.Module + " · " + ended
		if d.Why != "" {
			meta += " — " + d.Why
		}
		rows = append(rows, node{"id": d.ID, "title": d.Title, "state": stateLabels[d.State], "meta": meta})
	}
	return section("Done and dropped", "archive", "green", "The latest first, with how each ended",
		node{"kind": "list", "data": rows, "empty": "Nothing has ended yet.", "item": itemCard(), "toneField": "state", "toneColors": labelColors(), "pulse": livePulse, "open": openRequestCard()})
}

// ---------------------------------------------------------------- pieces

func kpi(label string, value int, hint, color, icon string) node {
	return node{"kind": "kpi", "label": label, "value": value, "description": hint, "color": color, "icon": icon}
}

func section(title, icon, color, description string, body node) node {
	n := node{"kind": "card", "withBorder": true, "p": "lg", "radius": "md", "gap": "sm", "children": []node{
		{"kind": "heading", "text": title, "level": 5, "icon": icon, "color": color},
	}}
	kids := n["children"].([]node)
	if description != "" {
		kids = append(kids, node{"kind": "text", "content": description, "size": "xs", "color": "dimmed"})
	}
	n["children"] = append(kids, body)
	return n
}

// itemLines is the template of a tree row (its badge is the tree's): the
// title, its id and a dimmed line.
func itemLines() node {
	return node{"kind": "vbox", "gap": 2, "children": []node{
		{"kind": "text", "content": "", "dataField": "$.item.title", "size": "sm", "fw": 500},
		{"kind": "hbox", "gap": 6, "wrap": true, "children": []node{
			idBadge(),
			{"kind": "text", "content": "", "dataField": "$.item.meta", "size": "xs", "color": "dimmed"},
		}},
	}}
}

// idBadge is a card's request id (<module>.RQ-<n>), as written.
func idBadge() node {
	return node{"kind": "badge", "text": "", "dataField": "$.item.id", "size": "xs", "variant": "outline", "color": "gray", "mono": true}
}

// livePulse: what pulses on the deck — the work running now.
var livePulse = []string{stateLabels[standRunning]}

// phaseColors give each phase its lane color, in the order work flows.
var phaseColors = map[string]string{
	api.PhaseRequests: "indigo", api.PhaseInputs: "cyan", api.PhasePreparation: "grape",
	api.PhaseBuild: "blue", api.PhaseVerify: "orange", api.PhaseRelease: "green",
}

// labelColors are the state colors keyed by the label a person reads.
func labelColors() map[string]string {
	out := map[string]string{}
	for st, label := range stateLabels {
		out[label] = stateColors[st]
	}
	return out
}

func countLive(rs []api.Release) int {
	n := 0
	for _, r := range rs {
		if r.State == api.ReleasePlanned || r.State == api.ReleaseFrozen {
			n++
		}
	}
	return n
}

func dropAction() node {
	return node{"id": "drop", "label": "Drop", "icon": "trash", "uri": deckFinishURI, "dangerous": true,
		"confirm": "Drop this demand? Its plan ends, dropped.",
		"args":    node{"id": "${item.id}", "outcome": api.PlanDropped, "by": "${person}", "note": "dropped from the deck"}}
}

func demandMeta(d api.Demand) string {
	parts := []string{d.Module}
	if d.Why != "" {
		parts = append(parts, d.Why)
	}
	return strings.Join(parts, " · ")
}

// stepLabel is a step in words: who does it and the recipe step it is.
func stepLabel(s api.PlanStep) string {
	who := s.Task
	if s.Doer == api.DoerPerson {
		who = "a person to " + s.Interaction
	}
	if s.Step != "" {
		return who + " (" + s.Step + ")"
	}
	return who
}

func phaseTitle(ph string) string {
	if ph == "" {
		return ph
	}
	return strings.ToUpper(ph[:1]) + ph[1:]
}

func paused(g taskapi.GroupLoad) string {
	if !g.On {
		return " · paused"
	}
	return ""
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// planIdle reports a demand whose plan is idle — its plan's state, not the one
// read from its to-dos: a question asked holds the plan itself.
func planIdle(d api.Demand) bool {
	return d.Plan != nil && (d.Plan.State == api.PlanIdle || d.Plan.State == "")
}

// personWaiting is the requests the person queue lists, held ones left out:
// what To do shows outside On hold. nil when the queue cannot be read.
func personWaiting(base queueBase) map[string]bool {
	out := map[string]bool{}
	for _, it := range base.items {
		if it.Held != nil || it.Action == api.ActionNotice {
			continue
		}
		for _, id := range []string{it.Request, it.Object} {
			if id != "" {
				out[id] = true
			}
		}
	}
	return out
}
