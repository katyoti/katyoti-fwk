// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// ctxWithAppFlux returns a context whose catalog holds ktw-docs' shipped
// drawio content and the app-flux test notation (drawiotesting.TestCatalog).
func ctxWithAppFlux(t *testing.T) context.Context {
	t.Helper()
	return drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
}

func TestTranslateLogicalDiagram_HappyPath(t *testing.T) {
	ctx := ctxWithAppFlux(t)

	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Title:     "Test diagram",
		Nodes: []drawioapi.LogicalNode{
			{ID: "billing", Label: "Billing", Kind: "Application",
				Attributes: map[string]any{"domain": "reference"}, Zone: "z1"},
			{ID: "ledger", Label: "Ledger", Kind: "Application",
				Attributes: map[string]any{"domain": "accounting"}},
		},
		Edges: []drawioapi.LogicalEdge{
			{ID: "e1", Source: "billing", Target: "ledger", Kind: "Flow",
				Attributes: map[string]any{"flow_type": "invoice"}},
		},
		Zones: []drawioapi.LogicalZone{
			{ID: "z1", Label: "Sources", Order: 0},
		},
		LayoutHints: &drawioapi.LayoutHints{Algorithm: drawioapi.LayoutLayered, Direction: "RIGHT"},
	}

	d, dr, err := TranslateLogicalDiagram(ctx, ld)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	// The drawing resolved: viewpoint, notation, library.
	if dr == nil || dr.Viewpoint.ID != "app-flux" || dr.Notation.ID != "app-flux" || dr.Library.ID != "app-flux" {
		t.Errorf("drawing: got %+v", dr)
	}

	// Title carried through.
	if d.Title != "Test diagram" {
		t.Errorf("Title: got %q", d.Title)
	}

	// Nodes: kind → Type, attributes → Data, zone preserved.
	if len(d.Nodes) != 2 {
		t.Fatalf("Nodes: want 2, got %d", len(d.Nodes))
	}
	n0 := d.Nodes[0]
	if n0.ID != "billing" || n0.Label != "Billing" || n0.Type != "Application" || n0.Zone != "z1" {
		t.Errorf("Node 0 mapping wrong: %+v", n0)
	}
	if n0.Data["domain"] != "reference" {
		t.Errorf("Node 0 Data: want domain=reference, got %v", n0.Data)
	}

	// Edges: kind → EdgeStyle, attributes → Data.
	if len(d.Edges) != 1 {
		t.Fatalf("Edges: want 1, got %d", len(d.Edges))
	}
	e0 := d.Edges[0]
	if e0.EdgeStyle != "Flow" {
		t.Errorf("EdgeStyle: got %q", e0.EdgeStyle)
	}
	if e0.Data["flow_type"] != "invoice" {
		t.Errorf("Edge Data: want flow_type=invoice, got %v", e0.Data)
	}

	// Zones: 1-1.
	if len(d.Zones) != 1 || d.Zones[0].ID != "z1" {
		t.Errorf("Zones: got %+v", d.Zones)
	}

	// Layout hints projected.
	if d.Layout == nil || d.Layout.Algorithm != "layered" || d.Layout.Direction != "RIGHT" {
		t.Errorf("Layout: got %+v", d.Layout)
	}
}

// TestTranslateLogicalDiagram_UnknownViewpoint asserts infra error
// surfaces as a Go error (NOT a soft empty pivot).
func TestTranslateLogicalDiagram_UnknownViewpoint(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/nonexistent",
		Nodes:     []drawioapi.LogicalNode{{ID: "n1", Label: "A", Kind: "X"}},
	}
	_, _, err := TranslateLogicalDiagram(ctx, ld)
	if err == nil {
		t.Fatal("want Go error for a missing viewpoint, got nil")
	}
}

// TestTranslateLogicalDiagram_NilInput asserts nil-safety.
func TestTranslateLogicalDiagram_NilInput(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	_, _, err := TranslateLogicalDiagram(ctx, nil)
	if err == nil {
		t.Fatal("want Go error for nil diagram, got nil")
	}
}

// TestTranslateLogicalDiagram_EmptyOptionals asserts a minimal diagram
// (only required fields) translates, and takes its layout from the
// viewpoint's profile.
func TestTranslateLogicalDiagram_EmptyOptionals(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Nodes:     []drawioapi.LogicalNode{{ID: "n1", Label: "A", Kind: "Application"}},
	}
	d, _, err := TranslateLogicalDiagram(ctx, ld)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(d.Nodes) != 1 || len(d.Edges) != 0 || len(d.Zones) != 0 {
		t.Errorf("EmptyOptionals shape: %+v", d)
	}
	if d.Layout == nil || d.Layout.Algorithm != "layered" || d.Layout.Direction != "RIGHT" {
		t.Errorf("layout should be the app-flux profile (layered RIGHT), got %+v", d.Layout)
	}
}

// TestTranslateLogicalDiagram_DiagramHintsWinOverProfile asserts the
// diagram's own direction wins, the profile fills the rest.
func TestTranslateLogicalDiagram_DiagramHintsWinOverProfile(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint:   "drawio-test:/viewpoint/app-flux",
		Nodes:       []drawioapi.LogicalNode{{ID: "n1", Label: "A", Kind: "Application"}},
		LayoutHints: &drawioapi.LayoutHints{Direction: "DOWN"},
	}
	d, _, err := TranslateLogicalDiagram(ctx, ld)
	if err != nil {
		t.Fatal(err)
	}
	if d.Layout.Direction != "DOWN" || d.Layout.Algorithm != "layered" {
		t.Errorf("want DOWN from the diagram and layered from the profile, got %+v", d.Layout)
	}
}

// TestTranslateLogicalDiagram_ZoneMembers asserts a zone's members list
// places the nodes that name no zone themselves.
func TestTranslateLogicalDiagram_ZoneMembers(t *testing.T) {
	ctx := ctxWithAppFlux(t)
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Nodes:     []drawioapi.LogicalNode{{ID: "a", Label: "A", Kind: "Application"}, {ID: "b", Label: "B", Kind: "Application", Zone: "z2"}},
		Zones:     []drawioapi.LogicalZone{{ID: "z1", Label: "One", Members: []string{"a", "b"}}, {ID: "z2", Label: "Two"}},
	}
	d, _, err := TranslateLogicalDiagram(ctx, ld)
	if err != nil {
		t.Fatal(err)
	}
	if d.Nodes[0].Zone != "z1" || d.Nodes[1].Zone != "z2" {
		t.Errorf("zones: a=%q b=%q, want z1 and z2 (a node's own zone wins)", d.Nodes[0].Zone, d.Nodes[1].Zone)
	}
}
