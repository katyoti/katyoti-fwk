// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE'S PANE STEERS EACH INCREMENT - each open
// increment's row carries, as the person: move it to another planned release
// of the stream (Ship in the next one, Plan for a later one), Take out, Hold,
// and a priority other than its own; the release carries Reschedule and Drop
// while planned, and Unfreeze while frozen with no candidate. Run with their
// row's arguments, the actions do what they say - Take out included once the
// release is frozen.
func TestAReleasesPaneSteersEachIncrement(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: st.Stream.ID})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: st.Stream.ID})
	open := func(title string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sm.AimIncrement(ctx, api.StreamInInput{ID: out.Increment.ID, Stream: st.Stream.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: out.Increment.ID, In: next.ID}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	mail, remind := open("Invoices by mail"), open("Reminders")

	page := func(id string) (map[string]any, string) {
		t.Helper()
		def, err := releasePane(ctx, rel, id)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		return tree, string(raw)
	}
	tree, raw := page(next.ID)
	rows := incrementsTree(t, tree)
	actions := map[string]map[string]any{}
	for _, a := range rows["actions"].([]any) {
		actions[a.(map[string]any)["id"].(string)] = a.(map[string]any)
	}
	for _, id := range []string{"move-" + later.ID, "take-out", "hold", "priority-P0", "priority-P3"} {
		if actions[id] == nil {
			t.Errorf("the increments' rows lack the action %s: %v", id, rows["actions"])
		}
	}
	if a := actions["move-"+later.ID]; a != nil && a["label"] != "Plan for 1.2.0" {
		t.Errorf("moving to the later release reads %v; want « Plan for 1.2.0 »", a["label"])
	}
	for _, r := range rows["data"].([]any) {
		row := r.(map[string]any)
		if row["open_increment"] != true || row["priority_not_P2"] != nil || row["priority_not_P0"] != true {
			t.Errorf("row %v: want it open, offering every priority but its own (P2)", row)
		}
	}
	for _, want := range []string{`"label":"Freeze it"`, `"label":"Reschedule it"`, `"label":"Drop it"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the planned release's pane lacks %s", want)
		}
	}

	// The row actions, run with their arguments.
	run := func(id, row string) {
		t.Helper()
		a := actions[id]
		args := map[string]any{}
		for k, v := range a["args"].(map[string]any) {
			switch v {
			case "${item.id}":
				args[k] = row
			case "${person}":
				args[k] = map[string]any{"id": "jdoe", "kind": "person"}
			default:
				args[k] = v
			}
		}
		b, _ := json.Marshal(args)
		var err error
		switch a["uri"] {
		case "ktw-agents:increments/plan-for":
			var in api.PlanForInput
			_ = json.Unmarshal(b, &in)
			_, err = inc.PlanFor(ctx, in)
		case "ktw-agents:increments/unship":
			var in api.GetInput
			_ = json.Unmarshal(b, &in)
			_, err = inc.Unship(ctx, in)
		case "ktw-agents:increments/hold":
			var in api.IncrementHoldInput
			_ = json.Unmarshal(b, &in)
			_, err = inc.Hold(ctx, in)
		default:
			t.Fatalf("no runner for %v", a["uri"])
		}
		if err != nil {
			t.Fatalf("%s on %s: %v", id, row, err)
		}
	}
	run("move-"+later.ID, mail)
	if g, _ := inc.Get(ctx, api.GetInput{ID: mail}); g.Increment.PlannedFor != later.ID || g.Increment.ShipsIn != "" {
		t.Errorf("moved to 1.2.0: ships in %q, planned for %q", g.Increment.ShipsIn, g.Increment.PlannedFor)
	}
	run("hold", remind)
	if g, _ := inc.Get(ctx, api.GetInput{ID: remind}); g.Increment.Held == nil || !strings.Contains(g.Increment.Held.Reason, "1.1.0") {
		t.Errorf("held from the release's pane: %+v; want on hold naming the release", g.Increment.Held)
	}
	// Held, it stays planned for the release: its pane shows it, with Resume.
	if g, _ := inc.Get(ctx, api.GetInput{ID: remind}); g.Increment.PlannedFor != next.ID {
		t.Errorf("held from %s: planned for %q; want it kept planned for the release it left", next.ID, g.Increment.PlannedFor)
	}
	if _, raw := page(next.ID); !strings.Contains(raw, `"text":"Planned for this release"`) || !strings.Contains(raw, `"id":"resume"`) || !strings.Contains(raw, remind) {
		t.Errorf("the release's pane does not show the held increment with Resume: %s", raw)
	}
	if _, err := inc.Resume(ctx, api.IncrementHoldInput{ID: remind, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: remind, In: next.ID}); err != nil {
		t.Fatal(err)
	}

	// Frozen: Unfreeze offered, Take out still works.
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: next.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, raw = page(next.ID); !strings.Contains(raw, `"label":"Unfreeze it"`) || !strings.Contains(raw, `"label":"Drop it"`) {
		t.Errorf("the frozen release's pane lacks Unfreeze or Drop")
	}
	run("take-out", remind)
	if g, _ := inc.Get(ctx, api.GetInput{ID: remind}); g.Increment.ShipsIn != "" {
		t.Errorf("taken out of the frozen release, it still ships in %s", g.Increment.ShipsIn)
	}
}

// incrementsTree is the tree of the section « Increments of this release ».
func incrementsTree(t *testing.T, n any) map[string]any {
	t.Helper()
	var found map[string]any
	var walk func(any, bool)
	walk = func(v any, in bool) {
		switch x := v.(type) {
		case map[string]any:
			if x["kind"] == "heading" && x["text"] == "Increments of this release" {
				in = true
			}
			if in && x["kind"] == "tree" && found == nil {
				found = x
				return
			}
			kids, _ := x["children"].([]any)
			here := in
			for _, k := range kids {
				walk(k, here)
				if km, ok := k.(map[string]any); ok && km["kind"] == "heading" && km["text"] == "Increments of this release" {
					here = true
				}
			}
		case []any:
			for _, k := range x {
				walk(k, in)
			}
		}
	}
	walk(n, false)
	if found == nil {
		t.Fatal("no tree under « Increments of this release »")
	}
	return found
}
