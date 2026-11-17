// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// Refine draws the next version of a diagram over the previous drawing,
// moving nothing the person has already seen: every node and zone the
// previous drawing has keeps its box, what a person pinned stays pinned,
// and a new node starts in its zone below the last member, or beside the
// first node it is linked to, moved aside if it lands on something; zones
// grow to hold what joins them. No layout engine runs — to tidy a drawing,
// lay it out again (Relayout). previous is the .drawio of the last version
// as the person left it (plain, compressed, .drawio.png or .drawio.svg); a
// previous drawing with none of the model's nodes gives a fresh Emit.
//
// The model itself comes from the caller: to keep what a person changed
// in draw.io (an Edit Data value, a box added by hand), build it from
// Read(previous).
func Refine(ctx context.Context, ld *drawioapi.LogicalDiagram, previous []byte) (EmitResult, error) {
	prev, err := Read(previous)
	if err != nil {
		return EmitResult{}, fmt.Errorf("read the previous drawing: %w", err)
	}
	out, err := Emit(ctx, ld, true)
	if err != nil || out.XML == "" {
		return out, err
	}
	// The draft gives each new node its drawn size.
	var size map[string]drawioapi.Box
	if draft, err := Read([]byte(out.XML)); err == nil {
		size = draft.Geometry
	}
	down, lanes := readsDown(ctx, ld)
	boxes, carried := carryOver(ld, prev, size, down, lanes)
	if len(carried) == 0 {
		return Emit(ctx, ld, false)
	}
	placed, err := archdiagram.Place([]byte(out.XML), boxes, prev.Pinned)
	if err != nil {
		return out, fmt.Errorf("place: %w", err)
	}
	res, err := archdiagram.Settle(placed, carried, 40)
	if err != nil {
		return out, fmt.Errorf("settle: %w", err)
	}
	out.XML = string(res.XML)
	// The draft's quality (every node at its model position, unplaced) is
	// not the drawing's: drop its warning and score what was placed.
	kept := out.Warnings[:0]
	for _, w := range out.Warnings {
		if !strings.HasPrefix(w, "quality: ") {
			kept = append(kept, w)
		}
	}
	out.Warnings = kept
	out.Quality, _ = Score(res.XML)
	if q := out.Quality; q != nil && q.Overlaps > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("quality: %d overlapping node pair(s)", q.Overlaps))
	}
	return out, nil
}

// readsDown reports whether the diagram reads top to bottom (its own
// layout hints over its viewpoint's, DOWN by default), and whether its
// zones are lanes (bands along the reading direction) rather than
// partitions (bands across it).
func readsDown(ctx context.Context, ld *drawioapi.LogicalDiagram) (down, lanes bool) {
	var profile *drawioapi.LayoutHints
	if dr, err := drawioapi.ResolveDrawing(ctx, ld.Viewpoint); err == nil && dr.Viewpoint != nil {
		profile = dr.Viewpoint.Layout
	}
	l := mergeLayout(projectLayoutHints(ld.LayoutHints), projectLayoutHints(profile))
	if l == nil {
		return true, false
	}
	return l.Direction == "" || l.Direction == "DOWN" || l.Direction == "UP", l.Lanes
}

