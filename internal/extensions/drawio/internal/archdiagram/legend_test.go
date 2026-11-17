// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"strings"
	"testing"
)

// TestLegend_KindsUsed asserts the legend lists every kind the diagram
// uses, once, labelled by the notation.
func TestLegend_KindsUsed(t *testing.T) {
	d := sampleDiagrams()["ktw-docs:/viewpoint/archimate3-layered"]
	xml := string(renderShipped(t, "ktw-docs:/viewpoint/archimate3-layered", d))
	for _, want := range []string{`label="Legend"`, `label="Business Actor"`, `label="Business Process"`, `label="Application Component"`, `label="Triggering"`, `label="Serving"`, `as="sourcePoint"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("legend missing %s", want)
		}
	}
	if n := strings.Count(xml, `label="Business Actor"`); n != 1 {
		t.Errorf("a kind appears once in the legend, got %d", n)
	}
}

// TestLegend_OneKindHasNoLegend asserts a single-kind diagram gets no
// legend — it would explain nothing.
func TestLegend_OneKindHasNoLegend(t *testing.T) {
	d := &ArchDiagram{Nodes: []*Node{{ID: "a", Label: "A", Type: "Application"}, {ID: "b", Label: "B", Type: "Application"}}}
	if strings.Contains(string(renderShipped(t, "drawio-test:/viewpoint/app-flux", d)), `label="Legend"`) {
		t.Error("one-kind diagram should have no legend")
	}
}

// TestOutsidePartitions: a node outside every zone goes before the zones
// when it only feeds them, after when it is only fed by them.
func TestOutsidePartitions(t *testing.T) {
	d := &ArchDiagram{
		Nodes: []*Node{{ID: "user"}, {ID: "app", Zone: "z1"}, {ID: "db", Zone: "z2"}, {ID: "mail"}, {ID: "both"}, {ID: "alone"}},
		Edges: []*Edge{
			{ID: "1", Source: "user", Target: "app"}, {ID: "2", Source: "app", Target: "db"}, {ID: "3", Source: "db", Target: "mail"},
			{ID: "4", Source: "both", Target: "app"}, {ID: "5", Source: "db", Target: "both"},
		},
	}
	got := outsidePartitions(d, map[string]int{"z1": 1, "z2": 2})
	if got["user"] != 0 || got["mail"] != 3 {
		t.Errorf("user before (0), mail after (3): %v", got)
	}
	if _, ok := got["both"]; ok {
		t.Errorf("a node both feeding and fed by zones stays free: %v", got)
	}
	if _, ok := got["alone"]; ok {
		t.Errorf("a node with no zoned neighbour stays free: %v", got)
	}
}
