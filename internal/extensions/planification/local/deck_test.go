// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	plancore "github.com/katyoti/katyoti-fwk/internal/extensions/planification/internal/core"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// THE PLANNING DECK is one page of generic nodes over the requests, their
// to-dos, the groups and the releases, in eight tabs — Overview, By state, By
// phase, Requests, Needs a person, Deliveries, History, Task groups, no Intake:
// the Overview holds the six state tiles (a click opens By state), the phase
// bar (a click opens By phase), Running now, Needs you (only when something
// does) and the current releases' increments, and neither board; By state each open request in the lane
// of where it stands, read from its to-dos; By phase each in the phase of the
// step it is on, intake's in Requests, the lanes bounded so what follows stays
// on screen; every card with its request id; a failed step in the Needs a
// person tab with its retry; the groups' gauges and the planning group's pause
// in the Task groups tab; the backlog beside the releases. Nothing of the
// retired planner. It is declared as a global panel, on every module too.
func TestTheDeckIsOnePageOfGenericNodes(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	w.tasks.groups = []taskapi.GroupLoad{{Name: api.PlanGroup, Running: 1, MaxRunning: 1, Queued: 2, On: true}}
	// One past its intake (its analyse ready, in preparation); one whose intake
	// is pulled (running, in requests); one whose intake failed (blocked).
	analysed := file(t, w.ctx, w.req, api.FileInput{Title: "Build invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, analysed.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	pulled := file(t, w.ctx, w.req, api.FileInput{Title: "Mail them", Module: "billing", Kind: api.KindBuild})
	if got, err := schedOf(t, w.ctx).Pull(w.ctx, schedapi.PullInput{ID: stepOf(t, w.ctx, pulled.ID, api.StepIntake).ID, By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull the intake: %+v, %v", got, err)
	}
	stuck := file(t, w.ctx, w.req, api.FileInput{Title: "Ship them", Module: "shipping", Kind: api.KindBuild})
	failed := stepOf(t, w.ctx, stuck.ID, api.StepIntake)
	failPastTheCap(t, w.ctx, failed.ID, "no SMTP") // past the cap: the person\'s
	rel := plan(t, w.ctx, releases(t, w.ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})

	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(w.ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def)
	page := string(raw)
	if def.Kind != "page" {
		t.Fatalf("deck = kind %q, want a page", def.Kind)
	}
	var labels []string
	tab := map[string]string{}
	for _, p := range def.DSLTree.(map[string]any)["tabs"].([]map[string]any) {
		labels = append(labels, p["label"].(string))
		raw, _ := json.Marshal(p["content"])
		tab[p["id"].(string)] = string(raw)
	}
	// Deliveries, History and Task groups load when their tab opens
	// : read what each loads.
	for _, id := range []string{api.DeckTabDeliveries, api.DeckTabHistory, api.DeckTabGroups} {
		got, err := views.DeckTab(w.ctx, api.DeckTabInput{Tab: id})
		if err != nil {
			t.Fatalf("DeckTab %s: %v", id, err)
		}
		raw, _ := json.Marshal(got.DSLTree)
		tab[id] = string(raw)
	}
	if got := strings.Join(labels, ", "); got != "Overview, By state, By phase, By module, Requests, Needs a person, Deliveries, History, Task groups" {
		t.Errorf("the deck's tabs: %s", got)
	}
	has := func(where, in string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(in, want) {
				t.Errorf("%s lacks %s", where, want)
			}
		}
	}
	has("the Overview", tab["overview"],
		`"label":"Open","value":3`, `"label":"Ready","value":1`, `"label":"Running","value":1`, `"label":"Blocked","value":1`, `"label":"Day/Week/Month"`, // the tiles, from the to-dos
		`"action":{"label":"By state","tab":"by-state"}`, // a tile opens By state
		`"kind":"gauge"`, `"label":"preparation","value":1`, `"label":"requests","value":2`, `"action":{"label":"By phase","tab":"by-phase"}`, // the phase bar opens By phase
		"Running now", `"id":"`+pulled.ID+`"`, "held by "+another.ID, // what runs, who holds it
		`"data-view":"deck-needs-tile"`, `"label":"Needs you","value":1`, `"screen":"todo"`, // what needs a person: a tile whose button opens « À faire »
		"Current releases", `"data-view":"deck-current-releases"`) // the current releases' increments (none here)
	if strings.Contains(tab["overview"], "Open Needs a person") {
		t.Error("the Overview still has a button to Needs a person: a click on a card opens it")
	}
	// The boards take the window's height, each lane scrolling inside itself.
	for _, id := range []string{"by-state", "by-phase", "needs", "deliveries"} {
		has(id, tab[id], `"laneHeight":"fill"`)
	}
	if strings.Contains(tab["overview"], `"kind":"lanes"`) {
		t.Error("the Overview still holds a board: By state and By phase hold them")
	}
	has("By state", tab["by-state"], `"laneField":"column"`, `"id":"done"`,
		`"column":"ready","id":"`+analysed.ID+`"`, `"column":"running","id":"`+pulled.ID+`"`, `"column":"blocked","id":"`+stuck.ID+`"`) // each in its state's lane
	has("By phase", tab["by-phase"], `"laneHeight":`, // the lanes scroll inside themselves
		`"id":"`+analysed.ID+`","lane":"preparation"`, `"id":"`+pulled.ID+`","lane":"requests"`) // each in its current step's phase, intake's in Requests
	has("Needs a person", tab["needs"], `"kind":"lanes"`, `"id":"failed"`, `"id":"refused"`, `"id":"waiting"`, // one board of three lanes
		`"column":"failed"`, `"todo":"`+failed.ID+`"`, `"uri":"ktw-agents:work/retry"`, `"when":"todo"`, // the retry only on a failed step
		`"uri":"ktw-agents:plans/finish"`, `"by":"${person}"`)
	has("Task groups", tab["groups"], `"label":"Pause the planning group"`, `"uri":"ktw-agents:plans/pause"`, `"kind":"gauge"`)
	has("Deliveries", tab["deliveries"], `"id":"backlog"`, `"uri":"ktw-agents:releases/untarget"`, `"uri":"ktw-agents:releases/target"`, rel.ID)
	has("Requests", tab["requests"], `"kind":"tree"`)
	// Every card and tree row shows its request id.
	for id, content := range tab {
		if (strings.Contains(content, `"item":`) || strings.Contains(content, `"kind":"tree"`)) && !strings.Contains(content, `"dataField":"$.item.id"`) {
			t.Errorf("the %s tab's cards do not show their id", id)
		}
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Errorf("the deck is off the page grammar: %v", err)
	}
	if strings.Contains(page, `"kind":"timeline"`) || strings.Contains(page, "The last steps") {
		t.Error("the deck still shows the last steps: History holds what ended")
	}
	if strings.Contains(strings.ToLower(page), "planner") {
		t.Error("the deck still speaks of the retired planner")
	}

	pane, err := application.UseService[objectpaneapi.ObjectPaneService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	global, err := pane.GlobalPanels(w.ctx, objectpaneapi.GlobalPanelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range global.Panels {
		if p.ID == "home:planning-deck" && p.Source == "ktw-agents:views/deck" {
			found = true
		}
	}
	if !found {
		t.Errorf("global panels = %+v; want the planning deck, its source views/deck", global.Panels)
	}
}

// The deck follows the to-dos: one request, rendered at each step of its life,
// moves lane and state as its steps are reported — its intake ready (requests),
// its analysis ready (preparation), pulled (running), its implementation ready
// (build), then a person's review (waiting, in Needs a person, verify). A request
// whose requester was asked a question waits for that answer.
func TestTheDeckFollowsTheToDos(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	card := func(id string) (lane, state, page string) {
		t.Helper()
		def, err := views.Deck(w.ctx, api.DeckInput{})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		page = string(raw)
		m := regexp.MustCompile(`\{"id":"` + regexp.QuoteMeta(id) + `","lane":"([a-z]+)","meta":"[^"]*","state":"([a-z]+)"`).FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("no card of %s on the deck", id)
		}
		return m[1], m[2], page
	}
	want := func(id, lane, state string) string {
		t.Helper()
		gotLane, gotState, page := card(id)
		if gotLane != lane || gotState != state {
			t.Fatalf("%s: lane %s, %s; want %s, %s", id, gotLane, gotState, lane, state)
		}
		return page
	}

	r := file(t, w.ctx, w.req, api.FileInput{Title: "Build invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	if page := want(r.ID, api.PhaseRequests, "ready"); !strings.Contains(page, `"description":"Nothing waits on you"`) || !strings.Contains(page, `"label":"Needs you","value":0`) {
		t.Error("the Overview's Needs you tile counts something while nothing needs a person")
	}
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	want(r.ID, api.PhasePreparation, "ready")
	analyse := stepOf(t, w.ctx, r.ID, "analyse")
	if got, err := schedOf(t, w.ctx).Pull(w.ctx, schedapi.PullInput{ID: analyse.ID, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the analysis: %+v, %v", got, err)
	}
	want(r.ID, api.PhasePreparation, "running")
	if _, err := schedOf(t, w.ctx).Report(w.ctx, schedapi.ReportInput{ID: analyse.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "analysed", By: session}}); err != nil {
		t.Fatal(err)
	}
	want(r.ID, api.PhaseBuild, "ready")
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built"})
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	page := want(r.ID, api.PhaseVerify, "waiting")
	if !strings.Contains(page, `"label":"Waiting","value":1`) || !strings.Contains(page, "a person's answer (in the Inbox)") {
		t.Errorf("the person's review is not counted and shown as waiting for their answer")
	}

	q := file(t, w.ctx, w.req, api.FileInput{Title: "Price it", Module: "billing", Kind: api.KindBuild})
	if _, err := w.req.Ask(w.ctx, api.AskInput{ID: q.ID, Question: "Monthly or yearly?", By: intake}); err != nil {
		t.Fatal(err)
	}
	_, state, _ := card(q.ID)
	if state != "waiting" {
		t.Errorf("an asked request is %s; want waiting for its requester's answer", state)
	}
}

// The deck's targets are every module, each module and each open release; a
// release target keeps the deck to that release's work.
func TestTheDeckTargetsModulesAndReleases(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	in := w.planned("Build invoices", "billing", "P1")
	out := w.planned("Mail them", "billing", "P2")
	rel := plan(t, w.ctx, releases(t, w.ctx), api.PlanInput{Module: "billing", Version: "1.4.0", Name: "Billing 1.4"})
	if _, err := releases(t, w.ctx).Target(w.ctx, api.TargetInput{ID: in, Release: rel.ID}); err != nil {
		t.Fatal(err)
	}
	// The project's system module carries `module` too: it is left out by
	// the system-module capability, not by its type or its name.
	if _, err := w.req.Ensure(w.ctx, api.EnsureInput{}); err != nil {
		t.Fatal(err)
	}
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := views.DeckTargets(w.ctx, api.DeckTargetsInput{})
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string][]string{}
	for _, tg := range targets.Targets {
		groups[tg.Group] = append(groups[tg.Group], tg.Value)
	}
	if len(groups[""]) != 1 || !slices.Contains(groups["Modules"], "billing") || !slices.Contains(groups["Releases"], rel.ID) {
		t.Fatalf("targets = %+v; want all modules, billing among the modules, the release among the releases", targets.Targets)
	}
	if slices.Contains(groups["Modules"], api.SystemModule) {
		t.Errorf("the system module offered as a target: %v", groups["Modules"])
	}

	def, err := views.Deck(w.ctx, api.DeckInput{Target: rel.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def)
	if !strings.Contains(def.Title, "Billing 1.4") || !strings.Contains(string(raw), in) || strings.Contains(string(raw), out) {
		t.Errorf("release deck = title %q; want Billing 1.4 holding %s and not %s", def.Title, in, out)
	}
	if !strings.Contains(string(raw), `"pulse":["running"]`) {
		t.Errorf("the deck does not pulse its running work")
	}
}

// A request whose question was answered no longer waits for its requester,
// even when its steps wait on something else.
func TestAnAnsweredRequestIsNotAPersonsWait(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	dep := file(t, w.ctx, w.req, api.FileInput{Title: "Model prices", Module: "billing", Kind: api.KindBuild})
	q := file(t, w.ctx, w.req, api.FileInput{Title: "Price it", Module: "billing", Kind: api.KindBuild, DependsOn: []string{dep.ID}})
	if _, err := w.req.Ask(w.ctx, api.AskInput{ID: q.ID, Question: "Monthly or yearly?", By: intake}); err != nil {
		t.Fatal(err)
	}
	qs := intakeQuestions(t, w.ctx, q.ID)
	if len(qs) != 1 {
		t.Fatalf("questions asked about %s: %d; want one human task", q.ID, len(qs))
	}
	if _, err := schedOf(t, w.ctx).Report(w.ctx, schedapi.ReportInput{ID: qs[0].ID, Result: schedapi.Result{
		Status: schedapi.StatusSuccess, Summary: "answered", Answer: "monthly", By: person}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if now, err := w.req.Get(w.ctx, api.GetInput{ID: q.ID}); err == nil && now.Request.State == api.PlanReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the answer did not plan its intake again")
		}
		time.Sleep(20 * time.Millisecond)
	}
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, q.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	if a := stepOf(t, w.ctx, q.ID, "analyse"); a.State != schedapi.StateWaiting {
		t.Fatalf("its analysis: %s; want waiting on %s", a.State, dep.ID)
	}
	def, err := views.Deck(w.ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def)
	if strings.Contains(string(raw), "its requester's answer") {
		t.Errorf("the deck still waits for the requester's answer of %s: %s", q.ID, raw)
	}
}

// Needs you on the Overview is a tile counting what waits, no card; the
// cards are in « Needs a person ».
func TestNeedsYouCardsAreInNeedsAPersonNotOnTheOverview(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	for _, title := range []string{"Ship them", "Mail them", "Print them"} {
		r := file(t, w.ctx, w.req, api.FileInput{Title: title, Module: "billing", Kind: api.KindBuild})
		failPastTheCap(t, w.ctx, stepOf(t, w.ctx, r.ID, api.StepIntake).ID, "no SMTP") // past the cap: the person\'s
	}
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(w.ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	tabs := def.DSLTree.(map[string]any)["tabs"].([]map[string]any)
	overview, _ := json.Marshal(tabs[0]["content"])
	page := string(overview)
	if !strings.Contains(page, `"data-view":"deck-needs-tile","description":"3 to unblock"`) || strings.Contains(page, `"todo":`) {
		t.Errorf("the Overview: want the Needs you tile counting 3 and no card: %s", page)
	}
	var needs []byte
	for _, tab := range tabs {
		if tab["id"] == "needs" {
			needs, _ = json.Marshal(tab["content"])
		}
	}
	if n := strings.Count(string(needs), `"todo":`); n != 3 {
		t.Errorf("Needs a person holds %d cards; want the 3 waiting: %s", n, needs)
	}
}

// The deck reads the scheduler once: every part shares one listing of the
// to-dos and one queue (it read them again for each request, slow on a
// workspace of many).
func TestTheDeckReadsTheSchedulerOnce(t *testing.T) {
	// Not in parallel: it counts the scheduler reads of the whole process.
	w := newPlannerWorld(t)
	for i, m := range []string{"billing", "shipping", "billing", "stock"} {
		file(t, w.ctx, w.req, api.FileInput{Title: "Work " + string(rune('A'+i)), Module: m, Kind: api.KindBuild})
	}
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := plancore.SchedulerReads()
	if _, err := views.Deck(w.ctx, api.DeckInput{}); err != nil {
		t.Fatal(err)
	}
	if reads := plancore.SchedulerReads() - before; reads != 2 {
		t.Errorf("one deck read the scheduler %d times; want 2 (one listing, one queue)", reads)
	}
}

// The By module tab: one card per module with open work, what needs attention
// first first, with its counts, what is held and by whom, and its next step;
// a click opens the module on its own planning deck.
func TestTheDeckHasAByModuleTab(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	ready := file(t, w.ctx, w.req, api.FileInput{Title: "Build invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, w.ctx, stepOf(t, w.ctx, ready.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	held := file(t, w.ctx, w.req, api.FileInput{Title: "Mail them", Module: "billing", Kind: api.KindBuild})
	if got, err := schedOf(t, w.ctx).Pull(w.ctx, schedapi.PullInput{ID: stepOf(t, w.ctx, held.ID, api.StepIntake).ID, By: another}); err != nil || got.Todo == nil {
		t.Fatalf("pull the intake: %+v, %v", got, err)
	}
	stuck := file(t, w.ctx, w.req, api.FileInput{Title: "Ship them", Module: "shipping", Kind: api.KindBuild})
	failPastTheCap(t, w.ctx, stepOf(t, w.ctx, stuck.ID, api.StepIntake).ID, "no SMTP") // past the cap: the person\'s

	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(w.ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("off the page grammar: %v", err)
	}
	tab := ""
	for _, p := range def.DSLTree.(map[string]any)["tabs"].([]map[string]any) {
		if p["id"] == "by-module" {
			raw, _ := json.Marshal(p["content"])
			tab = string(raw)
		}
	}
	if tab == "" {
		t.Fatal("no By module tab")
	}
	ship, bill := strings.Index(tab, `"id":"shipping"`), strings.Index(tab, `"id":"billing"`)
	if ship < 0 || bill < 0 || ship > bill {
		t.Errorf("modules shipping at %d, billing at %d; want both, the blocked one (shipping) first: %s", ship, bill, tab)
	}
	for _, want := range []string{"1 blocked", "held: " + another.ID, "next: analyse", `"panel":"planning-deck"`} {
		if !strings.Contains(tab, want) {
			t.Errorf("the By module tab lacks %q: %s", want, tab)
		}
	}
}

// The Done tile shows the requests ended done today, this week and this month
// at once: "Day/Week/Month" over their three counts - no switch.
func TestTheDoneTileShowsTheDayWeekAndMonthPeriods(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDone, By: person, Note: "done by hand"}); err != nil {
		t.Fatal(err)
	}
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	page := string(raw)
	if !strings.Contains(page, `"label":"Day/Week/Month"`) || !strings.Contains(page, `"value":"1/1/1"`) {
		t.Errorf("the Done tile: want Day/Week/Month over its three counts")
	}
	if strings.Contains(page, `"query":{"args":{"period"`) {
		t.Error("the Done tile still offers a switch")
	}
}
