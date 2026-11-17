// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// DELIVERY'S RUNNING TAB (view 2 variant B of the mock-up
// delivery-views-v2-mockup.html): only the increments being built, each
// unfolded - id, title, state badge, done of total, release - then its open
// requests only, the first three with their step dots, « + n more » for the
// rest; done requests only counted; a legend on top; every line opens.

type runningWorld struct {
	ctx                context.Context
	sm                 api.StreamService
	built, other, idle string
	defined, held      string
	reqs               map[string]string // the requests filed into built, by the state they are put in
	otherReq, idleReq  string
	otherBlocked       string
	current, later     string // the releases: built ships in current, idle is planned for later
}

func runningIncrementsWorld(t *testing.T) runningWorld {
	t.Helper()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	w := runningWorld{ctx: ctx, sm: sm, reqs: map[string]string{}}
	product, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	current := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2099-10-31", Stream: product.Stream.ID})
	w.current = current.ID
	mk := func(title, priority string, run bool) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{"What " + title + " gives"}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		id := out.Increment.ID
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: product.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: id, Column: priority, By: person}); err != nil || !r.OK {
			t.Fatalf("priority %s: %+v %v", priority, r, err)
		}
		if run {
			runIncrement(t, ctx, id)
		}
		return id
	}
	into := func(increment, title string) string {
		t.Helper()
		return file(t, ctx, req, api.FileInput{Title: title, Module: "billing", Increment: increment, Kind: api.KindBuild, By: person}).ID
	}
	w.built = mk("Invoices get paid", "P2", true)
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: w.built, In: current.ID}); err != nil {
		t.Fatal(err)
	}
	w.other = mk("Reminders", "P0", true)
	w.idle = mk("Statements", "P1", true)
	// Planned for a later release, not shipped in one: its release is that one.
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2099-11-30", Stream: product.Stream.ID})
	w.later = later.ID
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: w.idle, In: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	w.defined = mk("A loose idea", "P0", false) // never run: defined
	w.held = mk("Archive", "P0", true)
	if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: w.held, Column: "held", Reason: "waits on legal", By: person}); err != nil || !r.OK {
		t.Fatalf("hold: %+v %v", r, err)
	}
	into(w.held, "Archive old invoices")

	// built's requests, filed in this order, each put in its state.
	for _, name := range []string{"todo-a", "done", "dropped", "failed", "blocked", "on-you", "running", "todo-b"} {
		w.reqs[name] = into(w.built, "Request "+name)
	}
	s := schedOf(t, ctx)
	intake := func(name string) string { return stepOf(t, ctx, w.reqs[name], api.StepIntake).ID }
	pullAndReport(t, ctx, intake("blocked"), session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "what does done mean?"})
	pullAndReport(t, ctx, intake("failed"), session, schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke"})
	onYou := func(id string) {
		t.Helper()
		pullAndReport(t, ctx, stepOf(t, ctx, id, api.StepIntake).ID, session, schedapi.Result{Summary: "assessed",
			More: map[string]any{"add": []map[string]any{{"step": "go", "doer": "person", "interaction": "validate", "instructions": "Say go."}}}})
	}
	onYou(w.reqs["on-you"])
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: w.reqs["done"], Outcome: api.PlanDone, By: person, Note: "done by hand"}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: w.reqs["dropped"], Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}
	w.otherReq = into(w.other, "Remind by mail")
	onYou(w.otherReq)
	w.otherBlocked = into(w.other, "Remind by phone")
	pullAndReport(t, ctx, stepOf(t, ctx, w.otherBlocked, api.StepIntake).ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "which phone?"})
	w.idleReq = into(w.idle, "Monthly statement")
	// Another worker holds the running request's intake.
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: intake("running"), By: cwmapi.Actor{ID: "session-b", Kind: "agent"}}); err != nil {
		t.Fatal(err)
	}
	return w
}

// runningTab is the Running tab: its label, its count and its tree.
type runningTab struct {
	label string
	badge int
	tree  map[string]any
	raw   string
}

