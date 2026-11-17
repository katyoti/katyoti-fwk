// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE OVERVIEW'S RIGHT-HAND BOX LISTS EACH STREAM'S CURRENT RELEASE
// : Team priorities' data, its
// current groups only - never a next or later release, never what was
// delivered -; a separator per stream when several show; inside a group by
// priority then rank; each row in the compact « overview » variant with
// « id · version » under its title and its progress; a click opens it.
func TestTheOverviewListsEachStreamsCurrentReleaseIncrements(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	sm := streams(t, w.ctx)
	docsCur := plan(t, w.ctx, w.rel, api.PlanInput{Module: "billing", Version: "2.0.0", Date: "2099-10-15", Stream: w.docs}).ID
	docsNext := plan(t, w.ctx, w.rel, api.PlanInput{Module: "billing", Version: "2.1.0", Date: "2099-11-15", Stream: w.docs}).ID
	open := func(title, priority, release string) string {
		t.Helper()
		out, err := w.inc.Create(w.ctx, api.CreateIncrementInput{Module: "billing", Stream: w.docs, Title: title, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(w.ctx, api.StreamInInput{ID: out.Increment.ID, Stream: w.docs}); err != nil {
			t.Fatal(err)
		}
		if r, err := w.inc.BoardMove(w.ctx, api.BoardMoveInput{ID: out.Increment.ID, Column: priority, By: person}); err != nil || !r.OK {
			t.Fatalf("priority %s: %+v %v", priority, r, err)
		}
		if r, err := w.inc.BoardShip(w.ctx, api.BoardShipInput{ID: out.Increment.ID, In: release, By: person}); err != nil || !r.OK {
			t.Fatalf("ship in %s: %+v %v", release, r, err)
		}
		return out.Increment.ID
	}
	if r, err := w.inc.BoardShip(w.ctx, api.BoardShipInput{ID: w.docsOne, In: docsCur, By: person}); err != nil || !r.OK {
		t.Fatalf("ship %s: %+v %v", w.docsOne, r, err)
	}
	docsP0 := open("A quick start", "P0", docsCur)
	docsLater := open("A reference", "P1", docsNext)

	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Deck(w.ctx, api.DeckInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := render.ValidatePage(def.DSLTree); err != nil {
		t.Fatalf("the deck is off the page grammar: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	panel := findView(tree, "deck-current-releases")
	if panel == nil {
		t.Fatalf("the Overview has no current releases' panel")
	}
	// Each group: its separator's release, then its rows' ids.
	var got []string
	for _, k := range panel["children"].([]any) {
		n := k.(map[string]any)
		if id, _ := n["id"].(string); strings.HasPrefix(id, "tp-sep:") {
			got = append(got, id)
			continue
		}
		if n["kind"] != "table" {
			t.Errorf("the panel holds a %v; want separators and tables", n["kind"])
			continue
		}
		if n["variant"] != "overview" || n["schema"] != "ktw-agents:/data-type/increment-row" {
			t.Errorf("the rows are %v of %v; want the increment-row's overview variant", n["variant"], n["schema"])
		}
		if a, _ := n["open"].(map[string]any); a == nil || asText(a["navigate"]) != `{"object":"${item.id}"}` {
			t.Errorf("a click on a row opens %v; want its increment", n["open"])
		}
		for _, r := range n["data"].([]any) {
			row := r.(map[string]any)
			id := row["id"].(string)
			got = append(got, id)
			version := map[bool]string{true: "2.0.0", false: "1.1.0"}[id == docsP0 || id == w.docsOne]
			if row["place"] != id+" · "+version {
				t.Errorf("%s reads %q under its title; want %q", id, row["place"], id+" · "+version)
			}
			if p, _ := row["progress"].([]any); len(p) != 5 {
				t.Errorf("%s has no progress: %v", id, row["progress"])
			}
		}
	}
	want := []string{"tp-sep:current:" + docsCur, docsP0, w.docsOne,
		"tp-sep:current:" + w.current, w.curP0a, w.curP0b, w.curP2}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("the panel lists:\n got %v\nwant %v", got, want)
	}
	for _, never := range []string{w.inNext, w.inLater, w.inNone, w.delivered, docsLater} {
		if strings.Contains(asText(panel), `"id":"`+never+`"`) {
			t.Errorf("the panel lists %s, not in a current release", never)
		}
	}
	if !strings.Contains(asText(panel), "Product · Current release") || !strings.Contains(asText(panel), "Docs · Current release") {
		t.Errorf("the separators do not name each stream's current release: %s", asText(panel))
	}
}

// findView is the first node of a tree whose data-view is view.
func findView(n any, view string) map[string]any {
	switch v := n.(type) {
	case map[string]any:
		if v["data-view"] == view {
			return v
		}
		for _, c := range v {
			if f := findView(c, view); f != nil {
				return f
			}
		}
	case []any:
		for _, c := range v {
			if f := findView(c, view); f != nil {
				return f
			}
		}
	}
	return nil
}

func asText(n any) string {
	b, _ := json.Marshal(n)
	return string(b)
}
