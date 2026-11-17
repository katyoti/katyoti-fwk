// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ONE BACKLOG OF INCREMENTS IN NO RELEASE (feature): every
// open increment that ships in no release, across the streams, by priority -
// a done, dropped or shipped one is not in it - each with the release its
// stream would take it in; shipping one with no release named puts it in its
// stream's next planned release, and it leaves the backlog. Each stream's
// lane of the Delivery view lists its own, by the one increment card with its
// id, each offering that action naming the release (: the
// backlog panel's checks, replayed on the lanes that replace it).
func TestTheBacklogListsEveryIncrementInNoReleaseAndShipsOneInItsNextRelease(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	stream := func(name string) string {
		t.Helper()
		st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return st.Stream.ID
	}
	product, site := stream("Product"), stream("Site")
	open := func(title, stream, priority string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: stream}); err != nil {
			t.Fatal(err)
		}
		if priority != "" {
			if _, err := req.Assess(ctx, api.AssessInput{ID: out.Increment.ID, Priority: priority, By: person}); err != nil {
				t.Fatal(err)
			}
		}
		return out.Increment.ID
	}
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Date: "2027-02-01", Stream: product})
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: product})
	later := open("Reminders", product, "P2")
	urgent := open("Invoices by mail", product, "P0")
	elsewhere := open("A landing page", site, "P1")
	shipped := open("Already shipped", product, "")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: shipped, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	ended := open("Dropped", product, "")
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: ended, By: person}); err != nil {
		t.Fatal(err)
	}

	b, err := inc.Backlog(ctx, api.IncrementBacklogInput{})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, it := range b.Items {
		order = append(order, it.Increment.ID)
	}
	if want := []string{urgent, elsewhere, later}; strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("backlog %v; want %v (by priority, across streams, nothing shipped or ended)", order, want)
	}
	if it := b.Items[0]; it.Next != next.ID || !strings.Contains(it.NextName, "1.1.0") || it.StreamName != "Product" {
		t.Errorf("the first item: %+v; want its stream Product and its next release 1.1.0", it)
	}
	if it := b.Items[1]; it.Next != "" {
		t.Errorf("an increment whose stream has no planned release names one: %+v", it)
	}

	page, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.DSLTree)
	for _, want := range []string{`"id":"` + urgent + `"`, `"id":"` + next.ID + `"`, "Invoices by mail"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the lanes lack %q", want)
		}
	}
	// Shipping one in its stream's next release is its pane's release select.
	dp, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: urgent, Tab: "page"})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := json.Marshal(dp.DSLTree); !strings.Contains(string(d), `"label":"1.1.0 — ships in"`) || !strings.Contains(string(d), "ktw-agents:increments/plan-for") {
		t.Errorf("the backlog increment's pane offers no ship in 1.1.0: %s", d)
	}

	// : two tabs - Unaffected (no release) and Affected
	// (in a release, with it and where it stands) - each increment named by
	// its id, every card the one increment card.
	if len(b.Affected) != 1 || b.Affected[0].Increment.ID != shipped || !strings.Contains(b.Affected[0].ReleaseName, "1.1.0") || b.Affected[0].Summary == nil {
		t.Errorf("affected: %+v; want %s with its release 1.1.0 and its summary", b.Affected, shipped)
	}
	for _, it := range b.Affected {
		if it.Increment.ID == ended {
			t.Error("a dropped increment is affected")
		}
	}
	// The lanes: every increment in no release by its id, in its lane's
	// backlog; the shipped one is under its release, not in a backlog.
	lanes, err := sm.Lanes(ctx, api.LanesInput{})
	if err != nil {
		t.Fatal(err)
	}
	inBacklog := map[string]bool{}
	for _, l := range lanes.Lanes {
		for _, it := range l.Backlog {
			inBacklog[it.ID] = true
		}
	}
	if len(inBacklog) != 3 || !inBacklog[urgent] || !inBacklog[elsewhere] || !inBacklog[later] || inBacklog[shipped] {
		t.Errorf("the lanes' backlogs hold %v; want %s, %s and %s - not %s", inBacklog, urgent, elsewhere, later, shipped)
	}
	if err := page.Validate(); err != nil {
		t.Errorf("the Delivery view is not a valid page: %v", err)
	}

	out, err := inc.ShipIn(ctx, api.ShipInInput{ID: urgent})
	if err != nil {
		t.Fatalf("ship in its next release, none named: %v", err)
	}
	if out.Increment.ShipsIn != next.ID {
		t.Errorf("shipped in %q, want its stream's next planned release %s", out.Increment.ShipsIn, next.ID)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: elsewhere}); err == nil {
		t.Error("an increment whose stream has no planned release was shipped somewhere")
	}
	b, _ = inc.Backlog(ctx, api.IncrementBacklogInput{})
	for _, it := range b.Items {
		if it.Increment.ID == urgent {
			t.Error("a shipped increment is still in the backlog")
		}
	}
}