func (w runningWorld) tab(t *testing.T, lang string) runningTab {
	t.Helper()
	def, err := w.sm.DeliveryPage(w.ctx, api.DeliveryPageInput{Target: "billing", Language: lang})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the Delivery view must display: %v", err)
	}
	var tree struct {
		Tabs []struct {
			ID      string         `json:"id"`
			Label   string         `json:"label"`
			Badge   int            `json:"badge"`
			Content map[string]any `json:"content"`
		} `json:"tabs"`
	}
	raw, _ := json.Marshal(def.DSLTree)
	_ = json.Unmarshal(raw, &tree)
	var ids []string
	for _, tb := range tree.Tabs {
		ids = append(ids, tb.ID)
	}
	if strings.Join(ids, ",") != "planned,running,open,delivered" {
		t.Fatalf("the tabs %v; want planned, running, open, delivered - Running beside Planned delivery, as the mock-up", ids)
	}
	tb := tree.Tabs[1]
	// Running is a section loaded when the tab opens:
	// its rows are what RunningPage answers.
	if load, _ := tb.Content["load"].(map[string]any); load["method"] != "ktw-agents:streams/running-page" {
		t.Fatalf("the Running tab is not a section loading streams/running-page: %v", tb.Content)
	}
	page, err := w.sm.RunningPage(w.ctx, api.DeliveryPageInput{Target: "billing", Language: lang})
	if err != nil {
		t.Fatal(err)
	}
	if err := page.Validate(); err != nil {
		t.Fatalf("the Running tab must display: %v", err)
	}
	b, _ := json.Marshal(page.DSLTree)
	var content map[string]any
	_ = json.Unmarshal(b, &content)
	return runningTab{label: tb.Label, badge: tb.Badge, tree: content, raw: string(b)}
}

