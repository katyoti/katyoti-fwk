// SPDX-License-Identifier: Apache-2.0

// Package quality measures a laid-out diagram: overlaps, crossings, edges
// through nodes, bends, zone escapes, extent and density. It reads what a
// drawio file says (the reader's boxes and bend points), so it scores any
// diagram, generated or drawn by hand.
package quality

import (
	"math"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

type seg struct{ a, b drawioapi.Point }

// Score measures a diagram read back from a drawio file.
func Score(r *drawioapi.ReadResult) *drawioapi.QualityReport {
	q := &drawioapi.QualityReport{Nodes: len(r.Diagram.Nodes), Edges: len(r.Diagram.Edges)}
	boxes := make(map[string]drawioapi.Box, len(r.Diagram.Nodes))
	var ids []string
	for _, n := range r.Diagram.Nodes {
		if b, ok := r.Geometry[n.ID]; ok {
			boxes[n.ID] = b
			ids = append(ids, n.ID)
		}
	}

	// Overlaps.
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := boxes[ids[i]], boxes[ids[j]]
			// A box drawn wholly inside another is a grouping, not an overlap.
			if intersects(a, b) && !contains(a, b) && !contains(b, a) {
				q.Overlaps++
			}
		}
	}

	// Zone escapes.
	for _, n := range r.Diagram.Nodes {
		if n.Zone == "" {
			continue
		}
		zb, zok := r.Geometry[n.Zone]
		nb, nok := boxes[n.ID]
		if zok && nok && !contains(zb, nb) {
			q.ZoneEscapes++
		}
	}

	// Zone overlaps.
	var zones []drawioapi.Box
	for _, z := range r.Diagram.Zones {
		if b, ok := r.Geometry[z.ID]; ok {
			zones = append(zones, b)
		}
	}
	for i := 0; i < len(zones); i++ {
		for j := i + 1; j < len(zones); j++ {
			a, b := zones[i], zones[j]
			if intersects(a, b) && !contains(a, b) && !contains(b, a) {
				q.ZoneOverlaps++
			}
		}
	}

	// Zone intrusions: a node over a zone it is not a member of.
	for _, z := range r.Diagram.Zones {
		zb, ok := r.Geometry[z.ID]
		if !ok {
			continue
		}
		for _, n := range r.Diagram.Nodes {
			b, ok := boxes[n.ID]
			if !ok || n.Zone == z.ID || !intersects(zb, b) || contains(b, zb) {
				continue // a box drawn around the zone frames it
			}
			if own, nested := r.Geometry[n.Zone]; nested && n.Zone != "" && contains(zb, own) {
				continue // a member of a zone drawn inside this one
			}
			q.ZoneIntrusions++
		}
	}

	// Label collisions: a label over another label, or over a node.
	var labels []drawioapi.Box
	for _, e := range r.Diagram.Edges {
		if b, ok := r.EdgeLabels[e.ID]; ok {
			labels = append(labels, shrink(b, 1))
		}
	}
	for i := range labels {
		for j := i + 1; j < len(labels); j++ {
			if intersects(labels[i], labels[j]) {
				q.LabelCollisions++
			}
		}
		for _, id := range ids {
			if intersects(labels[i], boxes[id]) {
				q.LabelCollisions++
			}
		}
	}

	// What a person sees: labels over labels, out of their shape, shapes
	// covering each other.
	countFindings(q, CheckLayout(r))

	// Edges as polylines: source centre → bend points → target centre.
	type poly struct {
		src, tgt string
		segs     []seg
	}
	var polys []poly
	var total float64
	for _, e := range r.Diagram.Edges {
		sb, sok := boxes[e.Source]
		tb, tok := boxes[e.Target]
		if !sok || !tok {
			continue
		}
		pts := []drawioapi.Point{centre(sb)}
		for _, p := range r.EdgePoints[e.ID] {
			pts = append(pts, p)
			// A waypoint on an end's border is where the edge attaches
			// (ELK writes its start and end points), not a bend.
			if !onBorder(p, sb) && !onBorder(p, tb) {
				q.Bends++
			}
		}
		pts = append(pts, centre(tb))
		p := poly{src: e.Source, tgt: e.Target}
		for i := 0; i+1 < len(pts); i++ {
			s := seg{pts[i], pts[i+1]}
			p.segs = append(p.segs, s)
			total += math.Hypot(s.b.X-s.a.X, s.b.Y-s.a.Y)
		}
		polys = append(polys, p)
		// Edges through nodes: a segment entering a node that is not an end.
		for _, id := range ids {
			if id == e.Source || id == e.Target {
				continue
			}
			for _, s := range p.segs {
				if segmentHitsBox(s, shrink(boxes[id], 2)) {
					q.EdgesThroughNodes++
					break
				}
			}
		}
	}
	if len(polys) > 0 {
		q.MeanEdgeLength = round(total / float64(len(polys)))
	}

	// Crossings between different edges; segments touching a shared end
	// node do not count at that node.
	for i := 0; i < len(polys); i++ {
		for j := i + 1; j < len(polys); j++ {
			shared := polys[i].src == polys[j].src || polys[i].src == polys[j].tgt ||
				polys[i].tgt == polys[j].src || polys[i].tgt == polys[j].tgt
			for _, a := range polys[i].segs {
				for _, b := range polys[j].segs {
					if !properCross(a, b) {
						continue
					}
					if shared && nearAnyEnd(a, b, boxes, polys[i].src, polys[i].tgt, polys[j].src, polys[j].tgt) {
						continue
					}
					q.Crossings++
				}
			}
		}
	}

	// Extent and density.
	if len(ids) > 0 {
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		var covered float64
		for _, id := range ids {
			b := boxes[id]
			minX, minY = math.Min(minX, b.X), math.Min(minY, b.Y)
			maxX, maxY = math.Max(maxX, b.X+b.Width), math.Max(maxY, b.Y+b.Height)
			covered += b.Width * b.Height
		}
		q.Width, q.Height = round(maxX-minX), round(maxY-minY)
		if area := q.Width * q.Height; area > 0 {
			q.Density = math.Round(covered/area*1000) / 1000
		}
	}
	return q
}

