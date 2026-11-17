// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// To do and Planification's Needs you show the same waiting work, one source
// read by both; what is on hold shows only under To do's On hold.
func TestNeedsYouIsToDo(t *testing.T) {
	refused := "its to-dos are done, but shop.F-023 cannot become done: cwm: transition refused: feature has no transition from in-review to done"
	demand := func(id string, deliverables []api.Deliverable) api.Demand {
		var d api.Demand
		d.ID, d.Module, d.Title, d.State, d.Why, d.Deliverables = id, strings.Split(id, ".")[0], id, api.PlanBlocked, refused, deliverables
		return d
	}
	d := demand("shop.F-023", []api.Deliverable{{Title: "Its test", Check: "go test ./x", Verified: true}})

	// An end refused by what it is about, every deliverable verified: To do
	// lists it under Unblock.
	items := requestItems(context.Background(), nil, nil, d, nil, false)
	if len(items) != 1 || items[0].Action != api.ActionUnblock || !items[0].Resume || !strings.Contains(items[0].Asks, "no transition from in-review to done") {
		t.Fatalf("an end refused by its lifecycle: items %+v; want one Unblock naming why", items)
	}

	// Needs you lists what To do lists - and nothing To do does not.
	blocked := func(id string) stand {
		return stand{plan: api.PlannedObject{ID: id, Title: id}, state: stateLabels[api.PlanBlocked], why: refused}
	}
	stands := []stand{blocked("shop.F-023"), blocked("folio.F-005")}
	demands := map[string]api.Demand{"shop.F-023": d, "folio.F-005": demand("folio.F-005", nil)}
	ids := func(cards []node) string {
		raw, _ := json.Marshal(cards)
		var out []string
		for _, id := range []string{"shop.F-023", "folio.F-005"} {
			if strings.Contains(string(raw), `"id":"`+id+`"`) {
				out = append(out, id)
			}
		}
		return strings.Join(out, ",")
	}
	if _, cards := needsTab(stands, demands, map[string]bool{"shop.F-023": true, "folio.F-005": true}); ids(cards) != "shop.F-023,folio.F-005" {
		t.Errorf("both waiting in To do: Needs you lists %s", ids(cards))
	}
	// on hold: To do keeps it under On hold only (not in
	// the waiting set) - Needs you does not list it.
	if _, cards := needsTab(stands, demands, map[string]bool{"shop.F-023": true}); ids(cards) != "shop.F-023" {
		t.Errorf("one on hold: Needs you lists %s; want only the one waiting", ids(cards))
	}
}
