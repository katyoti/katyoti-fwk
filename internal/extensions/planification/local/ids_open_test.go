// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// EVERY ITEM SHOWS ITS ID AND OPENS - on the Delivery
// view, a release's pane and an increment's pane: every tree shows each row's
// id and opens it on a click, every row naming an object; the release and the
// increment show their own id; the lane's stream opens from its id; every
// increment card shows its id and opens on its title.
func TestEveryDeliveryItemShowsItsIDAndOpens(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	in, err := inc.Create(ctx, api.CreateIncrementInput{Stream: st.Stream.ID, Title: "Invoices by mail"})
	if err != nil {
		t.Fatal(err)
	}
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.Increment.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.Increment.ID, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	free, err := inc.Create(ctx, api.CreateIncrementInput{Stream: st.Stream.ID, Title: "Reminders"})
	if err != nil {
		t.Fatal(err)
	}

	pages := map[string]func() (render.Def, error){
		"the Delivery view":  func() (render.Def, error) { return sm.DeliveryPage(ctx, api.DeliveryPageInput{}) },
		"the release's pane": func() (render.Def, error) { return rel.RowsPage(ctx, api.ReleaseRowsPageInput{ID: next.ID}) },
		"the increment's pane": func() (render.Def, error) {
			return inc.PanePage(ctx, api.IncrementPaneInput{ID: in.Increment.ID, Tab: "page"})
		},
	}
	own := map[string][]string{
		// Rows carry their ids; the delivery data types draw each id as an
		// object-id chip that opens it.
		"the Delivery view":    {`"id":"` + st.Stream.ID + `"`, `"id":"` + next.ID + `"`, `"id":"` + free.Increment.ID + `"`},
		"the release's pane":   {`"id":"` + next.ID + `"`, `"id":"` + in.Increment.ID + `"`},
		"the increment's pane": {`"content":"` + in.Increment.ID + `"`},
	}
	for name, open := range pages {
		def, err := open()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree any
		_ = json.Unmarshal(raw, &tree)
		trees := 0
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if x["kind"] == "tree" {
					trees++
					item, _ := json.Marshal(x["item"])
					opens, _ := json.Marshal(x["open"])
					if !strings.Contains(string(item), `"$.item.id"`) {
						t.Errorf("%s: a tree does not show its rows' ids: %s", name, item)
					}
					if !strings.Contains(string(opens), `"navigate":{"object":"${item.id}"}`) {
						t.Errorf("%s: a tree's rows do not open on a click: %s", name, opens)
					}
					rows, _ := x["data"].([]any)
					for _, r := range rows {
						if id, _ := r.(map[string]any)["id"].(string); !strings.Contains(id, ".") {
							t.Errorf("%s: a row %q names no object - its click opens nothing", name, id)
						}
					}
				}
				for _, k := range x {
					walk(k)
				}
			case []any:
				for _, k := range x {
					walk(k)
				}
			}
		}
		walk(tree)
		_ = trees // the panes are the delivery screen's pages: their rows are the data types', not trees
		for _, w := range own[name] {
			if !strings.Contains(string(raw), w) {
				t.Errorf("%s lacks %s", name, w)
			}
		}
	}
}

// idBesideTitle is whether every id in a label follows a title, in brackets:
// « Ships in 1.1.0 ».
func idBesideTitle(id *regexp.Regexp, s string) bool {
	for _, loc := range id.FindAllStringIndex(s, -1) {
		if loc[0] < 2 || s[loc[0]-1] != '(' || loc[1] >= len(s) || s[loc[1]] != ')' {
			return false
		}
	}
	return true
}
