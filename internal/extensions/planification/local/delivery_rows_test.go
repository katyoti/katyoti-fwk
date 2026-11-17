// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY ROWS - the sources answer rows of the
// delivery data types: a stream's lane (its mode, its releases by date with
// the next one marked, its backlog with the held increment), a release's
// level (its stepper, its rows by priority, what is planned for it), an
// increment's detail (where it is assigned, where it can move).
func TestTheDeliverySourcesAnswerTheirRows(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2026-11-15", Stream: st.Stream.ID})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Name: "Spring", Date: "2026-12-15", Stream: st.Stream.ID})
	mk := func(title string) string {
		t.Helper()
		got, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: got.Increment.ID, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		return got.Increment.ID
	}
	ships, planned, held := mk("Send invoices by mail"), mk("Print invoices"), mk("Archive invoices")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: ships}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: planned, In: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: held, Reason: "waiting on a decision", By: person}); err != nil {
		t.Fatal(err)
	}

	lanes, err := sm.Lanes(ctx, api.LanesInput{ID: st.Stream.ID})
	if err != nil || len(lanes.Lanes) != 1 {
		t.Fatalf("lanes: %+v, %v; want the stream's one lane", lanes, err)
	}
	l := lanes.Lanes[0]
	if l.ID != st.Stream.ID || l.Mode != api.ModeManual || !strings.Contains(l.ModeText, "manual") || len(l.Releases) != 2 ||
		l.Releases[0].ID != next.ID || !l.Releases[0].Next || l.Releases[1].Next || l.Releases[1].Label != "1.2.0 — Spring" || l.Releases[1].Planned != 1 {
		t.Fatalf("the lane: %+v; want manual, its two releases by date, 1.1.0 next, 1.2.0 named once with its name and one planned for it", l)
	}
	if l.SchemeMode != "semver · manual" || l.NextText != "next: by hand" || l.Releases[1].Summary != "2026-12-15 · 0/0 done" || l.Releases[1].PlannedText != "+1 planned" {
		t.Fatalf("the lane's display lines: scheme_mode %q, next_text %q, the later card's summary %q", l.SchemeMode, l.NextText, l.Releases[1].Summary)
	}
	if len(l.Backlog) != 2 || !backlogHas(l.Backlog, held, true) || !backlogHas(l.Backlog, planned, false) {
		t.Fatalf("the backlog: %+v; want the planned increment and the held one, held shown", l.Backlog)
	}
	if g := l.Releases[0].Gates; len(g) == 0 || g[0].Passed == nil {
		t.Fatalf("the next release's gates: %+v; want its gates, the increments one judged", g)
	}
	for _, d := range l.Releases[0].Gates {
		if (d.Name == api.GateSignOff || d.Name == api.GateChangeLog) && d.Passed != nil {
			t.Errorf("the sign-off of a release with no candidate: %+v; want it not judged yet", d)
		}
	}

	rows, err := rel.Rows(ctx, api.GetInput{ID: next.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rows.Release.ID != next.ID || len(rows.Ships) != 1 || rows.Ships[0].ID != ships || rows.Ships[0].Priority != api.DefaultPriority ||
		len(rows.Ships[0].Progress) != 5 || len(rows.Planned) != 0 || rows.ShipPlanned || rows.NextAction == "" {
		t.Fatalf("the next release's rows: %+v; want its one increment by priority with its 5-segment bar, nothing planned, a next action", rows)
	}
	if rows.Eyebrow != "Product · "+next.ID+" · the stream's next release" || rows.Title != "Product 1.1.0" || !strings.HasPrefix(rows.Due, "due 2026-11-15 · ") {
		t.Fatalf("the release header: eyebrow %q, title %q, due %q", rows.Eyebrow, rows.Title, rows.Due)
	}
	if len(rows.Stepper) != 4 || rows.Stepper[0].Name != api.ReleasePlanned || rows.Stepper[0].State != "current" || rows.Stepper[1].State != "todo" {
		t.Fatalf("the stepper: %+v; want planned current, the rest to do", rows.Stepper)
	}
	if opts := rows.Ships[0].Options; len(opts) != 1 || opts[0].Value != later.ID || opts[0].Label != "1.2.0 — Spring — planned for" {
		t.Fatalf("the row's Change release choices: %+v; want the later release, as planned for (not its own release, not the backlog)", opts)
	}
	lr, err := rel.Rows(ctx, api.GetInput{ID: later.ID})
	if err != nil || len(lr.Planned) != 1 || lr.Planned[0].ID != planned || lr.Planned[0].PlannedFor != later.ID || lr.Planned[0].PlannedChip != "planned" || lr.ShipPlanned {
		t.Fatalf("the later release's rows: %+v, %v; want the increment planned for it, not offered to ship (it is not next)", lr, err)
	}

	d, err := inc.Detail(ctx, api.IncrementDetailInput{ID: planned})
	if err != nil {
		t.Fatal(err)
	}
	if d.Row.ID != planned || !strings.HasPrefix(d.Assigned, "Planned for 1.2.0 — Spring") || !strings.Contains(d.Assigned, "ships once 1.1.0 is out") ||
		!strings.HasPrefix(d.Proposal, "not run yet") {
		t.Fatalf("the detail: %+v; want planned for 1.2.0, shipping once 1.1.0 is out, not run yet", d)
	}
	modes := map[string]string{}
	for _, o := range d.Options {
		modes[o.ID] = o.Mode
	}
	if _, own := modes[later.ID]; modes[next.ID] != "ships-in" || own || modes[""] != "backlog" {
		t.Fatalf("Change release offers %+v; want 1.1.0 ships in and the backlog - never the release it is planned for", d.Options)
	}
	if len(d.Options) != 2 {
		t.Fatalf("the planned increment's Change release: %+v; want the next release and the backlog, not the release it is planned for", d.Options)
	}
	if hd, err := inc.Detail(ctx, api.IncrementDetailInput{ID: held}); err != nil || hd.Row.Held != "waiting on a decision" || hd.Row.Standing != api.StandingOnHold || hd.Row.HeldText != api.StandingOnHold || len(hd.Options) != 0 {
		t.Fatalf("the held increment's detail: %+v, %v; want on hold with its reason, no release to move to", hd, err)
	}
}

func backlogHas(items []api.BacklogItem, id string, held bool) bool {
	for _, it := range items {
		if it.ID == id {
			return it.Held == held
		}
	}
	return false
}

// A RELEASE A CUT MADE SITS AT ITS CUT - the cut gives
// it no planned date (the stream's next release and its days left stay those
// of its planned releases), but its card sits on the timeline at the stream's
// last cut, its summary leading with that day; its header names no due date.
func TestACutReleaseSitsAtItsCut(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Docs", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	done, err := inc.Create(ctx, api.CreateIncrementInput{Stream: st.Stream.ID, Title: "A how-to page", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: done.Increment.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	cut, err := sm.Cut(ctx, api.CutInput{ID: st.Stream.ID, By: person})
	if err != nil || cut.Release == nil {
		t.Fatalf("the cut: %+v, %v", cut, err)
	}
	if cut.Release.Date != "" {
		t.Errorf("the cut release is dated %q; want no planned date", cut.Release.Date)
	}
	day := time.Now().UTC().Format("2006-01-02")
	lanes, err := sm.Lanes(ctx, api.LanesInput{ID: st.Stream.ID})
	if err != nil || len(lanes.Lanes) != 1 || len(lanes.Lanes[0].Releases) != 1 {
		t.Fatalf("the lane: %+v, %v", lanes, err)
	}
	if c := lanes.Lanes[0].Releases[0]; c.Date != day || !strings.HasPrefix(c.Summary, day+" · ") || c.DaysLeft != nil {
		t.Errorf("the cut release's card: date %q, summary %q, days left %v; want it placed at its cut %s, no days left", c.Date, c.Summary, c.DaysLeft, day)
	}
	rows, err := rel.Rows(ctx, api.GetInput{ID: cut.Release.ID})
	if err != nil || rows.Due != "" || !rows.CanUnfreeze || !rows.CanDrop {
		t.Errorf("the cut release's header: due %q, unfreeze %v, drop %v, %v; want no due date, unfreeze and drop allowed (frozen, no candidate)", rows.Due, rows.CanUnfreeze, rows.CanDrop, err)
	}
}
