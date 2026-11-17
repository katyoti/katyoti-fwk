// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// Lint doesn't hit the catalog — uses context.Background() directly.
func TestLint_CleanDiagram(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: "A", Kind: "X"},
			{ID: "b", Label: "B", Kind: "X"},
		},
		Edges: []drawioapi.LogicalEdge{{ID: "e1", Source: "a", Target: "b", Kind: "F"}},
	}
	r, _ := Lint(context.Background(), ld)
	if len(r.Warnings) != 0 {
		t.Errorf("clean diagram should produce no warnings, got: %v", r.Warnings)
	}
}

func TestLint_LongLabel(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: strings.Repeat("very-long-label-", 10), Kind: "X"},
			{ID: "b", Label: "B", Kind: "X"},
		},
		Edges: []drawioapi.LogicalEdge{{ID: "e1", Source: "a", Target: "b", Kind: "F"}},
	}
	r, _ := Lint(context.Background(), ld)
	if findCode(r.Warnings, "long-label") == nil {
		t.Errorf("want long-label warning, got: %v", r.Warnings)
	}
}

func TestLint_OrphanNode(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: "A", Kind: "X"}, // orphan — no edges
		},
	}
	r, _ := Lint(context.Background(), ld)
	if findCode(r.Warnings, "orphan-node") == nil {
		t.Errorf("want orphan-node warning, got: %v", r.Warnings)
	}
}

func TestLint_OrphanNodeButZoned_NotWarned(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: "A", Kind: "X", Zone: "z1"}, // no edges but has zone
		},
		Zones: []drawioapi.LogicalZone{{ID: "z1", Label: "Z", Order: 0}},
	}
	r, _ := Lint(context.Background(), ld)
	if findCode(r.Warnings, "orphan-node") != nil {
		t.Errorf("zoned node should not be flagged orphan, got: %v", r.Warnings)
	}
}

func TestLint_EmptyZone(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes:     []drawioapi.LogicalNode{{ID: "a", Label: "A", Kind: "X"}},
		Zones:     []drawioapi.LogicalZone{{ID: "z-empty", Label: "Empty", Order: 0}},
	}
	r, _ := Lint(context.Background(), ld)
	if findCode(r.Warnings, "empty-zone") == nil {
		t.Errorf("want empty-zone warning, got: %v", r.Warnings)
	}
}

func TestLint_ZoneWithMembers_NotEmpty(t *testing.T) {
	ld := &drawioapi.LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes: []drawioapi.LogicalNode{
			{ID: "a", Label: "A", Kind: "X", Zone: "z1"},
		},
		Zones: []drawioapi.LogicalZone{{ID: "z1", Label: "Z", Order: 0}},
	}
	r, _ := Lint(context.Background(), ld)
	if findCode(r.Warnings, "empty-zone") != nil {
		t.Errorf("non-empty zone should not be flagged, got: %v", r.Warnings)
	}
}

func TestLint_NilDiagram(t *testing.T) {
	r, err := Lint(context.Background(), nil)
	if err != nil {
		t.Errorf("Lint should not error on nil, got: %v", err)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("nil diagram should not produce warnings, got: %v", r.Warnings)
	}
}
