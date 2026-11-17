// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE SHOWS WHAT IT DELIVERED OR WHAT HOLDS IT BACK — its pane lists
// its increments by priority, how many are done; each held one says what it
// waits on (its hold, an increment it depends on still open, a goal or
// feature no work serves); a stream that cuts its releases shows on its coming
// release the increments done since the last one.
func TestAReleaseShowsWhatItDeliveredOrWhatHoldsItBack(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Stream: st.Stream.ID})
	in := func(title string) string {
		t.Helper()
		id := increment(t, ctx, title).ID
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: id, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: r.ID}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	ready, running, held, waiting, unplanned := in("Invoices by mail"), in("Reminders"), in("Dunning"), in("Payment links"), in("Exports")
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: ready, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: held, Reason: "the bank's API is late", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: held, In: r.ID}); err != nil {
		t.Fatal(err) // a hold leaves its release: put back to be seen there
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: waiting, To: running, Kind: api.EdgeDependsOn}); err != nil {
		t.Fatal(err)
	}
	uc := usecase(t, ctx, cwm, "Export to CSV")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: unplanned}); err != nil {
		t.Fatal(err)
	}

	page := func(id string) (string, map[string]map[string]any) {
		t.Helper()
		def, err := releasePane(ctx, rel, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the release's pane must display: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree any
		_ = json.Unmarshal(raw, &tree)
		rows := map[string]map[string]any{}
		var walk func(n any, section string)
		walk = func(n any, section string) {
			switch v := n.(type) {
			case map[string]any:
				if v["kind"] == "card" {
					if kids, _ := v["children"].([]any); len(kids) > 0 {
						if h, _ := kids[0].(map[string]any); h["kind"] == "heading" {
							section, _ = h["text"].(string)
						}
					}
				}
				if v["kind"] == "tree" {
					for _, d := range v["data"].([]any) {
						row := d.(map[string]any)
						row["section"] = section
						rows[row["id"].(string)] = row
					}
				}
				for _, c := range v {
					walk(c, section)
				}
			case []any:
				for _, c := range v {
					walk(c, section)
				}
			}
		}
		walk(tree, "")
		return string(raw), rows
	}
	raw, rows := page(r.ID)
	for _, want := range []string{`"label":"Increments"`, `"valueText":"1/5"`, "NOT READY"} {
		if !strings.Contains(raw, want) {
			t.Errorf("the pane lacks %s", want)
		}
	}
	const carried = "Increments of this release"
	for id, want := range map[string]struct{ section, meta string }{
		ready:     {carried, ""},
		running:   {carried, ""},
		held:      {carried, "waits: on hold: the bank's API is late"},
		waiting:   {carried, "waits: waits on Reminders"},
		unplanned: {carried, "waits: Export to CSV: no work planned"},
	} {
		row := rows[id]
		if row == nil || row["section"] != want.section || !strings.Contains(row["meta"].(string), want.meta) {
			t.Errorf("%s: %v; want in %s saying %q", id, row, want.section, want.meta)
		}
	}

	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(d.Panels, func(p objectpaneapi.ObjectPanel) bool { return p.ID == "tab:release" }) {
		t.Errorf("the Release panel is not on its pane: %+v", d.Panels)
	}

	// A periodic stream's coming release: what its next cut takes.
	pst, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Platform", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d"}})
	if err != nil {
		t.Fatal(err)
	}
	coming := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Stream: pst.Stream.ID})
	cut := increment(t, ctx, "Faster startup").ID
	if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: cut, Stream: pst.Stream.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: cut, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	if _, rows := page(coming.ID); rows[cut] == nil || rows[cut]["section"] != "Riding along" || !strings.Contains(rows[cut]["meta"].(string), "its freeze takes it in") {
		t.Errorf("the periodic stream's coming release: %v; want %s ready, riding along apart from what it carries", rows[cut], cut)
	}
}

