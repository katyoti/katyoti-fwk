// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT PLANNED FOR A LATER RELEASE - planned for
// a later release of its stream it ships in none: the intention is recorded
// and the release's pane lists it apart; planned for the next release it ships
// in it; a release of another stream, or one no longer planned, is refused.
// Once the earlier release is frozen, the later one is the next and one action
// ships every increment planned for it - a held one stays planned, saying why.
// A dropped release returns what was planned for it to the backlog, each
// saying which release was dropped.
func TestAnIncrementIsPlannedForALaterRelease(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	stream := func(name string) string {
		t.Helper()
		st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return st.Stream.ID
	}
	product, docs := stream("Product"), stream("Docs")
	open := func(title, st string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: st}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: product})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: product})
	other := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "0.9.0", Date: "2027-01-15", Stream: docs})

	mail, remind, held := open("Invoices by mail", product), open("Reminders", product), open("Dunning", product)
	got := func(id string) api.Increment {
		t.Helper()
		g, err := inc.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return g.Increment
	}

	// Planned for the later release: no release takes it yet.
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: mail, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{mail, remind, held} {
		if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: id, In: later.ID, By: person}); err != nil {
			t.Fatalf("planning %s for the later release: %v", id, err)
		}
	}
	if g := got(mail); g.PlannedFor != later.ID || g.ShipsIn != "" {
		t.Errorf("planned for %s: ships in %q, planned for %q; want it out of %s and planned for the later release", later.ID, g.ShipsIn, g.PlannedFor, next.ID)
	}
	if s, _ := rel.Scope(ctx, api.GetInput{ID: later.ID}); s.Total != 0 {
		t.Errorf("the later release's scope holds %d target(s) of what is only planned for it", s.Total)
	}
	def, err := releasePane(ctx, rel, later.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	if !strings.Contains(string(raw), `"text":"Planned for this release"`) || !strings.Contains(string(raw), mail) {
		t.Errorf("the later release's pane does not list %s apart as planned for it: %s", mail, raw)
	}
	if strings.Contains(string(raw), "ktw-agents:releases/ship-planned") {
		t.Error("the later release offers to ship what is planned for it before it is the next")
	}
	ipage, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: mail, Tab: "page"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(ipage.DSLTree); !strings.Contains(string(raw), "planned for 1.2.0") {
		t.Errorf("the increment's pane does not say what it is planned for: %s", raw)
	}

	// Refused: another stream's release; the next release ships it instead.
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: remind, In: other.ID, By: person}); err == nil {
		t.Error("an increment was planned for a release of another stream")
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: remind, In: next.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if g := got(remind); g.ShipsIn != next.ID || g.PlannedFor != "" {
		t.Errorf("planned for the next release: ships in %q, planned for %q; want it shipped in %s", g.ShipsIn, g.PlannedFor, next.ID)
	}
	if _, err := rel.ShipPlanned(ctx, api.MoveInput{ID: later.ID, By: person}); err == nil {
		t.Error("what is planned for a release was shipped before it is its stream's next")
	}

	// The earlier release frozen, the later one is the next: one action ships
	// what is planned for it; the held one stays.
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: held, Reason: "the bank is late", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: next.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if def, err = releasePane(ctx, rel, later.ID); err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(def.DSLTree); !strings.Contains(string(raw), "ktw-agents:releases/ship-planned") {
		t.Error("the next release offers no action shipping what is planned for it")
	}
	out, err := rel.ShipPlanned(ctx, api.MoveInput{ID: later.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Shipped, []string{mail}) || len(out.Stayed) != 1 || !strings.Contains(out.Stayed[0], held) {
		t.Errorf("ship planned: shipped %v, stayed %v; want %s shipped and %s staying, on hold", out.Shipped, out.Stayed, mail, held)
	}
	if g := got(mail); g.ShipsIn != later.ID || g.PlannedFor != "" {
		t.Errorf("shipped: ships in %q, planned for %q; want it in %s, no longer only planned", g.ShipsIn, g.PlannedFor, later.ID)
	}

	// A dropped release returns what is planned for it to the backlog.
	far := excepted(t, ctx, cwm, rel, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2027-03-01", Stream: product})
	if _, err := inc.Resume(ctx, api.IncrementHoldInput{ID: held, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: held, In: far.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Drop(ctx, api.MoveInput{ID: far.ID, By: person, Note: "merged into 1.2.0"}); err != nil {
		t.Fatal(err)
	}
	if g := got(held); g.PlannedFor != "" || g.ShipsIn != "" || !strings.Contains(g.Returned, "1.3.0") {
		t.Errorf("its release dropped: ships in %q, planned for %q, returned %q; want it back in the backlog saying 1.3.0 was dropped", g.ShipsIn, g.PlannedFor, g.Returned)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: held, In: far.ID, By: person}); err == nil {
		t.Error("an increment was planned for a dropped release")
	}
	if _, err := inc.Unplan(ctx, api.GetInput{ID: held}); err != nil {
		t.Errorf("unplanning an increment planned for nothing: %v", err)
	}
}