// carryOver is where each node and zone goes: its box in the previous
// drawing, or — for a new node — next to the last member of its zone,
// across the reading direction (beside it when the diagram reads down,
// below it when it reads right: the zone grows within its own band), else
// beside the first node it is linked to, else below the drawing; a new
// zone goes around its members. Lanes are bands along the reading
// direction: there a new member stays in its lane's row — next to a node
// of the same lane it is linked to when that place is free, else past the
// lane's last member. A new node keeps its drawn size (size, by id) and is
// centred on the row or column it joins. It also returns the ids of the
// nodes carried over.
func carryOver(ld *drawioapi.LogicalDiagram, prev *drawioapi.ReadResult, size map[string]drawioapi.Box, down, lanes bool) (map[string]drawioapi.Box, []string) {
	boxes := map[string]drawioapi.Box{}
	for _, z := range ld.Zones {
		if b, ok := prev.Geometry[z.ID]; ok {
			boxes[z.ID] = b
		}
	}
	var carried []string
	bottom, left := 0.0, math.Inf(1)
	for _, n := range ld.Nodes {
		if b, ok := prev.Geometry[n.ID]; ok {
			boxes[n.ID] = b
			carried = append(carried, n.ID)
			bottom = math.Max(bottom, b.Y+b.Height)
			left = math.Min(left, b.X)
		}
	}
	if len(carried) == 0 {
		return boxes, nil // nothing carried over: a first drawing
	}
	neighbour := map[string]string{}
	for _, e := range ld.Edges {
		if _, ok := neighbour[e.Target]; !ok {
			neighbour[e.Target] = e.Source
		}
		if _, ok := neighbour[e.Source]; !ok {
			neighbour[e.Source] = e.Target
		}
	}
	beside := map[string]int{}
	fresh := 0
	// A new member of a zone goes past its last box: across the reading
	// direction in a partition, along it in a lane.
	along := down
	if lanes {
		along = !down
	}
	last := map[string]drawioapi.Box{}
	zoneOf := map[string]string{}
	for _, n := range ld.Nodes {
		zoneOf[n.ID] = n.Zone
		if b, ok := boxes[n.ID]; ok && n.Zone != "" {
			l, seen := last[n.Zone]
			if !seen || (along && b.X+b.Width > l.X+l.Width) || (!along && b.Y+b.Height > l.Y+l.Height) {
				last[n.Zone] = b
			}
		}
	}
	// next is the box of new node id past ref: to its right when along,
	// else below it, centred on ref.
	next := func(id string, ref drawioapi.Box, along bool) drawioapi.Box {
		w, h := ref.Width, ref.Height
		if b, ok := size[id]; ok && b.Width > 0 && b.Height > 0 {
			w, h = b.Width, b.Height
		}
		if along {
			return drawioapi.Box{X: ref.X + ref.Width + 60, Y: ref.Y + ref.Height/2 - h/2, Width: w, Height: h}
		}
		return drawioapi.Box{X: ref.X + ref.Width/2 - w/2, Y: ref.Y + ref.Height + 40, Width: w, Height: h}
	}
	isZone := map[string]bool{}
	for _, z := range ld.Zones {
		isZone[z.ID] = true
	}
	// free reports whether a box lands on no node.
	free := func(b drawioapi.Box) bool {
		for id, o := range boxes {
			if !isZone[id] && b.X < o.X+o.Width && o.X < b.X+b.Width && b.Y < o.Y+o.Height && o.Y < b.Y+b.Height {
				return false
			}
		}
		return true
	}
	for _, n := range ld.Nodes {
		if _, ok := boxes[n.ID]; ok {
			continue
		}
		if lanes && n.Zone != "" {
			// Beside a linked node of the same lane, in its row, when free.
			if nb := neighbour[n.ID]; zoneOf[nb] == n.Zone {
				if m, ok := boxes[nb]; ok {
					if b := next(n.ID, m, along); free(b) {
						boxes[n.ID] = b
						continue
					}
				}
			}
		}
		if l, ok := last[n.Zone]; ok {
			b := next(n.ID, l, along)
			boxes[n.ID] = b
			last[n.Zone] = b
			continue
		}
		if m, ok := boxes[neighbour[n.ID]]; ok {
			k := beside[neighbour[n.ID]]
			beside[neighbour[n.ID]]++
			boxes[n.ID] = drawioapi.Box{X: m.X + m.Width + 80, Y: m.Y + float64(k)*(m.Height+40), Width: m.Width, Height: m.Height}
			continue
		}
		boxes[n.ID] = drawioapi.Box{X: left + float64(fresh)*200, Y: bottom + 80, Width: 160, Height: 60}
		fresh++
	}
	for _, z := range ld.Zones {
		if _, ok := boxes[z.ID]; ok {
			continue
		}
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, n := range ld.Nodes {
			if n.Zone != z.ID && !slices.Contains(z.Members, n.ID) {
				continue
			}
			b := boxes[n.ID]
			minX, minY = math.Min(minX, b.X), math.Min(minY, b.Y)
			maxX, maxY = math.Max(maxX, b.X+b.Width), math.Max(maxY, b.Y+b.Height)
		}
		if !math.IsInf(minX, 1) {
			boxes[z.ID] = drawioapi.Box{X: minX - 30, Y: minY - 45, Width: maxX - minX + 60, Height: maxY - minY + 75}
		}
	}
	return boxes, carried
}
