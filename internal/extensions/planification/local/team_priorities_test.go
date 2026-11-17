// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// TEAM PRIORITIES (view 7 of the mock-up
// delivery-views-v2-mockup.html): the increments of the streams shown in the
// order they will be done - delivered in the current release, a group per
// open release by due date, no release - a separator per group with its release's due date, days
// left and how much is done, by priority then rank inside; the stream and
// delivered toggles; each line's drop-downs run board-move and board-ship.

type teamWorld struct {
	ctx                     context.Context
	cwm                     cwmapi.CWMService
	inc                     api.IncrementService
	rel                     api.ReleaseService
	product, docs           string
	current, next, later    string
	delivered, older        string
	curP2, curP0a, curP0b   string
	inNext, inLater, inNone string
	docsOne                 string
}

func teamPrioritiesWorld(t *testing.T) teamWorld {
	t.Helper()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	w := teamWorld{ctx: ctx, cwm: cwm, inc: inc, rel: rel}
	mk := func(name string, order int, color string) string {
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual, Order: order, Color: color}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	w.product, w.docs = mk("Product", 1, "#3651D4"), mk("Docs", 2, "")
	w.current = plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2099-10-31", Stream: w.product}).ID
	w.next = plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2099-11-30", Stream: w.product}).ID
	w.later = excepted(t, ctx, cwm, rel, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2099-12-31", Stream: w.product}).ID
	open := func(stream, title, priority string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: stream, Title: title, Description: "What " + title + " gives.", By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: stream}); err != nil {
			t.Fatal(err)
		}
		if priority != "" {
			if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: out.Increment.ID, Column: priority, By: person}); err != nil || !r.OK {
				t.Fatalf("priority %s: %+v %v", priority, r, err)
			}
		}
		return out.Increment.ID
	}
	ship := func(id, release string) {
		t.Helper()
		if r, err := inc.BoardShip(ctx, api.BoardShipInput{ID: id, In: release, By: person}); err != nil || !r.OK {
			t.Fatalf("ship %s in %s: %+v %v", id, release, r, err)
		}
	}
	done := func(id string) {
		t.Helper()
		if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: id, By: person, Force: true, Note: forcedNote}); err != nil {
			t.Fatal(err)
		}
	}
	w.delivered = open(w.product, "Invoices by mail", "P1")
	ship(w.delivered, w.current)
	done(w.delivered)
	w.older = open(w.product, "Older history", "")
	ship(w.older, w.next)
	done(w.older)
	w.curP2 = open(w.product, "Reminders", "P2")
	ship(w.curP2, w.current)
	w.curP0a = open(w.product, "Payments", "P0")
	ship(w.curP0a, w.current)
	w.curP0b = open(w.product, "Refunds", "P0")
	ship(w.curP0b, w.current)
	w.inNext = open(w.product, "Statements", "P1")
	ship(w.inNext, w.next)
	w.inLater = open(w.product, "Exports", "P3")
	ship(w.inLater, w.later)
	w.inNone = open(w.product, "A loose idea", "P2")
	w.docsOne = open(w.docs, "A user guide", "P1")
	return w
}

