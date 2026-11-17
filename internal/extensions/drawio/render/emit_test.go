// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// TestEmit_NilDiagram asserts a missing diagram is a soft content error,
// not a Go error.
func TestEmit_NilDiagram(t *testing.T) {
	out, err := Emit(context.Background(), nil, true)
	if err != nil {
		t.Fatalf("want nil error (soft validation), got %v", err)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "diagram") {
		t.Errorf("want diagram-required error, got %v", out.Errors)
	}
}

func mustContain(t *testing.T, xml string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(xml, w) {
			t.Errorf("XML missing %q", w)
		}
	}
	if t.Failed() {
		t.Logf("--- XML ---\n%s", xml)
	}
}

// TestEmit_AppFlux draws the app-flux test viewpoint with the shipped templates:
// the domain colour, the XLS badge and border, the catalog link, the flow's
// interface-code links and colour, the page title, and a legend.
func TestEmit_AppFlux(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Title:     "Order intake",
		Nodes: []drawioapi.LogicalNode{
			{ID: "billing", Label: "Billing", Kind: "Application", Attributes: map[string]any{
				"domain": "reference", "markers": []any{"xls"}, "description": "Invoices and prices",
				"external_url": "https://catalog.example.com/app/billing",
			}},
			{ID: "ledger", Label: "Ledger", Kind: "Application", Attributes: map[string]any{"domain": "invoicing"}},
			{ID: "sheet", Label: "Tarifs.xlsx", Kind: "Spreadsheet"},
		},
		Edges: []drawioapi.LogicalEdge{
			{ID: "f1", Source: "billing", Target: "ledger", Kind: "Flow", Attributes: map[string]any{
				"flow_type":       "invoice",
				"interface_codes": []any{map[string]any{"code": "IF-7349", "url": "https://catalog.example.com/if/1"}},
				"data_types":      []any{"Facture"},
			}},
			{ID: "f2", Source: "sheet", Target: "billing", Kind: "Flow"},
		},
	}
	out, err := Emit(ctx, ld, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("errors: %v", out.Errors)
	}
	for _, w := range out.Warnings {
		t.Logf("warning: %s", w)
	}
	mustContain(t, out.XML,
		`ktw-viewpoint="drawio-test:/viewpoint/app-flux"`,
		`id="n-billing"`, `fillColor=#d5e8d4`, `strokeColor=#107C41`, `strokeWidth=3`, // reference domain + xls
		`label="XLS"`, `parent="n-billing"`, // the badge part
		`https://catalog.example.com/app/billing`, `Invoices and prices`,
		`fillColor=#fff2cc`,                                                  // invoicing domain
		`IF-7349`, `https://catalog.example.com/if/1`, `strokeColor=#CC6600`, // flow link and colour
		`Order intake`, // page title
		`label="Legend"`, `label="Application"`, `label="Spreadsheet"`, `label="Flow"`,
	)
	// Ledger has no xls marker: no badge, its stroke is the palette's.
	if strings.Contains(out.XML, `id="n-ledger-tpl-Application-xls"`) {
		t.Error("the badge must not be drawn without the xls marker")
	}
}

// TestEmit_ArchimateUnknownKindAndFallback: an unknown kind is an error,
// and the render still draws it with the library's default template.
func TestEmit_ArchimateUnknownKindAndFallback(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/archimate3-layered",
		Nodes: []drawioapi.LogicalNode{
			{ID: "c", Label: "Customer", Kind: "BusinessActor"},
			{ID: "p", Label: "Place order", Kind: "BusinessProcess"},
			{ID: "x", Label: "Mystery", Kind: "Unicorn"},
		},
		Edges: []drawioapi.LogicalEdge{{ID: "e", Source: "c", Target: "p", Kind: "triggering"}},
	}
	out, err := Emit(ctx, ld, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "Unicorn") {
		t.Fatalf("want one unknown-kind error, got %v", out.Errors)
	}
	mustContain(t, out.XML, `appType=actor`, `appType=proc`, `Mystery`)
}

// TestEmit_UMLClassCompartments: a Class lists its attributes and
// operations, one per line.
func TestEmit_UMLClassCompartments(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/uml2-class",
		Nodes: []drawioapi.LogicalNode{
			{ID: "order", Label: "Order", Kind: "Class", Attributes: map[string]any{
				"attributes": []any{"+id: string", "+total: decimal"},
				"operations": []any{"+submit()"},
			}},
		},
	}
	out, err := Emit(ctx, ld, true)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.XML, `+id: string&lt;br&gt;+total: decimal`, `+submit()`, `&lt;hr&gt;`)
}