// lines are the tab's lines in order: « IN:<id>:<state> » for an
// increment's head, « RQ:<id>:<status>:<dots> » for a request, « MORE:<id>:<label> ».
func (r runningTab) lines() []string {
	var out []string
	var walk func(n any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			switch {
			case v["data-increment"] != nil:
				// The increment's card: its head, then its rows inside it.
				out = append(out, "IN:"+v["data-increment"].(string)+":"+v["data-state"].(string))
			case v["data-request"] != nil:
				out = append(out, "RQ:"+v["data-request"].(string)+":"+v["data-state"].(string)+":"+strings.Join(dotsOf(v), ","))
				return
			case v["data-more"] != nil:
				out = append(out, "MORE:"+v["data-more"].(string)+":"+v["text"].(string))
				return
			}
			for _, k := range []string{"children", "item", "data", "card", "panels", "content"} {
				walk(v[k])
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(r.tree)
	return out
}

// dotsOf are a row's step dots: « <step>=<status> ».
func dotsOf(n any) []string {
	var out []string
	var walk func(n any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["kind"] == "stepper" {
				for _, s := range v["steps"].([]any) {
					st := s.(map[string]any)
					out = append(out, st["id"].(string)+"="+st["status"].(string))
				}
				return
			}
			for _, k := range []string{"children", "item"} {
				walk(v[k])
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(n)
	return out
}

// Only the increments being built, by priority then id, each with its state
// badge, done of total and release; the tab's count is theirs.
func TestDeliveryRunningIncrements(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	tab := w.tab(t, "")
	if tab.label != "Running" || tab.badge != 3 {
		t.Errorf("the tab: %q (%d); want Running (3): the three increments being built", tab.label, tab.badge)
	}
	var heads []string
	for _, l := range tab.lines() {
		if strings.HasPrefix(l, "IN:") {
			heads = append(heads, l)
		}
	}
	want := []string{"IN:" + w.other + ":on-you", "IN:" + w.idle + ":ready", "IN:" + w.built + ":running"}
	if strings.Join(heads, " ") != strings.Join(want, " ") {
		t.Errorf("the increments:\n got %v\nwant %v (P0, P1, P2; never the defined %s nor the held %s)", heads, want, w.defined, w.held)
	}
	for _, s := range []string{`"content":"Invoices get paid"`, `"text":"running","variant":"light"`, `"color":"orange"`, `"color":"grape","kind":"badge","size":"xs","text":"on you"`,
		`"color":"blue","kind":"badge","size":"xs","text":"ready"`, `"data-view":"delivery-running-cards"`, `"nodeField":"card"`, `"toneField":"tone"`, `"tone":"orange"`, `"content":"1/7 · 1.1.0 · Product"`, `"content":"0/1 · 1.2.0 · Product"`, `"navigate":{"object":"` + w.built + `"}`} {
		if !strings.Contains(tab.raw, s) {
			t.Errorf("the tab lacks %s: %s", s, tab.raw)
		}
	}
}

// An increment shows its open requests only - running, waiting on a person,
// blocked, then the rest, by id - the first three, then « + n more » opening
// its Requests tab; its done and dropped ones are only counted.
func TestDeliveryRunningRequests(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	tab := w.tab(t, "")
	var built []string
	in := false
	for _, l := range tab.lines() {
		if strings.HasPrefix(l, "IN:") {
			in = strings.HasPrefix(l, "IN:"+w.built+":")
			continue
		}
		if in {
			id, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(l, "RQ:"), "MORE:"), ":")
			built = append(built, l[:strings.Index(l, ":")]+":"+id)
		}
	}
	want := []string{"RQ:" + w.reqs["running"], "RQ:" + w.reqs["on-you"], "RQ:" + w.reqs["failed"], "MORE:" + w.built}
	if strings.Join(built, " ") != strings.Join(want, " ") {
		t.Errorf("built's rows:\n got %v\nwant %v", built, want)
	}
	for _, s := range []string{`"text":"+ 3 more"`, `"panel":"tab:increment-open-requests"`, `"navigate":{"object":"` + w.reqs["running"] + `"}`, `"content":"Request running"`} {
		if !strings.Contains(tab.raw, s) {
			t.Errorf("the tab lacks %s", s)
		}
	}
	for _, not := range []string{`"data-request":"` + w.reqs["done"] + `"`, `"data-request":"` + w.reqs["dropped"] + `"`, "Archive old invoices"} {
		if strings.Contains(tab.raw, not) {
			t.Errorf("the tab shows %s: a done or dropped request, or one of a held increment", not)
		}
	}
	// No « more » where all its open requests show.
	if strings.Contains(tab.raw, `"data-more":"`+w.other+`"`) {
		t.Error("« + n more » on an increment showing all its open requests")
	}
}

// Each step's dot says its state: done, running, waiting on a person, failed
// or blocked, to come; the legend names them, in French for a French person.
func TestDeliveryRunningDots(t *testing.T) {
	long.Test(t)
	t.Parallel()
	w := runningIncrementsWorld(t)
	tab := w.tab(t, "")
	rows := map[string]string{}
	for _, l := range tab.lines() {
		if strings.HasPrefix(l, "RQ:") {
			parts := strings.SplitN(l, ":", 4)
			rows[parts[1]] = parts[3]
		}
	}
	for name, first := range map[string]string{
		"running": "intake=running,analyse=todo",
		"failed":  "intake=failed,analyse=todo",
	} {
		if got := rows[w.reqs[name]]; !strings.HasPrefix(got, first) {
			t.Errorf("the %s request's dots: %s; want them to start %s", name, got, first)
		}
	}
	if got := rows[w.reqs["on-you"]]; !strings.HasPrefix(got, "intake=done,") || !strings.Contains(got, "go=on-you") {
		t.Errorf("the on-you request's dots: %s; want intake done and its person's step go on-you", got)
	}
	if got := rows[w.idleReq]; !strings.HasPrefix(got, "intake=todo,") {
		t.Errorf("a request not started: %s; want every dot to come", got)
	}
	if got := rows[w.otherBlocked]; !strings.HasPrefix(got, "intake=blocked,analyse=todo") {
		t.Errorf("the blocked request's dots: %s; want them to start intake=blocked", got)
	}
	for _, s := range []string{`"data-view":"delivery-running-legend"`, `"status":"on-you"`, `"content":"waits on you"`, `"content":"failed / blocked"`, `"content":"to come"`} {
		if !strings.Contains(tab.raw, s) {
			t.Errorf("the legend lacks %s", s)
		}
	}
	fr := w.tab(t, "fr")
	if fr.label != "En cours" {
		t.Errorf("the tab in French: %q; want En cours", fr.label)
	}
	if !strings.Contains(fr.raw, `"label":"Ouvrir `+w.reqs["running"]+`"`) {
		t.Errorf("a request row's click in French: want « Ouvrir %s », like its increment's head", w.reqs["running"])
	}
	for _, s := range []string{`"text":"à vous"`, `"text":"prêt"`, `"text":"en cours"`, `"content":"vous attend"`, `"content":"à venir"`, `"text":"+ 3 de plus"`} {
		if !strings.Contains(fr.raw, s) {
			t.Errorf("the French tab lacks %s", s)
		}
	}
}
