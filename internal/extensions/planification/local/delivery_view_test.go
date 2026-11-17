// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE DELIVERY VIEW (F-066) - a global panel that is also
// a stream's own pane: globally every stream is a lane - its mode and next cut
// with Cut now and Change mode, its releases as cards (id, stage, done of
// total, a dot per gate, what is planned for it), its backlog with its held
// increments (Resume) and the others (Ship in its next release), each card
// showing its id and opening it; called with a stream's id it is that lane
// alone.
func TestTheDeliveryViewShowsEveryStreamAsALane(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "object-panels", "delivery.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`source: "ktw-agents:streams/delivery-page"`, "global: true", `- "stream"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the Delivery panel lacks %s", want)
		}
	}

	ctx, _, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	product, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d"}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Docs", StreamFields: api.StreamFields{VersionScheme: api.SchemeDate, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	p, d := product.Stream.ID, docs.Stream.ID
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: p})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: p})
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
	shipped, free, held, aimed := open("Invoices by mail", p), open("Reminders", p), open("Dunning", p), open("Later work", p)
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: shipped, In: next.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: held, Reason: "the bank is late", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: aimed, In: later.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	guide := open("A user guide", d)

	def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{Target: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatalf("the Delivery view must display: %v", err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	all := string(raw)
	// The mock-up's screen: the lanes on a timeline, the selected release below,
	// the selected increment beside - sections loading with the page's selection.
	for _, want := range []string{
		`"kind":"split"`, `"kind":"timeline-lanes"`, `"schema":"ktw-agents:/data-type/stream-lane"`,
		`"method":"ktw-agents:releases/rows-page"`, `"id":"${selection.release}"`,
		`"method":"ktw-agents:increments/panel-page"`, `"id":"${selection.increment}"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the Delivery view lacks %s", want)
		}
	}
	if strings.Contains(all, `"kind":"accordion"`) {
		t.Error("the Delivery view still stacks accordion forms")
	}
	if sel, _ := def.Data.(map[string]any)["selection"].(map[string]any); sel["release"] != next.ID || sel["increment"] != shipped {
		t.Errorf("the screen opens on %v; want the first stream's next release %s and its first increment %s", sel, next.ID, shipped)
	}
	// Its lanes' rows: a lane per stream, its mode and next cut, its releases
	// with their ids, stage, what is planned for them and a dot per gate, its
	// backlog - held included, the shipped one not.
	for _, want := range []string{
		`"id":"` + p + `"`, `"id":"` + d + `"`, "periodic every 14d - next cut", "manual - a person cuts it",
		`"id":"` + next.ID + `"`, `"id":"` + later.ID + `"`, `"stage":"planned"`, `"planned_text":"+1 planned"`, `"name":"increments"`,
		`"id":"` + free + `"`, `"id":"` + held + `"`, `"held":true`, `"id":"` + guide + `"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the Delivery view's lanes lack %s", want)
		}
	}
	// A lane's actions are its data type's: Change mode and Cut now (no New increment: increments come from studies).
	lane, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "data-types", "stream-lane.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ktw-agents:streams/cut", "ktw-agents:streams/set", "content: object-id"} {
		if !strings.Contains(string(lane), want) {
			t.Errorf("a lane's display lacks %s", want)
		}
	}
	// Every action on an increment of the backlog is in its pane: Resume the
	// held one, its release select - the next ships it in, a later one plans
	// it -, the planned one saying what it is planned for.
	drawer := func(id string) string {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: id, Tab: "page"})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the pane of %s must display: %v", id, err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
	if h := drawer(held); !strings.Contains(h, "ktw-agents:increments/resume") || !strings.Contains(h, `"text":"on-hold`) {
		t.Errorf("the held increment's pane offers no Resume, or hides its hold: %s", h)
	}
	if f := drawer(free); !strings.Contains(f, `"label":"1.1.0 — ships in"`) || !strings.Contains(f, `"label":"1.2.0 — planned for"`) || !strings.Contains(f, "ktw-agents:increments/plan-for") {
		t.Errorf("the free increment's release select: %s; want 1.1.0 ships in, 1.2.0 planned for", f)
	}
	if a := drawer(aimed); !strings.Contains(a, "planned for 1.2.0") || !strings.Contains(a, `"value":"`+later.ID+`"`) {
		t.Errorf("the planned increment's pane: %s; want planned for 1.2.0, its select showing it", a)
	}
	// Its New increment creates one in the stream: in its module, aimed at it.
	made, err := inc.Create(ctx, api.CreateIncrementInput{Stream: d, Title: "A how-to page", Goals: []string{"I find how to start in one page"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if made.Increment.Stream != d || made.Increment.Module != "billing" || len(made.Increment.Delivery) != 1 {
		t.Errorf("created from a lane: %+v; want it aimed at %s, in its module, with its goal", made.Increment, d)
	}
	lanes, err := sm.Lanes(ctx, api.LanesInput{ID: p})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range lanes.Lanes[0].Backlog {
		if it.ID == shipped {
			t.Errorf("the backlog lists %s, which ships in %s", shipped, next.ID)
		}
	}

	// A stream's own pane: its lane alone, its next release selected.
	def, err = sm.DeliveryPage(ctx, api.DeliveryPageInput{ID: d})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(def.DSLTree)
	if one := string(raw); !strings.Contains(one, `"id":"`+d+`"`) || strings.Contains(one, `"id":"`+p+`"`) {
		t.Errorf("the Docs stream's pane: %s; want its lane alone", one)
	}
	if def, err = sm.DeliveryPage(ctx, api.DeliveryPageInput{ID: p}); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(def.DSLTree)
	if one := string(raw); !strings.Contains(one, `"id":"`+later.ID+`"`) || !strings.Contains(one, `"id":"`+held+`"`) || !strings.Contains(one, `"kind":"timeline-lanes"`) {
		t.Errorf("the Product stream's pane: %s; want its whole lane, its releases and its backlog, held included", one)
	}
	if sel, _ := def.Data.(map[string]any)["selection"].(map[string]any); sel["release"] != next.ID {
		t.Errorf("the Product stream's pane opens on %v; want its next release %s", sel, next.ID)
	}
}
