// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE INCREMENT'S PANE IS CLEAR AND FREE OF ERRORS — every tab a valid page
// (every node known, every required prop there); the tab it opens on names it
// and says where it ships.
func TestTheIncrementsPaneIsClearAndValid(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})
	empty := increment(t, ctx, "Nothing yet")
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id         string
		wants, not []string
	}{
		{in.ID, []string{"Invoices reach their customers by mail", "ships in 1.4.0"}, nil},
		{empty.ID, []string{"Nothing yet"}, []string{"ships in"}},
	} {
		for _, tab := range []string{"open-requests", "open-request-list", "goals", "page", "blockers", "blocker-list"} {
			page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: c.id, Tab: tab})
			if err != nil {
				t.Fatalf("%s's %s tab: %v", c.id, tab, err)
			}
			if err := render.ValidatePage(page.DSLTree); err != nil {
				t.Errorf("%s's %s tab is not a valid page: %v", c.id, tab, err)
			}
		}
		page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: c.id})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(page.DSLTree)
		for _, want := range c.wants {
			if !strings.Contains(string(raw), want) {
				t.Errorf("%s's pane lacks %q", c.id, want)
			}
		}
		for _, not := range c.not {
			if strings.Contains(string(raw), not) {
				t.Errorf("%s's pane says %q", c.id, not)
			}
		}
	}
}
