// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// deliveryWorld is a stream with its next and a later release, an increment
// shipped in the next one holding a feature, one planned for the later one,
// one held in the backlog.
type deliveryWorld struct {
	stream, next, later, shipped, planned, held, feature string
}

func newDeliveryWorld(t *testing.T) (deliveryWorld, context.Context, api.StreamService, api.ReleaseService, api.IncrementService) {
	t.Helper()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	st, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: "Product", StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModePeriodic, Every: "14d"}})
	if err != nil {
		t.Fatal(err)
	}
	w := deliveryWorld{stream: st.Stream.ID}
	w.next = plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: w.stream}).ID
	w.later = plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Name: "Spring", Date: "2027-02-01", Stream: w.stream}).ID
	mk := func(title string) string {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Stream: w.stream, Title: title, Goals: []string{"I " + strings.ToLower(title)}, By: person})
		if err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID
	}
	w.shipped, w.planned, w.held = mk("Send invoices by mail"), mk("Print invoices"), mk("Archive invoices")
	w.feature = want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, w.feature)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: w.feature, Increment: w.shipped}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: w.shipped, In: w.next}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: w.planned, In: w.later, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Hold(ctx, api.IncrementHoldInput{ID: w.held, Reason: "waiting for the sandbox", By: person}); err != nil {
		t.Fatal(err)
	}
	return w, ctx, sm, rel, inc
}

// screenJSON is a page's tree as JSON, the page validated first:
// screenJSON(t, name)(page()).
func screenJSON(t *testing.T, name string) func(render.Def, error) string {
	t.Helper()
	return func(def render.Def, err error) string {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("%s must display: %v", name, err)
		}
		b, _ := json.Marshal(def.DSLTree)
		return string(b)
	}
}

// THE DELIVERY VIEW IS THE MOCK-UP'S SCREEN - one split
// page: the streams' lanes on a timeline, the selected release's level below
// (a region reading the page's selection), the selected increment's drawer in
// the side panel; no card stack, no accordion; it opens on the next release.
func TestTheDeliveryViewIsTheMockUpsScreen(t *testing.T) {
	t.Parallel()
	w, ctx, sm, _, _ := newDeliveryWorld(t)
	def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{})
	all := screenJSON(t, "the Delivery view")(def, err)
	var tree map[string]any
	_ = json.Unmarshal([]byte(all), &tree)
	// The screen is the « Planned delivery » tab.
	if tree["kind"] != "tabs" {
		t.Fatalf("the Delivery view: %v; want its two tabs", tree["kind"])
	}
	tree = tree["tabs"].([]any)[0].(map[string]any)["content"].(map[string]any)
	kinds := []string{}
	var split map[string]any
	for _, c := range tree["children"].([]any) {
		n := c.(map[string]any)
		kinds = append(kinds, n["kind"].(string))
		if n["kind"] == "split" {
			split = n
		}
	}
	if tree["kind"] != "vbox" || strings.Join(kinds, ",") != "text,timeline-lanes,split" {
		t.Fatalf("the Delivery view: %v %v; want the intro, the lanes full width, then the release and the increment side by side", tree["kind"], kinds)
	}
	main, _ := split["main"].(map[string]any)
	side, _ := split["side"].(map[string]any)
	if split["sideWidth"] != float64(380) || loadMethod(main) != "ktw-agents:releases/rows-page" || loadMethod(side) != "ktw-agents:increments/panel-page" {
		t.Errorf("the split below the timeline: %v; want the release section left, the picked increment's short panel right (ops.RQ-267), 380 wide", split)
	}
	if n := strings.Count(all, "Colours are the Workflows view's"); n != 1 {
		t.Errorf("the intro paragraph shows %d times; want once, the mock-up's", n)
	}
	for _, want := range []string{`"method":"ktw-agents:releases/rows-page"`, `"id":"${selection.release}"`, `"id":"${selection.increment}"`,
		`"schema":"ktw-agents:/data-type/stream-lane"`, `"id":"` + w.next + `"`, `"id":"` + w.held + `"`} {
		if !strings.Contains(all, want) {
			t.Errorf("the Delivery view lacks %s", want)
		}
	}
	for _, not := range []string{`"kind":"accordion"`, `"kind":"card"`} {
		if strings.Contains(all, not) {
			t.Errorf("the Delivery view still holds %s", not)
		}
	}
	sel, _ := def.Data.(map[string]any)["selection"].(map[string]any)
	if sel["release"] != w.next || sel["increment"] != w.shipped {
		t.Errorf("the screen opens on %v; want %s and its increment %s", sel, w.next, w.shipped)
	}
}

