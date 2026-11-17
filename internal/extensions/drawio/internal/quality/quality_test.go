// SPDX-License-Identifier: Apache-2.0

package quality

import (
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

func read(nodes map[string]drawioapi.Box, edges [][2]string, points map[string][]drawioapi.Point) *drawioapi.ReadResult {
	r := &drawioapi.ReadResult{Diagram: &drawioapi.LogicalDiagram{}, Geometry: nodes, EdgePoints: points}
	for id := range nodes {
		r.Diagram.Nodes = append(r.Diagram.Nodes, drawioapi.LogicalNode{ID: id})
	}
	for i, e := range edges {
		r.Diagram.Edges = append(r.Diagram.Edges, drawioapi.LogicalEdge{ID: string(rune('a' + i)), Source: e[0], Target: e[1]})
	}
	return r
}

func TestScore_CleanDiagram(t *testing.T) {
	q := Score(read(map[string]drawioapi.Box{
		"a": {X: 0, Y: 0, Width: 100, Height: 50},
		"b": {X: 300, Y: 0, Width: 100, Height: 50},
	}, [][2]string{{"a", "b"}}, nil))
	if q.Overlaps != 0 || q.Crossings != 0 || q.EdgesThroughNodes != 0 || q.Bends != 0 {
		t.Errorf("clean diagram scored %+v", q)
	}
	if q.Width != 400 || q.Height != 50 || q.MeanEdgeLength != 300 {
		t.Errorf("extent: %+v", q)
	}
}

func TestScore_Defects(t *testing.T) {
	q := Score(read(map[string]drawioapi.Box{
		"a": {X: 0, Y: 100, Width: 50, Height: 50},   // left
		"b": {X: 400, Y: 100, Width: 50, Height: 50}, // right
		"c": {X: 200, Y: 0, Width: 50, Height: 50},   // top
		"d": {X: 200, Y: 200, Width: 50, Height: 50}, // bottom
		"m": {X: 180, Y: 110, Width: 40, Height: 30}, // in the middle of a→b
		"o": {X: 410, Y: 110, Width: 50, Height: 50}, // overlaps b
	}, [][2]string{{"a", "b"}, {"c", "d"}}, map[string][]drawioapi.Point{"a": {{X: 100, Y: 125}}}))
	if q.Overlaps != 1 {
		t.Errorf("overlaps: want 1, got %d", q.Overlaps)
	}
	if q.Crossings != 1 {
		t.Errorf("crossings: want 1 (a→b × c→d), got %d", q.Crossings)
	}
	if q.EdgesThroughNodes < 1 {
		t.Errorf("a→b passes through m, got %d", q.EdgesThroughNodes)
	}
	if q.Bends != 1 {
		t.Errorf("bends: want 1, got %d", q.Bends)
	}
}

func TestScore_ZoneEscape(t *testing.T) {
	r := read(map[string]drawioapi.Box{"a": {X: 0, Y: 0, Width: 50, Height: 50}, "z": {X: 100, Y: 100, Width: 200, Height: 200}}, nil, nil)
	r.Diagram.Nodes = []drawioapi.LogicalNode{{ID: "a", Zone: "z"}}
	if q := Score(r); q.ZoneEscapes != 1 {
		t.Errorf("zone escapes: want 1, got %d", q.ZoneEscapes)
	}
}

func TestScore_SharedEndNotACrossing(t *testing.T) {
	// Two edges fanning out of one node cross nowhere else.
	q := Score(read(map[string]drawioapi.Box{
		"hub": {X: 0, Y: 0, Width: 100, Height: 100},
		"x":   {X: 300, Y: -100, Width: 50, Height: 50},
		"y":   {X: 300, Y: 150, Width: 50, Height: 50},
	}, [][2]string{{"hub", "x"}, {"hub", "y"}}, nil))
	if q.Crossings != 0 {
		t.Errorf("fan-out edges cross only at their shared end, got %d", q.Crossings)
	}
}

// TestScore_AttachmentPointsAreNotBends: waypoints on the ends' borders
// (ELK's start and end points) are not bends.
func TestScore_AttachmentPointsAreNotBends(t *testing.T) {
	q := Score(read(map[string]drawioapi.Box{
		"a": {X: 0, Y: 0, Width: 100, Height: 50},
		"b": {X: 300, Y: 200, Width: 100, Height: 50},
	}, [][2]string{{"a", "b"}}, map[string][]drawioapi.Point{"a": {{X: 100, Y: 25}, {X: 200, Y: 25}, {X: 200, Y: 225}, {X: 300, Y: 225}}}))
	if q.Bends != 2 {
		t.Errorf("want 2 bends (the two corners), got %d", q.Bends)
	}
}

// TestScore_NestingIsNotAnOverlap: a box drawn wholly inside another is a
// grouping.
func TestScore_NestingIsNotAnOverlap(t *testing.T) {
	q := Score(read(map[string]drawioapi.Box{
		"group": {X: 0, Y: 0, Width: 400, Height: 300},
		"child": {X: 20, Y: 20, Width: 100, Height: 50},
	}, nil, nil))
	if q.Overlaps != 0 {
		t.Errorf("nesting counted as %d overlaps", q.Overlaps)
	}
}

func TestScore_ZoneIntrusion(t *testing.T) {
	r := read(map[string]drawioapi.Box{
		"z":   {X: 0, Y: 0, Width: 300, Height: 200},
		"in":  {X: 20, Y: 40, Width: 100, Height: 50},
		"out": {X: 150, Y: 100, Width: 100, Height: 50}, // drawn over z, not a member
	}, nil, nil)
	r.Diagram.Nodes = []drawioapi.LogicalNode{{ID: "in", Zone: "z"}, {ID: "out"}}
	r.Diagram.Zones = []drawioapi.LogicalZone{{ID: "z"}}
	if q := Score(r); q.ZoneIntrusions != 1 {
		t.Errorf("zone intrusions: want 1, got %d", q.ZoneIntrusions)
	}
}

func TestScore_ZoneOverlap(t *testing.T) {
	r := read(map[string]drawioapi.Box{
		"z1": {X: 0, Y: 0, Width: 200, Height: 200},
		"z2": {X: 150, Y: 0, Width: 200, Height: 200},
		"z3": {X: 10, Y: 10, Width: 50, Height: 50}, // nested in z1
	}, nil, nil)
	r.Diagram.Nodes = nil
	r.Diagram.Zones = []drawioapi.LogicalZone{{ID: "z1"}, {ID: "z2"}, {ID: "z3"}}
	if q := Score(r); q.ZoneOverlaps != 1 {
		t.Errorf("zone overlaps: want 1 (z1×z2), got %d", q.ZoneOverlaps)
	}
}