// A WANT RECORDED DONE WAITS ON THE PERSON, NOT ON PLANNING — an increment
// holding a feature realized (built, its criteria passing) with no open work
// serving it says the feature awaits a person's confirmation: plan-work files
// nothing for it, so "no work planned" would ask for what never comes. A
// feature no work serves and not realized still says no work is planned.
func TestARealizedFeatureAnIncrementHoldsAwaitsAPersonsConfirmation(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	hold := func(f string) {
		t.Helper()
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}
	mailed := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: mailed, To: "realized", By: session, Note: "built and tested"}); err != nil {
		t.Fatal(err)
	}
	hold(mailed)
	hold(want(t, ctx, cwm, "feature", "Print the invoices"))

	waits, err := inc.Waits(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(waits.Waits, "\n")
	if !strings.Contains(got, "Mail the invoices: awaits a person's confirmation") || strings.Contains(got, "Mail the invoices: no work planned") {
		t.Errorf("the realized feature: %q; want it awaiting a person's confirmation, never 'no work planned'", got)
	}
	if !strings.Contains(got, "Print the invoices: no work planned") {
		t.Errorf("a feature no work serves: %q; want 'no work planned'", got)
	}
}

// A PERSON STEERS AN INCREMENT FROM ITS PANE —an open increment's pane sets
// its priority (requests assess, the person's) and puts it on hold saying why;
// held, it offers Resume; what those actions run moves the increment.
func TestAPersonSteersAnIncrementFromItsPane(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	pane := func() string {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "page"})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the pane must display: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		return string(raw)
	}
	raw := pane()
	for _, want := range []string{`"aria-label":"Priority"`,
		`"uri":"ktw-agents:requests/assess"`, `"priority":"${value}"`, `"by":"${person}"`,
		`"uri":"ktw-agents:increments/hold"`, `"reason":"${value.reason}"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the pane lacks %s", want)
		}
	}
	if strings.Contains(raw, `"uri":"ktw-agents:increments/resume"`) {
		t.Error("an increment not on hold offers Resume")
	}
	// What the actions run: the priority moves, the hold shows Resume.
	if _, err := req.Assess(ctx, api.AssessInput{ID: in.ID, Priority: "P0", By: person}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pane(), `"text":"P0"`) {
		t.Error("the pane does not show the priority set")
	}
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: in.ID, Reason: "waiting for the bank", By: person}); err != nil {
		t.Fatal(err)
	}
	if raw := pane(); !strings.Contains(raw, `"uri":"ktw-agents:increments/resume"`) || strings.Contains(raw, `"uri":"ktw-agents:increments/hold"`) {
		t.Error("a held increment's pane: want Resume in place of Put on hold")
	}
}

// AN INCREMENT'S SUMMARY IS PLANIFICATION'S ANSWER - what it
// holds done of the total, its requests by where they stand and what it waits
// on, as its pane counts them: a view reads it, never recomputes it.
func TestAnIncrementsSummaryIsPlanificationsAnswer(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc, plans := increments(t, ctx), plansOf(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	blocked := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: blocked.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Block(ctx, api.BlockInput{ID: blocked.ID, Note: "the gateway refuses", By: person}); err != nil {
		t.Fatal(err)
	}
	running := file(t, ctx, req, api.FileInput{Title: "Track the mails", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: running.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	s, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 1 || s.Done != 0 || s.Blocked != 1 || s.ToDo != 1 || s.Running != 0 || s.Awaiting != 0 || s.RequestsDone != 0 || s.State != api.IncrementOpen {
		t.Errorf("the summary: %+v; want 0 of 1 held, one request blocked, one to do (none running yet)", s)
	}
	if len(s.Waits) == 0 || !strings.Contains(strings.Join(s.Waits, "; "), "Mail the invoices is blocked: the gateway refuses") {
		t.Errorf("the summary's waits: %v; want the blocked request named", s.Waits)
	}
}