func (w teamWorld) data(t *testing.T, in api.TeamPrioritiesInput) api.TeamPrioritiesOutput {
	t.Helper()
	out, err := w.inc.TeamPriorities(w.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// order is each group's kind and its lines' ids.
func teamOrder(d api.TeamPrioritiesOutput) []string {
	var out []string
	for _, g := range d.Groups {
		for _, l := range g.Lines {
			out = append(out, g.Kind+":"+l.ID)
		}
	}
	return out
}

func TestTeamPrioritiesOrder(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	got := teamOrder(w.data(t, api.TeamPrioritiesInput{}))
	want := []string{
		"delivered:" + w.delivered,
		"current:" + w.curP0a, "current:" + w.curP0b, "current:" + w.curP2,
		"next:" + w.inNext,
		"release:" + w.inLater,
		"none:" + w.docsOne, "none:" + w.inNone, // Docs has no release; P1 before P2 across the streams
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the order:\n got %v\nwant %v", got, want)
	}
	for _, g := range w.data(t, api.TeamPrioritiesInput{}).Groups {
		for _, l := range g.Lines {
			if l.ID == w.older {
				t.Error("an increment delivered in another release than the current one is shown: never the older history")
			}
		}
	}
}

func TestTeamPrioritiesSeparators(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	d := w.data(t, api.TeamPrioritiesInput{})
	def, err := w.inc.TeamPrioritiesPage(w.ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("Team priorities must display: %v", err)
	}
	b, _ := json.Marshal(def.DSLTree)
	page := string(b)
	for _, g := range d.Groups {
		switch g.Kind {
		case api.TeamGroupCurrent, api.TeamGroupNext, api.TeamGroupRelease:
			if len(g.Releases) != 1 || g.Releases[0].Date == "" || g.Releases[0].DaysLeft == nil || g.Stream != w.product {
				t.Errorf("the %s separator names %+v; want its release with its due date and days left", g.Kind, g.Releases)
			}
		case api.TeamGroupNone:
			if len(g.Releases) != 0 {
				t.Errorf("No release names releases: %+v", g.Releases)
			}
		}
	}
	for _, want := range []string{`"id":"tp-sep:delivered"`, `"id":"tp-sep:current:` + w.current + `"`, `"id":"tp-sep:next:` + w.next + `"`,
		`"id":"tp-sep:release:` + w.later + `"`, `"id":"tp-sep:none"`, `"data-stream":"` + w.product + `"`,
		"Product · Current release", "Product · Next release", "Product · Release", "due 31/10 · ", " days left · 3 increments · 0% done", "due 30/11", "due 31/12", "not planned · 2 increments", "Delivered in the current releases"} {
		if !strings.Contains(page, want) {
			t.Errorf("the separators lack %q", want)
		}
	}
	if strings.Contains(page, "not planned · 2 increments · ") {
		t.Error("No release shows how much is done")
	}
}

func TestTeamPrioritiesFilters(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	d := w.data(t, api.TeamPrioritiesInput{HiddenStreams: []string{w.docs}})
	if len(d.Streams) != 2 || !d.Streams[0].On || d.Streams[1].On || d.Streams[0].ID != w.product {
		t.Fatalf("the stream toggles: %+v; want Product on, Docs off", d.Streams)
	}
	for _, id := range teamOrder(d) {
		if strings.HasSuffix(id, w.docsOne) {
			t.Error("a hidden stream's increment is shown")
		}
	}
	d = w.data(t, api.TeamPrioritiesInput{HideDelivered: true})
	for _, g := range d.Groups {
		if g.Kind == api.TeamGroupDelivered {
			t.Error("the delivered are shown though hidden")
		}
	}
	// The toggles re-read the page with the stream flipped and the delivered flipped.
	def, err := w.inc.TeamPrioritiesPage(w.ctx, api.TeamPrioritiesInput{HiddenStreams: []string{w.docs}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	page := string(b)
	for _, want := range []string{
		`"query":{"args":{"hidden_streams":["` + w.docs + `","` + w.product + `"],"hide_delivered":false}}`,
		`"query":{"args":{"hidden_streams":[],"hide_delivered":false}}`,
		`"query":{"args":{"hidden_streams":["` + w.docs + `"],"hide_delivered":true}}`,
		`"id":"tp-delivered"`, `"color":"blue","data-on":true,"data-stream":"` + w.product + `","id":"tp-stream:` + w.product + `"`,
		`{"color":"#3651D4","kind":"monogram","size":10,"text":" "}`} {
		if !strings.Contains(page, want) {
			t.Errorf("the toggles lack %s", want)
		}
	}
	if d := w.data(t, api.TeamPrioritiesInput{HiddenStreams: []string{w.docs, w.product}}); len(d.Groups) != 0 {
		t.Errorf("no stream shown, yet %d groups", len(d.Groups))
	}
}

func TestTeamPrioritiesVerbs(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	def, err := w.inc.TeamPrioritiesPage(w.ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	page := string(b)
	for _, want := range []string{
		`"id":"tp-rows:current:` + w.current + `"`, `"id":"tp-rows:none"`, `"schema":"ktw-agents:/data-type/increment-row"`, `"variant":"team"`,
		`"navigate":{"object":"${item.id}"}`, `"id":"` + w.inNext + `"`, `"release":"1.1.0"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the rows lack %s", want)
		}
	}
	// The verbs the drop-downs run move it at once, and the list re-sorts.
	if r, err := w.inc.BoardMove(w.ctx, api.BoardMoveInput{ID: w.curP2, Column: "P0", By: person}); err != nil || !r.OK {
		t.Fatalf("priority: %+v %v", r, err)
	}
	if r, err := w.inc.BoardShip(w.ctx, api.BoardShipInput{ID: w.inNone, In: w.later, By: person}); err != nil || !r.OK {
		t.Fatalf("a later release: %+v %v", r, err)
	}
	if r, err := w.inc.BoardShip(w.ctx, api.BoardShipInput{ID: w.inLater, In: w.current, By: person}); err != nil || !r.OK {
		t.Fatalf("the current release: %+v %v", r, err)
	}
	if r, err := w.inc.BoardShip(w.ctx, api.BoardShipInput{ID: w.inNext, In: "out", By: person}); err != nil || !r.OK {
		t.Fatalf("no release: %+v %v", r, err)
	}
	got := strings.Join(teamOrder(w.data(t, api.TeamPrioritiesInput{HiddenStreams: []string{w.docs}})), " ")
	want := strings.Join([]string{"delivered:" + w.delivered,
		"current:" + w.curP2, "current:" + w.curP0a, "current:" + w.curP0b, "current:" + w.inLater, // P0 now: the older first
		"release:" + w.inNone, "none:" + w.inNext}, " ")
	if got != want {
		t.Fatalf("after the changes:\n got %s\nwant %s", got, want)
	}
	for _, id := range []string{w.inNone} {
		g, _ := w.inc.Get(w.ctx, api.GetInput{ID: id})
		if g.Increment.PlannedFor != w.later || g.Increment.ShipsIn != "" {
			t.Errorf("a later release plans it (planned-for): %+v", g.Increment)
		}
	}
	if g, _ := w.inc.Get(w.ctx, api.GetInput{ID: w.inLater}); g.Increment.ShipsIn != w.current {
		t.Errorf("the current release ships it in: %+v", g.Increment)
	}
}

func TestTeamPrioritiesLine(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	if r, err := w.inc.BoardMove(w.ctx, api.BoardMoveInput{ID: w.inNone, Column: "held", Reason: "waits for the new mail server", By: person}); err != nil || !r.OK {
		t.Fatalf("hold: %+v %v", r, err)
	}
	var line api.TeamLine
	for _, g := range w.data(t, api.TeamPrioritiesInput{}).Groups {
		for _, l := range g.Lines {
			if l.ID == w.inNone {
				line = l
			}
		}
	}
	if line.Description != "What A loose idea gives." || !strings.Contains(line.Blocks, "waits for the new mail server") {
		t.Errorf("the line: %+v; want its description and what blocks it", line)
	}
	def, err := w.inc.TeamPrioritiesPage(w.ctx, api.TeamPrioritiesInput{Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	page := string(b)
	for _, want := range []string{"Priorités d'équipe", "Version actuelle", "Version suivante", `"content":"Product · Version"`, "Sans version", "Livré dans la version actuelle",
		"jours restants", `"description":"What A loose idea gives."`, `"held":"waits for the new mail server"`} {
		if !strings.Contains(page+def.Title, want) {
			t.Errorf("the French page lacks %q", want)
		}
	}
}

// A frozen current release takes no more increments: it is offered only on
// the lines already in it, never as a target to the others.
func TestTeamPrioritiesVerbsFrozen(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	if _, err := w.rel.Freeze(w.ctx, api.MoveInput{ID: w.current, By: person}); err != nil {
		t.Fatal(err)
	}
	offers := func(id string) []string {
		t.Helper()
		for _, g := range w.data(t, api.TeamPrioritiesInput{}).Groups {
			for _, l := range g.Lines {
				if l.ID == id {
					var vs []string
					for _, o := range l.Options {
						vs = append(vs, o.Value)
					}
					return vs
				}
			}
		}
		t.Fatalf("%s is not listed", id)
		return nil
	}
	if got := strings.Join(offers(w.curP2), " "); got != w.current+" "+w.next+" "+w.later+" out" {
		t.Errorf("a line in the frozen current release offers %s; want it, then the next, the later, none", got)
	}
	for _, id := range []string{w.inNext, w.inLater, w.inNone} {
		if got := offers(id); strings.Contains(strings.Join(got, " "), w.current) {
			t.Errorf("%s is offered the frozen current release: %v", id, got)
		}
	}
	def, err := w.inc.TeamPrioritiesPage(w.ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(def.DSLTree)
	if page := string(b); !strings.Contains(page, `"release":"1.1.0"`) {
		t.Error("the rows no longer name the release they ship in")
	}
}

// An undated release is ordered on its own date - after the dated ones -
// never on the periodic cut it is placed at, though that cut falls before
// them; it shows no due date (Team priorities and the Overview's lanes).
func TestTeamPrioritiesUndatedReleaseLast(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Packages", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d", Order: 1}})
	if err != nil {
		t.Fatal(err)
	}
	undated := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Stream: st.Stream.ID}).ID
	dated := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.9.0", Date: "2099-11-30", Stream: st.Stream.ID}).ID
	for _, in := range []string{dated, undated} {
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: st.Stream.ID, Title: "Work for " + in, Description: "What it gives.", By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if r, err := inc.BoardShip(ctx, api.BoardShipInput{ID: out.Increment.ID, In: in, By: person}); err != nil || !r.OK {
			t.Fatalf("ship in %s: %+v %v", in, r, err)
		}
	}
	d, err := inc.TeamPriorities(ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range d.Groups {
		for _, r := range g.Releases {
			got = append(got, g.Kind+":"+r.ID+"@"+r.Date)
		}
	}
	want := api.TeamGroupCurrent + ":" + dated + "@2099-11-30 " + api.TeamGroupNext + ":" + undated + "@"
	if strings.Join(got, " ") != want {
		t.Errorf("Team priorities' releases: %v; want %s - the dated current, the undated next with no due date", got, want)
	}
	lanes, err := sm.Lanes(ctx, api.LanesInput{ID: st.Stream.ID})
	if err != nil || len(lanes.Lanes) != 1 {
		t.Fatalf("lanes: %+v %v", lanes, err)
	}
	var ids []string
	for _, c := range lanes.Lanes[0].Releases {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, " ") != dated+" "+undated {
		t.Errorf("the lane's releases: %v; want %s then the undated %s", ids, dated, undated)
	}
}

// Every open release of the streams shown is a group of its own - each
// stream's current release too - ordered by due date with the streams'
// releases interleaved (a same date by stream order, the undated last), its
// increments by priority then rank; no Later bucket.
func TestTeamPrioritiesReleaseSections(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	mk := func(name string, order int) string {
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual, Order: order}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	a, b := mk("Alpha", 1), mk("Beta", 2)
	a1 := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2099-10-10", Stream: a}).ID
	a2 := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2099-12-10", Stream: a}).ID
	b1 := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.1.0", Date: "2099-11-10", Stream: b}).ID
	b2 := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.2.0", Date: "2099-12-10", Stream: b}).ID
	b3 := excepted(t, ctx, cwm, rel, api.PlanInput{Module: "billing", Version: "2.3.0", Stream: b}).ID
	open := func(stream, release, priority string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: stream, Title: "Work in " + release, Description: "What it gives.", By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: stream}); err != nil {
			t.Fatal(err)
		}
		if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: out.Increment.ID, Column: priority, By: person}); err != nil || !r.OK {
			t.Fatalf("priority %s: %+v %v", priority, r, err)
		}
		if r, err := inc.BoardShip(ctx, api.BoardShipInput{ID: out.Increment.ID, In: release, By: person}); err != nil || !r.OK {
			t.Fatalf("ship in %s: %+v %v", release, r, err)
		}
		return out.Increment.ID
	}
	b3x := open(b, b3, "P1")
	a1low := open(a, a1, "P2")
	a1top := open(a, a1, "P0")
	b1old := open(b, b1, "P1")
	b1new := open(b, b1, "P1")
	b2x := open(b, b2, "P2")
	a2x := open(a, a2, "P3")
	d, err := inc.TeamPriorities(ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range d.Groups {
		var rels, lines []string
		for _, r := range g.Releases {
			rels = append(rels, r.ID)
		}
		for _, l := range g.Lines {
			lines = append(lines, l.ID)
		}
		got = append(got, g.Kind+"@"+g.Stream+"/"+strings.Join(rels, ",")+"="+strings.Join(lines, ","))
	}
	want := []string{
		api.TeamGroupCurrent + "@" + a + "/" + a1 + "=" + a1top + "," + a1low,
		api.TeamGroupCurrent + "@" + b + "/" + b1 + "=" + b1old + "," + b1new,
		api.TeamGroupNext + "@" + a + "/" + a2 + "=" + a2x,
		api.TeamGroupNext + "@" + b + "/" + b2 + "=" + b2x,
		api.TeamGroupRelease + "@" + b + "/" + b3 + "=" + b3x,
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the release groups:\n got %v\nwant %v", got, want)
	}
	def, err := inc.TeamPrioritiesPage(ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	bj, _ := json.Marshal(def.DSLTree)
	page := string(bj)
	for _, want := range []string{`"id":"tp-sep:current:` + a1 + `"`, `"id":"tp-sep:current:` + b1 + `"`, "Alpha · Current release", "Beta · Current release", "Beta · Release"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if strings.Contains(page, "Later") {
		t.Error("the page still has a Later section")
	}
}