// TestEmit_HardErrorBlocksRender: an unresolvable viewpoint draws nothing.
func TestEmit_HardErrorBlocksRender(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/nonexistent",
		Nodes:     []drawioapi.LogicalNode{{ID: "n1", Label: "A", Kind: "Application"}},
	}
	out, _ := Emit(ctx, ld, true)
	if out.XML != "" || len(out.Errors) == 0 {
		t.Errorf("want errors and no XML, got %+v", out)
	}
}

// TestEmit_OutsideViewpointWarns: a kind the notation knows but the
// viewpoint leaves out is a lint warning.
func TestEmit_OutsideViewpointWarns(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/archimate3-application-cooperation",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: "App", Kind: "ApplicationComponent"},
			{ID: "b", Label: "Actor", Kind: "BusinessActor"},
		},
		Edges: []drawioapi.LogicalEdge{{ID: "e", Source: "a", Target: "b", Kind: "serving"}},
	}
	out, err := Emit(ctx, ld, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(out.Warnings, " | ")
	if !strings.Contains(joined, "BusinessActor") || !strings.Contains(joined, "outside") && !strings.Contains(joined, "not one viewpoint") {
		t.Errorf("want an outside-viewpoint warning, got %v", out.Warnings)
	}
}

// TestEmit_PageChoiceAndValues: a diagram picks the dossier frame of the
// app-flux library and fills it; the choice reads back from the drawing.
func TestEmit_PageChoiceAndValues(t *testing.T) {
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Title:     "Architecture applicative actuelle",
		Nodes:     []drawioapi.LogicalNode{{ID: "a", Label: "Billing", Kind: "Application"}},
		Page:      &drawioapi.PageRef{Name: "dossier", Values: map[string]string{"project_code": "PRJ-042", "project_name": "Suivi de marge"}},
	}
	out, err := Emit(ctx, ld, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range out.Warnings {
		if strings.Contains(w, "page") {
			t.Errorf("page problem: %s", w)
		}
	}
	if len(out.Errors) > 0 {
		t.Fatalf("errors %v", out.Errors)
	}
	mustContain(t, out.XML, `PRJ-042 · Suivi de marge`, `Architecture applicative actuelle`, `Dossier d&#39;architecture`)
	r, err := Read([]byte(out.XML))
	if err != nil {
		t.Fatal(err)
	}
	if r.Diagram.Page == nil || r.Diagram.Page.Name != "dossier" || r.Diagram.Page.Values["project_code"] != "PRJ-042" {
		t.Errorf("page choice did not read back: %+v", r.Diagram.Page)
	}
}

// TestEmit_DisconnectedZonesKeepTheirColumns: a landscape is rarely one
// connected graph. Every island's members stay in their zone's columns —
// laid out island by island, each island had its own columns and the
// zones' frames overlapped.
func TestEmit_DisconnectedZonesKeepTheirColumns(t *testing.T) {
	needBun(t)
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Zones:     []drawioapi.LogicalZone{{ID: "up", Label: "Up", Order: 1}, {ID: "down", Label: "Down", Order: 2}},
	}
	for _, island := range []string{"a", "b", "c"} {
		ld.Nodes = append(ld.Nodes,
			drawioapi.LogicalNode{ID: island + "1", Label: island + "1", Kind: "Application", Zone: "up"},
			drawioapi.LogicalNode{ID: island + "2", Label: island + "2", Kind: "Application", Zone: "down"})
		ld.Edges = append(ld.Edges, drawioapi.LogicalEdge{ID: island, Source: island + "1", Target: island + "2", Kind: "Flow"})
	}
	// Apps with no flow in the picture: islands of one.
	ld.Nodes = append(ld.Nodes,
		drawioapi.LogicalNode{ID: "lone-up", Label: "lone up", Kind: "Application", Zone: "up"},
		drawioapi.LogicalNode{ID: "lone-down", Label: "lone down", Kind: "Application", Zone: "down"})
	out, err := Emit(ctx, ld, false)
	if err != nil {
		t.Fatal(err)
	}
	if q := out.Quality; q == nil || q.ZoneOverlaps > 0 || q.ZoneIntrusions > 0 {
		t.Fatalf("zones must not overlap: %+v", out.Quality)
	}
	r, err := Read([]byte(out.XML))
	if err != nil {
		t.Fatal(err)
	}
	up, down := r.Geometry["up"], r.Geometry["down"]
	if up.X+up.Width > down.X {
		t.Errorf("zone up %+v must end before zone down %+v", up, down)
	}
}