// THE RELEASE LEVEL AND THE DRAWER ARE THE MOCK-UP'S -
// the release level: its header (stepper, gates) beside its primary next
// action, the legend, Ships in this release (n) and Planned for this release
// (n) as compact increment rows; the increment's pane: where it is with its
// release select, Hold, priority, its requests bar, its delivery goals, its
// proposal in its own tab.
func TestTheReleaseLevelAndTheDrawerAreTheMockUps(t *testing.T) {
	t.Parallel()
	w, ctx, _, rel, inc := newDeliveryWorld(t)
	level := screenJSON(t, "the release level")(rel.RowsPage(ctx, api.ReleaseRowsPageInput{ID: w.next}))
	for _, want := range []string{
		`"schema":"ktw-agents:/data-type/release-rows","variant":"header"`, `"next_step":"freeze"`, `"next_label":"Finish 1 increment, then Freeze"`,
		`"kind":"legend"`, `"variant":"label"`, "Ships in this release (1)", `"schema":"ktw-agents:/data-type/increment-row"`, `"id":"` + w.shipped + `"`,
	} {
		if !strings.Contains(level, want) {
			t.Errorf("the release level lacks %s", want)
		}
	}
	// The header's other actions follow the stage: a planned release may be
	// rescheduled and dropped, not unfrozen.
	if rows, err := rel.Rows(ctx, api.GetInput{ID: w.next}); err != nil || !rows.CanReschedule || !rows.CanDrop || rows.CanUnfreeze {
		t.Errorf("a planned release's header: %+v, %v; want reschedule and drop, no unfreeze", rows, err)
	}
	later := screenJSON(t, "the later release's level")(rel.RowsPage(ctx, api.ReleaseRowsPageInput{ID: w.later}))
	if !strings.Contains(later, "Planned for this release (1)") || !strings.Contains(later, `"id":"`+w.planned+`"`) || strings.Contains(later, "Ship the 1 planned") {
		t.Errorf("the later release's level: %s; want its planned increment, not offered to ship (it is not next)", later)
	}

	pane := func(name, id, tab string) string {
		t.Helper()
		return screenJSON(t, name)(inc.PanePage(ctx, api.IncrementPaneInput{ID: id, Tab: tab}))
	}
	drawer := pane("the increment's Increment tab", w.shipped, "page")
	for _, want := range []string{
		`"content":"` + w.shipped + `"`, `"data-view":"increment-page-head"`, "ships in 1.1.0",
		`"label":"Take out"`, `"label":"Put on hold"`, `"aria-label":"Priority"`, "ktw-agents:requests/assess",
		`"data-view":"increment-actions"`, `"kind":"gauge"`, "Its proposal",
	} {
		if !strings.Contains(drawer, want) {
			t.Errorf("the increment's pane lacks %s", want)
		}
	}
	// Take out while its release is planned; Drop it under the More menu.
	if !strings.Contains(drawer, `"label":"Take out"`) || !strings.Contains(drawer, "ktw-agents:increments/drop") {
		t.Errorf("the pane of an increment in a planned release offers no Take out or no Drop it: %s", drawer)
	}
	planned := pane("the planned increment's pane", w.planned, "page")
	if !strings.Contains(planned, "planned for 1.2.0 — Spring") || !strings.Contains(planned, `"label":"1.1.0 — `) || !strings.Contains(planned, `"value":"`+w.later+`"`) {
		t.Errorf("the planned increment's pane: %s; want planned for 1.2.0, 1.1.0 offered, the select showing 1.2.0", planned)
	}
	// Not yet run: Run, the person's go; its Claude
	// Code session only once run (TestAnIncrementNotYetRunOffersRun).
	if !strings.Contains(planned, `"data-control":"run-increment"`) || strings.Contains(planned, `"label":"Run in Claude Code"`) {
		t.Errorf("the planned increment's pane: %s; want Run, not Run in Claude Code before it is run", planned)
	}
	if goals := pane("the planned increment's Goals tab", w.planned, "goals"); !strings.Contains(goals, "I print invoices") {
		t.Errorf("the planned increment's Goals tab: %s; want its delivery goal", goals)
	}
	// Frozen - its scope ready: what the increment holds taken out first -,
	// the release keeps what it ships.
	if _, err := inc.Untarget(ctx, api.UntargetInput{ID: w.feature}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: w.next, By: person}); err != nil {
		t.Fatalf("freeze: %v", err)
	} else {
		frozen := pane("the pane, its release frozen", w.shipped, "page")
		if strings.Contains(frozen, `"label":"Take out"`) {
			t.Error("Take out is offered while its release is frozen: one click would change what a frozen release ships")
		}
		if rows, err := rel.Rows(ctx, api.GetInput{ID: w.next}); err != nil || !rows.CanUnfreeze {
			t.Errorf("a frozen release with no candidate cannot be unfrozen from its header: %+v, %v", rows, err)
		}
	}
}

// EACH OBJECT'S PANE IS THE SAME VIEW - the release
// pane is the release level, the increment pane the drawer, the stream pane
// its lane with its next release selected.
func TestEachObjectsPaneIsTheSameView(t *testing.T) {
	t.Parallel()
	// Everything the earlier panes showed stays within reach: an Overview tab.
	for file, source := range map[string]string{"release.yaml": "ktw-agents:releases/rows-page", "increment-open-requests.yaml": "ktw-agents:increments/pane-page", "delivery.yaml": "ktw-agents:streams/delivery-page",
		"release-overview.yaml": "ktw-agents:releases/page"} {
		b, err := os.ReadFile(filepath.Join("..", "contrib", "embed", "cwm", "object-panels", file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `source: "`+source+`"`) {
			t.Errorf("the %s pane's source is not %s", file, source)
		}
	}
	w, ctx, sm, _, _ := newDeliveryWorld(t)
	def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{ID: w.stream})
	one := screenJSON(t, "the stream's pane")(def, err)
	if !strings.Contains(one, `"kind":"timeline-lanes"`) || !strings.Contains(one, `"id":"`+w.stream+`"`) {
		t.Errorf("the stream's pane: %s; want its lane on the timeline", one)
	}
	if sel, _ := def.Data.(map[string]any)["selection"].(map[string]any); sel["release"] != w.next {
		t.Errorf("the stream's pane opens on %v; want its next release %s", sel, w.next)
	}
	if def.Title != "Product" {
		t.Errorf("the stream's pane is titled %q; want the stream's name", def.Title)
	}
}

// loadMethod is the method a decoded section node loads.
func loadMethod(n map[string]any) any {
	load, _ := n["load"].(map[string]any)
	return load["method"]
}
