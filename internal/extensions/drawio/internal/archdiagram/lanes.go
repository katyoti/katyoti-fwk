// SPDX-License-Identifier: Apache-2.0

// lanes.go — zones drawn as lanes: bands across the reading direction
// that every flow crosses (BPMN pools and lanes, AWS groups). ELK orders
// the nodes along the reading direction; each zone then stacks its own
// nodes in a band, its inner zones below them, and is drawn around both.

package archdiagram

import (
	"context"
	"math"
	"sort"
)

// laneHeader is the room a zone's label takes: on the band's leading edge
// for side headers (a draw.io swimlane drawn horizontal=0), above its
// content for top headers (an AWS group).
const laneHeader = 30.0

// laneTopHeader is the room a top header takes above a zone's content.
const laneTopHeader = 40.0

// lanePad is the room between a band's edge and its nodes.
const lanePad = 20.0

// labelRoom is the room kept under each row for labels drawn below their
// shape (an icon's name, an event's label).
const labelRoom = 16.0

// LayoutLanes lays the diagram out with its zones as lanes. The reading
// direction is RIGHT or DOWN (LEFT and UP read as their mirror). With
// side headers (the default) a zone's label sits on the band's leading
// edge and nested bands share the trailing edge; with top headers
// (LayoutOptions.LaneHeader "top") a zone's label sits above its content
// and nested zones are boxes inside boxes.
func LayoutLanes(ctx context.Context, d *ArchDiagram) error {
	// Order along the reading direction: ELK on the flat graph, zones
	// ignored (partitions would put each lane in its own layers).
	flat := *d
	flat.Zones = nil
	if err := RunELKLayout(ctx, &flat); err != nil {
		return err
	}
	for _, e := range d.Edges {
		e.BendPoints = nil // the lanes move nodes across; draw.io routes
		e.LabelAt = nil
	}
	down := d.Layout != nil && (d.Layout.Direction == "DOWN" || d.Layout.Direction == "UP")
	top := d.Layout != nil && d.Layout.LaneHeader == "top"
	along := func(n *Node) (float64, float64) {
		if down {
			return n.Y, n.Height
		}
		return n.X, n.Width
	}
	across := func(n *Node) (float64, float64) {
		if down {
			return n.X, n.Width
		}
		return n.Y, n.Height
	}
	place := func(n *Node, a, c float64) {
		if down {
			n.X, n.Y = c, a
		} else {
			n.X, n.Y = a, c
		}
	}

	byID := map[string]*Zone{}
	for _, z := range d.Zones {
		byID[z.ID] = z
	}
	children := map[string][]*Zone{}
	var roots []*Zone
	for _, z := range d.Zones {
		if p := byID[z.Parent]; p != nil {
			children[p.ID] = append(children[p.ID], z)
		} else {
			roots = append(roots, z)
		}
	}
	byOrder := func(zs []*Zone) {
		sort.SliceStable(zs, func(i, j int) bool { return zs[i].Order < zs[j].Order })
	}
	byOrder(roots)
	depth := map[string]int{}
	levels := 0
	var measure func(z *Zone, k int)
	measure = func(z *Zone, k int) {
		depth[z.ID] = k
		levels = max(levels, k+1)
		byOrder(children[z.ID])
		for _, c := range children[z.ID] {
			measure(c, k+1)
		}
	}
	for _, z := range roots {
		measure(z, 0)
	}

	// Along the reading direction: ELK's position, after the headers or
	// the nesting indents.
	indent := laneHeader
	if top {
		indent = lanePad
	}
	minA := math.Inf(1)
	for _, n := range d.Nodes {
		a, _ := along(n)
		minA = min(minA, a)
	}
	offset := float64(levels)*indent + lanePad
	maxA := 0.0
	for _, n := range d.Nodes {
		a, size := along(n)
		c, _ := across(n)
		place(n, a-minA+offset, c)
		maxA = max(maxA, a-minA+offset+size)
	}
	end := maxA + lanePad

	members := map[string][]*Node{}
	var loose []*Node
	for _, n := range d.Nodes {
		if byID[n.Zone] != nil {
			members[n.Zone] = append(members[n.Zone], n)
		} else {
			loose = append(loose, n)
		}
	}

	cursor := 0.0
	// stack places nodes in rows from the cursor, in ELK's cross order,
	// and moves the cursor past them.
	stack := func(ns []*Node) {
		if len(ns) == 0 {
			return
		}
		sort.SliceStable(ns, func(i, j int) bool {
			ci, _ := across(ns[i])
			cj, _ := across(ns[j])
			return ci < cj
		})
		type row struct {
			spans   [][2]float64
			extent  float64
			members []*Node
		}
		var rows []*row
		for _, n := range ns {
			a, size := along(n)
			var into *row
			for _, r := range rows {
				free := true
				for _, s := range r.spans {
					if a < s[1]+lanePad && s[0] < a+size+lanePad {
						free = false
						break
					}
				}
				if free {
					into = r
					break
				}
			}
			if into == nil {
				into = &row{}
				rows = append(rows, into)
			}
			into.spans = append(into.spans, [2]float64{a, a + size})
			into.members = append(into.members, n)
			_, extent := across(n)
			into.extent = max(into.extent, extent)
		}
		c := cursor + lanePad
		for _, r := range rows {
			for _, n := range r.members {
				a, _ := along(n)
				_, extent := across(n)
				place(n, a, c+(r.extent-extent)/2)
			}
			c += r.extent + lanePad + labelRoom
		}
		cursor = c
	}
	box := func(z *Zone, from, size float64) {
		lead := float64(depth[z.ID]) * indent
		trail := end
		if top {
			trail = end + float64(levels-depth[z.ID])*indent
		}
		if down {
			z.Bounds = &Rect{X: from, Y: lead, Width: size, Height: trail - lead}
		} else {
			z.Bounds = &Rect{X: lead, Y: from, Width: trail - lead, Height: size}
		}
	}
	var lay func(z *Zone)
	lay = func(z *Zone) {
		start := cursor
		if top {
			cursor += laneTopHeader
		}
		stack(members[z.ID])
		for _, c := range children[z.ID] {
			lay(c)
		}
		if top && len(children[z.ID]) > 0 {
			cursor += lanePad
		}
		if cursor-start < 2*lanePad+60 {
			cursor = start + 2*lanePad + 60
		}
		box(z, start, cursor-start)
	}
	stack(loose)
	if len(loose) > 0 {
		cursor += lanePad
	}
	for _, z := range roots {
		lay(z)
		if top {
			cursor += lanePad
		}
	}
	return nil
}
