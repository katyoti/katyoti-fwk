// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY VIEW'S TABS AND ITS INCREMENTS' MOVES (
// F-093): « Planned delivery », « Running », « Open increments » - the
// increments in no release - and « Delivered »; the moves its menus run: a
// priority changes it, a move in another stream is refused, On hold asks why
// and a priority resumes it, Ship in takes it out of Open increments, a loose
// one gets its stream.
func TestTheDeliveryViewsTabsAndItsIncrementsMoves(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	product, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	p := product.Stream.ID
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: p})
	open := func(title string, goals ...string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: goals, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: p}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	shipped, free := open("Invoices by mail", "I send invoices by mail", "My customers pay on time"), open("Reminders")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: shipped, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	loose, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "A loose idea", By: person})
	if err != nil {
		t.Fatal(err)
	}
	// The Open increments table lists the increments
	// in no release: held ones in its held filter.
	table := func(held string) string {
		t.Helper()
		def, err := inc.OpenPage(ctx, api.OpenIncrementsInput{Stream: "all", Held: held, Shown: 25})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	listed := func(rows, id string) bool { return strings.Contains(rows, `"data-increment":"`+id+`"`) }
	def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{Target: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the Delivery view must display: %v", err)
	}
	var tree struct {
		Tabs []struct {
			Label string `json:"label"`
			Badge int    `json:"badge"`
		} `json:"tabs"`
	}
	raw, _ := json.Marshal(def.DSLTree)
	_ = json.Unmarshal(raw, &tree)
	tabs := tree.Tabs
	// Running sits between them: none runs here
	// Delivered last.
	if len(tabs) != 4 || tabs[0].Label != "Planned delivery" || tabs[0].Badge != 1 || tabs[1].Label != "Running" || tabs[1].Badge != 0 ||
		tabs[2].Label != "Open increments" || tabs[2].Badge != 2 || tabs[3].Label != "Delivered" {
		t.Fatalf("the tabs: %v; want Planned delivery (1), Running (0), Open increments (2) and Delivered", tabs)
	}
	if rows := table("not-held"); listed(rows, shipped) || !listed(rows, free) || !listed(rows, loose.Increment.ID) {
		t.Errorf("Open increments lists the increments in no release, not the one shipping: %s", rows)
	}

	// A drop in another priority changes it; in another stream's lane it is refused.
	moved, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: p, Column: "P0", By: person})
	if err != nil || !moved.OK || moved.Undo == nil {
		t.Fatalf("a drop on P0: %+v %v", moved, err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: free}); got.Increment.Priority != "P0" {
		t.Errorf("after the drop its priority is %q; want P0", got.Increment.Priority)
	}
	if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: "none", Column: "P1", By: person}); err != nil || r.OK {
		t.Errorf("a drop in another stream's lane: %+v %v; want refused", r, err)
	}

	// On hold asks why and holds it with that reason, shown on its card.
	if r, _ := inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: p, Column: "held", By: person}); r.OK {
		t.Error("held without a reason")
	}
	if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: p, Column: "held", Reason: "waits for the bank", By: person}); err != nil || !r.OK {
		t.Fatalf("held: %+v %v", r, err)
	}
	if rows := table("held"); !listed(rows, free) || !strings.Contains(rows, `"data-held":"`+free+`"`) {
		t.Errorf("the held increment is not in the held filter with its held tag: %s", rows)
	}
	// Dragged to a priority it resumes there.
	if r, err := inc.BoardMove(ctx, api.BoardMoveInput{ID: free, Row: p, Column: "P1", By: person}); err != nil || !r.OK || !strings.Contains(r.Message, "resumed at P1") {
		t.Fatalf("resumed: %+v %v", r, err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: free}); got.Increment.Held != nil || got.Increment.Priority != "P1" {
		t.Errorf("after resuming: held %v priority %q; want resumed at P1", got.Increment.Held, got.Increment.Priority)
	}

	// Ship in ships it and it leaves the board; a loose one gets its stream.
	if r, err := inc.BoardShip(ctx, api.BoardShipInput{ID: free, In: next.ID, By: person}); err != nil || !r.OK || r.Undo == nil {
		t.Fatalf("ship in: %+v %v", r, err)
	}
	if listed(table("not-held"), free) {
		t.Error("a shipped increment stays in Open increments")
	}
	if r, err := inc.SetStream(ctx, api.IncrementStreamInput{ID: loose.Increment.ID, Stream: p, By: person}); err != nil || !r.OK {
		t.Fatalf("set stream: %+v %v", r, err)
	}
	if got, _ := inc.Get(ctx, api.GetInput{ID: loose.Increment.ID}); got.Increment.Stream != p {
		t.Errorf("its stream: %q; want %s", got.Increment.Stream, p)
	}
}
