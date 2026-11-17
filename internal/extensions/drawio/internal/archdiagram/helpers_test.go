// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/template"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// renderShipped draws d with one of ktw-docs' shipped viewpoints, without
// layout (positions are the test's).
func renderShipped(t *testing.T, viewpoint string, d *ArchDiagram) []byte {
	t.Helper()
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	dr, err := drawioapi.ResolveDrawing(ctx, viewpoint)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := template.Parse(dr.Library)
	if err != nil {
		t.Fatal(err)
	}
	d.Viewpoint = viewpoint
	out, problems, err := Render(ctx, d, dr, lib, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Logf("problem: %s", p)
	}
	return out
}

// sampleDiagrams are small diagrams for each shipped viewpoint: zones,
// scalar and nested attributes, edges with labels.
func sampleDiagrams() map[string]*ArchDiagram {
	return map[string]*ArchDiagram{
		"drawio-test:/viewpoint/app-flux": {
			Title: "App flux sample",
			Zones: []*Zone{{ID: "amont", Label: "AMONT", Order: 0}},
			Nodes: []*Node{
				{ID: "billing", Label: "Billing", Type: "Application", Zone: "amont", X: 10, Y: 40, Data: map[string]any{
					"domain": "reference", "markers": []any{"xls"}, "description": "Invoices", "external_url": "https://x.test/billing",
				}},
				{ID: "ledger", Label: "Ledger", Type: "Application", X: 300, Y: 40, Data: map[string]any{"domain": "invoicing"}},
				{ID: "note", Label: "Pilote BE", Type: "Note", X: 10, Y: 200},
			},
			Edges: []*Edge{{ID: "f1", Source: "billing", Target: "ledger", EdgeStyle: "Flow", Label: "prices", Data: map[string]any{
				"flow_type": "catalog", "interface_codes": []any{map[string]any{"code": "IF-1", "url": "u1"}},
			}}},
		},
		"ktw-docs:/viewpoint/archimate3-layered": {
			Title: "ArchiMate sample",
			Nodes: []*Node{
				{ID: "customer", Label: "Customer", Type: "BusinessActor", X: 0, Y: 0},
				{ID: "order", Label: "Place order", Type: "BusinessProcess", X: 0, Y: 150},
				{ID: "app", Label: "OrderApp", Type: "ApplicationComponent", X: 0, Y: 300},
			},
			Edges: []*Edge{
				{ID: "e1", Source: "customer", Target: "order", EdgeStyle: "triggering"},
				{ID: "e2", Source: "app", Target: "order", EdgeStyle: "serving"},
			},
		},
		"ktw-docs:/viewpoint/uml2-class": {
			Title: "UML sample",
			Nodes: []*Node{
				{ID: "order", Label: "Order", Type: "Class", X: 0, Y: 0, Data: map[string]any{"attributes": []any{"+id"}, "operations": []any{"+submit()"}}},
				{ID: "line", Label: "Line", Type: "Class", X: 300, Y: 0},
			},
			Edges: []*Edge{{ID: "c", Source: "order", Target: "line", EdgeStyle: "composition"}},
		},
	}
}