func centre(b drawioapi.Box) drawioapi.Point {
	return drawioapi.Point{X: b.X + b.Width/2, Y: b.Y + b.Height/2}
}

// onBorder reports whether p lies on b's outline (within a pixel).
func onBorder(p drawioapi.Point, b drawioapi.Box) bool {
	inX := p.X >= b.X-1 && p.X <= b.X+b.Width+1
	inY := p.Y >= b.Y-1 && p.Y <= b.Y+b.Height+1
	nearX := math.Abs(p.X-b.X) <= 1 || math.Abs(p.X-b.X-b.Width) <= 1
	nearY := math.Abs(p.Y-b.Y) <= 1 || math.Abs(p.Y-b.Y-b.Height) <= 1
	return (inX && nearY) || (inY && nearX)
}

func intersects(a, b drawioapi.Box) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func contains(outer, inner drawioapi.Box) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width && inner.Y+inner.Height <= outer.Y+outer.Height
}

func shrink(b drawioapi.Box, d float64) drawioapi.Box {
	return drawioapi.Box{X: b.X + d, Y: b.Y + d, Width: math.Max(0, b.Width-2*d), Height: math.Max(0, b.Height-2*d)}
}

// segmentHitsBox reports whether a segment enters a box's interior.
func segmentHitsBox(s seg, b drawioapi.Box) bool {
	in := func(p drawioapi.Point) bool {
		return p.X > b.X && p.X < b.X+b.Width && p.Y > b.Y && p.Y < b.Y+b.Height
	}
	if in(s.a) || in(s.b) {
		return true
	}
	c := []drawioapi.Point{{X: b.X, Y: b.Y}, {X: b.X + b.Width, Y: b.Y}, {X: b.X + b.Width, Y: b.Y + b.Height}, {X: b.X, Y: b.Y + b.Height}}
	for i := range c {
		if properCross(s, seg{c[i], c[(i+1)%4]}) {
			return true
		}
	}
	return false
}

// properCross reports whether two segments cross at a single interior point.
func properCross(p, q seg) bool {
	d1 := orient(q.a, q.b, p.a)
	d2 := orient(q.a, q.b, p.b)
	d3 := orient(p.a, p.b, q.a)
	d4 := orient(p.a, p.b, q.b)
	return d1*d2 < 0 && d3*d4 < 0
}

func orient(a, b, c drawioapi.Point) float64 {
	v := (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
	switch {
	case v > 1e-9:
		return 1
	case v < -1e-9:
		return -1
	}
	return 0
}

// nearAnyEnd reports whether the crossing of a and b lies inside one of the
// end nodes the two edges share (edges fanning out of one node).
func nearAnyEnd(a, b seg, boxes map[string]drawioapi.Box, ends ...string) bool {
	p, ok := crossPoint(a, b)
	if !ok {
		return false
	}
	for _, id := range ends {
		bx := boxes[id]
		if p.X >= bx.X-1 && p.X <= bx.X+bx.Width+1 && p.Y >= bx.Y-1 && p.Y <= bx.Y+bx.Height+1 {
			return true
		}
	}
	return false
}

func crossPoint(p, q seg) (drawioapi.Point, bool) {
	x1, y1, x2, y2 := p.a.X, p.a.Y, p.b.X, p.b.Y
	x3, y3, x4, y4 := q.a.X, q.a.Y, q.b.X, q.b.Y
	den := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if math.Abs(den) < 1e-12 {
		return drawioapi.Point{}, false
	}
	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / den
	return drawioapi.Point{X: x1 + t*(x2-x1), Y: y1 + t*(y2-y1)}, true
}

func round(f float64) float64 { return math.Round(f*10) / 10 }
