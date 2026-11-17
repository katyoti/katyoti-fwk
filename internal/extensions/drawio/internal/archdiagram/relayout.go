// SPDX-License-Identifier: Apache-2.0

// relayout.go — layout as a stage on a .drawio: read the drawing, lay it
// out with ELK (groups are compound nodes, laid out recursively), write the
// new geometry back. Only mxGeometry elements change; every other byte of
// the document stays as it was.

package archdiagram

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// RelayoutResult is a drawing laid out again.
type RelayoutResult struct {
	// XML is the laid-out document. A compressed input comes back
	// uncompressed; otherwise only geometry differs from the input.
	XML []byte
	// Moved counts the vertices given a new box.
	Moved int
	// Pinned lists the cell ids kept where they were.
	Pinned []string
	// Report lists what the layout left alone or had to adjust.
	Report []string
}

// containerPadding is the room ELK leaves inside a group: the top holds
// the group's label.
const containerPadding = "[top=45,left=25,bottom=25,right=25]"

// relNode is a node of the ELK graph a relayout sends: a leaf with its
// size, or a group whose size ELK computes from its children.
type relNode struct {
	ID            string            `json:"id"`
	Width         float64           `json:"width,omitempty"`
	Height        float64           `json:"height,omitempty"`
	X             float64           `json:"x,omitempty"`
	Y             float64           `json:"y,omitempty"`
	LayoutOptions map[string]string `json:"layoutOptions,omitempty"`
	Children      []*relNode        `json:"children,omitempty"`
	Edges         []relEdge         `json:"edges,omitempty"`
}

// relEdge is an ELK edge; on output, Container names the node its
// sections are relative to (elkjs ignores elk.json.edgeCoords).
type relEdge struct {
	ID        string         `json:"id"`
	Sources   []string       `json:"sources"`
	Targets   []string       `json:"targets"`
	Labels    []elkLabel_t   `json:"labels,omitempty"`
	Sections  []elkSection_t `json:"sections,omitempty"`
	Container string         `json:"container,omitempty"`
}

// geoSpan locates a cell's mxGeometry element in the document.
type geoSpan struct{ start, end int }

// Relayout lays a .drawio document out again with the given layout
// options (nil means DefaultLayout). Vertices keep their size; groups
// (a vertex with vertices inside, by parent or drawn around them) are
// laid out recursively and resized to their content; cells a person
// pinned (ktw-pinned="1" or draw.io's locked style) keep their box, and
// what they would overlap moves aside. The legend and page decorations
// stay where they are.
func Relayout(ctx context.Context, data []byte, layout *LayoutOptions) (*RelayoutResult, error) {
	return relayout(ctx, data, layout, nil)
}

// Settle places what is new in a drawing without moving what is not:
// the cells whose model id is in fixed keep their box, the others keep
// theirs unless it overlaps, in which case they move aside; groups grow
// to hold their content. No layout engine runs.
func Settle(data []byte, fixed []string, spacing float64) (*RelayoutResult, error) {
	set := map[string]bool{}
	for _, id := range fixed {
		set[id] = true
	}
	return relayout(context.Background(), data, &LayoutOptions{Direction: "RIGHT", NodeSpacing: spacing}, set)
}

// relayout lays a drawing out with ELK, seeded by its positions, or —
// when fixed is not nil — only settles it around the fixed cells.
func relayout(ctx context.Context, data []byte, layout *LayoutOptions, fixed map[string]bool) (*RelayoutResult, error) {
	doc := data
	res := &RelayoutResult{}
	head := bytes.TrimSpace(doc)
	if bytes.Contains(head[:min(len(head), 512)], []byte("<mxfile")) && compressed(head) {
		expanded, err := drawioapi.DecompressDiagram(head)
		if err != nil {
			return nil, err
		}
		doc = expanded
		res.Report = append(res.Report, "the input was compressed: the output is written uncompressed")
	}
	cells, err := parseCells(doc)
	if err != nil {
		return nil, err
	}
	spans, err := scanGeometry(doc)
	if err != nil {
		return nil, err
	}
	if layout == nil {
		layout = DefaultLayout()
	}
	p := newPlan(cells)
	if len(p.members) == 0 {
		res.XML = doc
		res.Report = append(res.Report, "nothing to lay out: the drawing has no vertices")
		return res, nil
	}
	if fixed != nil {
		return p.settle(doc, spans, cells, fixed, layout, res), nil
	}

	graph, edges := p.elkGraph(layout, res)
	in, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("relayout: marshal ELK graph: %w", err)
	}
	out, err := LayoutELKGraph(ctx, in)
	if err != nil {
		return nil, err
	}
	var laid relNode
	if err := json.Unmarshal(out, &laid); err != nil {
		return nil, fmt.Errorf("relayout: parse ELK output: %w", err)
	}

	boxes := map[string]drawioapi.Box{}
	collectBoxes(laid.Children, 0, 0, boxes)
	// Edge sections are relative to their container: make them absolute
	// before the boxes move.
	for i, e := range laid.Edges {
		c, ok := boxes[e.Container]
		if !ok {
			continue
		}
		for j := range e.Sections {
			for k := range e.Sections[j].BendPoints {
				laid.Edges[i].Sections[j].BendPoints[k].X += c.X
				laid.Edges[i].Sections[j].BendPoints[k].Y += c.Y
			}
		}
	}
	p.anchor(boxes)
	moved := p.keepPinned(boxes, layout, res)
	p.ride(boxes)
	points := map[string][]drawioapi.Point{}
	labels := map[string]drawioapi.Point{} // label centres, absolute
	for _, e := range laid.Edges {
		if len(e.Sections) == 0 || moved[e.Sources[0]] || moved[e.Targets[0]] {
			continue
		}
		var pts []drawioapi.Point
		for _, bp := range e.Sections[0].BendPoints {
			pts = append(pts, drawioapi.Point{X: bp.X + p.dx, Y: bp.Y + p.dy})
		}
		points[e.ID] = dropCollinear(pts)
		if len(e.Labels) > 0 && e.Labels[0].Width > 0 {
			l, c := e.Labels[0], boxes[e.Container]
			labels[e.ID] = drawioapi.Point{X: l.X + c.X + p.dx + l.Width/2, Y: l.Y + c.Y + p.dy + l.Height/2}
		}
	}

	// Rewrite: vertex boxes relative to their parent in the document,
	// edge waypoints relative to the edge's parent.
	var edits []edit
	for _, id := range p.members {
		b, ok := boxes[id]
		if !ok {
			continue
		}
		c := p.byID[id]
		s, ok := spans[id]
		if !ok || c.geo == nil {
			continue
		}
		ox, oy := p.newAbs(c.parent, boxes)
		edits = append(edits, edit{s, setBox(doc[s.start:s.end], b.X-ox, b.Y-oy, b.Width, b.Height, p.isGroup[id])})
		res.Moved++
	}
	for _, c := range cells {
		if !c.edge || !p.in[c.source] && !p.in[c.target] {
			continue
		}
		s, ok := spans[c.id]
		if !ok {
			continue
		}
		ox, oy := p.newAbs(c.parent, boxes)
		var (
			pts    []drawioapi.Point
			offset *drawioapi.Point
		)
		if edges[c.id] {
			for _, q := range points[c.id] {
				pts = append(pts, drawioapi.Point{X: q.X - ox, Y: q.Y - oy})
			}
			if at, ok := labels[c.id]; ok {
				offset = labelOffsetOn(at, boxes[c.source], points[c.id], boxes[c.target])
			}
		}
		// An edge left out of the layout, or one bent around a pinned
		// cell, loses its old waypoints: draw.io routes it.
		edits = append(edits, edit{s, setPoints(doc[s.start:s.end], pts, offset)})
	}
	if e, ok := p.legendAside(doc, spans, boxes); ok {
		edits = append(edits, e)
		res.Report = append(res.Report, "the legend moved beside the drawing: the layout reached it")
	}
	if e, ok := p.markBelow(doc, spans, boxes); ok {
		edits = append(edits, e)
	}
	res.XML = apply(doc, edits)
	res.Pinned = p.pinned
	return res, nil
}

// settle keeps every box where it is drawn, moves aside what overlaps a
// fixed or pinned node, and grows each group to hold its content.
func (p *plan) settle(doc []byte, spans map[string]geoSpan, cells []*cell, fixed map[string]bool, layout *LayoutOptions, res *RelayoutResult) *RelayoutResult {
	own := p.pinned
	p.settling = true
	boxes := map[string]drawioapi.Box{}
	for _, id := range p.members {
		boxes[id] = p.oldAbs[id]
		c := p.byID[id]
		if fixed[c.props[AttrID]] && c.props[AttrRole] == RoleNode && !p.pin[id] {
			p.pin[id] = true
			p.pinned = append(p.pinned, id)
		}
	}
	moved := p.keepPinned(boxes, layout, &RelayoutResult{})
	// Zones grow to hold their new members first, so a new node outside
	// them is cleared from the zones as they will be drawn.
	fit := func() {
		for _, id := range p.members {
			if len(p.children[id]) > 0 && !p.pinnedAbove(id) && p.parent[id] == "" {
				// The room the model emitter leaves inside a zone.
				p.fitPadded(id, boxes, zonePadding, zonePadding)
			}
		}
	}
	fit()
	// A new node clears every box but its own containers: fixed nodes of
	// other zones, other zones.
	gap := layout.NodeSpacing
	for _, id := range p.members {
		if p.pin[id] || len(p.children[id]) > 0 {
			continue
		}
		for pass := 0; pass < 50; pass++ {
			hit := false
			for _, o := range p.members {
				if o == id || p.ancestor(o, id) || p.ancestor(id, o) || !intersectsBox(boxes[id], boxes[o]) {
					continue
				}
				if !p.pin[o] && len(p.children[o]) == 0 {
					continue // two new nodes: keepPinned's siblings rule
				}
				b := boxes[id]
				p.shift(id, 0, boxes[o].Y+boxes[o].Height+gap-b.Y, boxes)
				moved[id], hit = true, true
			}
			if !hit {
				break
			}
		}
	}
	fit()
	p.pinned = own

	var edits []edit
	for _, id := range p.members {
		b, c := boxes[id], p.byID[id]
		s, ok := spans[id]
		parentMoved := false
		if pb, ok := boxes[c.parent]; ok && pb != p.oldAbs[c.parent] {
			parentMoved = true
		}
		if !ok || c.geo == nil || b == p.oldAbs[id] && !parentMoved {
			continue
		}
		ox, oy := p.newAbs(c.parent, boxes)
		edits = append(edits, edit{s, setBox(doc[s.start:s.end], b.X-ox, b.Y-oy, b.Width, b.Height, len(p.children[id]) > 0)})
		res.Moved++
	}
	for _, c := range cells {
		if !c.edge || !moved[c.source] && !moved[c.target] {
			continue
		}
		if s, ok := spans[c.id]; ok {
			edits = append(edits, edit{s, setPoints(doc[s.start:s.end], nil, nil)})
		}
	}
	if e, ok := p.legendAside(doc, spans, boxes); ok {
		edits = append(edits, e)
	}
	if e, ok := p.markBelow(doc, spans, boxes); ok {
		edits = append(edits, e)
	}
	res.XML = apply(doc, edits)
	res.Pinned = own
	return res
}

// compressed reports whether an mxfile's first diagram holds base64 text
// rather than an mxGraphModel.
func compressed(doc []byte) bool {
	i := bytes.Index(doc, []byte("<diagram"))
	if i < 0 {
		return false
	}
	j := bytes.IndexByte(doc[i:], '>')
	if j < 0 {
		return false
	}
	rest := bytes.TrimSpace(doc[i+j+1:])
	return !bytes.HasPrefix(rest, []byte("<"))
}

// plan is what a relayout moves: the member vertices and the layout tree
// they form.
type plan struct {
	byID     map[string]*cell
	in       map[string]bool     // a member vertex
	members  []string            // member ids, document order
	parent   map[string]string   // layout parent ("" = top level)
	children map[string][]string // layout children
	isGroup  map[string]bool
	oldAbs   map[string]drawioapi.Box
	pinned   []string
	pin      map[string]bool
	dx, dy   float64 // the shift that put the layout where the drawing was
	settling bool    // Settle: no layout, only moves aside
}

func newPlan(cells []*cell) *plan {
	p := &plan{byID: map[string]*cell{}, in: map[string]bool{}, parent: map[string]string{},
		children: map[string][]string{}, isGroup: map[string]bool{}, oldAbs: map[string]drawioapi.Box{}, pin: map[string]bool{}}
	for _, c := range cells {
		p.byID[c.id] = c
	}
	r := &reader{byID: p.byID}
	for _, c := range cells {
		if !c.vertex || c.geo == nil || c.parent == "0" || c.id == "0" || r.insideLegend(c) || c.props[AttrRole] == RoleDiagram {
			continue
		}
		p.in[c.id] = true
		p.members = append(p.members, c.id)
		x, y := r.abs(c)
		p.oldAbs[c.id] = drawioapi.Box{X: x, Y: y, Width: c.geo.Width, Height: c.geo.Height}
	}
	// Layout parent: the document parent when it is a member; else, for a
	// cell this engine did not write, the smallest member drawn around it
	// (a hand-drawn group). A node or zone the engine wrote belongs where
	// its document parent says, wherever it is drawn.
	for _, id := range p.members {
		c := p.byID[id]
		if p.in[c.parent] {
			p.parent[id] = c.parent
			continue
		}
		if role := c.props[AttrRole]; role == RoleNode || role == RoleZone {
			continue
		}
		b := p.oldAbs[id]
		best, area := "", math.Inf(1)
		for _, o := range p.members {
			ob := p.oldAbs[o]
			if o == id || !contains(ob, b) || ob == b {
				continue
			}
			if a := ob.Width * ob.Height; a < area {
				best, area = o, a
			}
		}
		p.parent[id] = best
	}
	// A cycle cannot happen through the document parent; guard the drawn
	// containment all the same.
	for _, id := range p.members {
		seen := map[string]bool{id: true}
		for q := p.parent[id]; q != ""; q = p.parent[q] {
			if seen[q] {
				p.parent[id] = ""
				break
			}
			seen[q] = true
		}
	}
	for _, id := range p.members {
		c := p.byID[id]
		if c.props[AttrPinned] == "1" || strings.Contains(";"+c.style, ";locked=1") {
			p.pin[id] = true
			p.pinned = append(p.pinned, id)
		}
	}
	for _, id := range p.members {
		if par := p.parent[id]; par != "" {
			p.children[par] = append(p.children[par], id)
		}
	}
	for _, id := range p.members {
		// A group is laid out inside when an edge reaches its content. A
		// pinned group, or one no edge reaches (nothing tells a layout
		// more than how it is drawn), keeps its content as drawn and is
		// laid out as one box.
		if len(p.children[id]) > 0 && !p.pinnedAbove(id) && p.linked(id) {
			p.isGroup[id] = true
		}
	}
	return p
}

// pinnedAbove reports whether id or a layout ancestor is pinned.
func (p *plan) pinnedAbove(id string) bool {
	for q := id; q != ""; q = p.parent[q] {
		if p.pin[q] {
			return true
		}
	}
	return false
}

// elkGraph builds the compound ELK graph and returns the ids of the
// edges it carries.
func (p *plan) elkGraph(layout *LayoutOptions, res *RelayoutResult) (*relNode, map[string]bool) {
	opts := map[string]string{
		"elk.algorithm":        layout.Algorithm,
		"elk.direction":        layout.Direction,
		"elk.spacing.nodeNode": fmt.Sprintf("%.0f", layout.NodeSpacing),
		"elk.layered.spacing.nodeNodeBetweenLayers": fmt.Sprintf("%.0f", layout.LayerSpacing),
		"elk.edgeRouting":                    "ORTHOGONAL",
		"elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
		"elk.hierarchyHandling":              "INCLUDE_CHILDREN",
		// The model order is the document's, which a relayout never
		// changes: ordering by drawn position instead let two nodes of
		// a layer swap places on every run.
		"elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
		// The drawing's positions are the starting point: layers and the
		// order inside them come from where things are drawn, so a laid-out
		// drawing laid out again stays as it is.
		"elk.layered.layering.strategy":                    "INTERACTIVE",
		"elk.layered.crossingMinimization.semiInteractive": "true",
		"elk.separateConnectedComponents":                  "false",
	}
	for k, v := range layout.ExtraOptions {
		opts[k] = v
	}
	root := &relNode{ID: "root", LayoutOptions: opts}
	nodes := map[string]*relNode{}
	// The top level: members with no layout parent. The content of a group
	// laid out as one box is not in the graph; it rides with the group.
	var top []string
	for _, id := range p.members {
		if p.parent[id] == "" {
			top = append(top, id)
		}
	}
	var build func(ids []string) []*relNode
	build = func(ids []string) []*relNode {
		var out []*relNode
		for _, id := range ids {
			b := p.oldAbs[id]
			n := &relNode{ID: id, X: b.X, Y: b.Y}
			if par := p.parent[id]; par != "" && p.isGroup[par] {
				n.X, n.Y = b.X-p.oldAbs[par].X, b.Y-p.oldAbs[par].Y
			}
			n.LayoutOptions = map[string]string{"elk.position": fmt.Sprintf("(%.0f,%.0f)", n.X, n.Y)}
			nodes[id] = n
			if p.isGroup[id] {
				n.LayoutOptions["elk.padding"] = containerPadding
				n.Children = build(append([]string(nil), p.children[id]...))
			} else {
				n.Width, n.Height = b.Width, b.Height
			}
			out = append(out, n)
		}
		return out
	}
	root.Children = build(top)
	if len(p.edgeCells()) == 0 {
		// A drawing without edges is packed, not layered.
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, id := range top {
			b := p.oldAbs[id]
			minX, minY = math.Min(minX, b.X), math.Min(minY, b.Y)
			maxX, maxY = math.Max(maxX, b.X+b.Width), math.Max(maxY, b.Y+b.Height)
		}
		packed(opts, drawioapi.Box{Width: maxX - minX, Height: maxY - minY}, layout.NodeSpacing)
	}

	// Partitions: groups carrying ktw-order keep their order, when every
	// top-level node has one.
	partitioned := len(root.Children) > 0
	for _, n := range root.Children {
		if _, err := strconv.Atoi(p.byID[n.ID].props[AttrOrder]); err != nil {
			partitioned = false
			break
		}
	}
	if partitioned {
		opts["org.eclipse.elk.partitioning.activate"] = "true"
		for _, n := range root.Children {
			if n.LayoutOptions == nil {
				n.LayoutOptions = map[string]string{}
			}
			n.LayoutOptions["org.eclipse.elk.partitioning.partition"] = p.byID[n.ID].props[AttrOrder]
		}
	} else {
		delete(opts, "org.eclipse.elk.partitioning.activate")
	}

	carried := map[string]bool{}
	for _, c := range p.edgeCells() {
		s, t := c.source, c.target
		if nodes[s] == nil || nodes[t] == nil {
			res.Report = append(res.Report, fmt.Sprintf("edge %q not laid out: an end is not laid out (legend, decoration or inside a pinned group)", c.id))
			continue
		}
		if s == t || p.ancestor(s, t) || p.ancestor(t, s) {
			res.Report = append(res.Report, fmt.Sprintf("edge %q not laid out: it joins a group to its own content", c.id))
			continue
		}
		e := relEdge{ID: c.id, Sources: []string{s}, Targets: []string{t}}
		// The label's size, so the layout keeps room for it.
		if w, h := drawioapi.LabelSize(c.label); w > 0 {
			e.Labels = []elkLabel_t{{ID: c.id + "-label", Text: c.id, Width: w, Height: h}}
			opts["elk.edgeLabels.placement"] = "CENTER"
		}
		root.Edges = append(root.Edges, e)
		carried[c.id] = true
	}
	return root, carried
}

// linked reports whether an edge ends inside group id.
func (p *plan) linked(id string) bool {
	for _, c := range p.edgeCells() {
		if p.ancestor(id, c.source) || p.ancestor(id, c.target) {
			return true
		}
	}
	return false
}

// packed sets the options that pack a graph without edges: rectangle
// packing to the proportions of box, in the drawing's order.
func packed(opts map[string]string, box drawioapi.Box, spacing float64) {
	opts["elk.algorithm"] = "rectpacking"
	opts["elk.hierarchyHandling"] = "SEPARATE_CHILDREN"
	if box.Height > 0 {
		opts["elk.aspectRatio"] = strconv.FormatFloat(box.Width/box.Height, 'f', 2, 64)
	}
	if spacing > 0 {
		opts["elk.spacing.nodeNode"] = fmt.Sprintf("%.0f", spacing/2)
	}
}

func (p *plan) edgeCells() []*cell {
	var out []*cell
	for _, c := range p.byID {
		if c.edge {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

func (p *plan) ancestor(a, b string) bool {
	for q := p.parent[b]; q != ""; q = p.parent[q] {
		if q == a {
			return true
		}
	}
	return false
}

// collectBoxes turns ELK's parent-relative positions into page boxes.
func collectBoxes(ns []*relNode, ox, oy float64, out map[string]drawioapi.Box) {
	for _, n := range ns {
		x, y := ox+n.X, oy+n.Y
		out[n.ID] = drawioapi.Box{X: x, Y: y, Width: n.Width, Height: n.Height}
		collectBoxes(n.Children, x, y, out)
	}
}

// anchor shifts the layout so its top-left corner is where the drawing's
// was, and places what rides with a pinned group.
func (p *plan) anchor(boxes map[string]drawioapi.Box) {
	oldMinX, oldMinY := math.Inf(1), math.Inf(1)
	newMinX, newMinY := math.Inf(1), math.Inf(1)
	for id, b := range boxes {
		o := p.oldAbs[id]
		oldMinX, oldMinY = math.Min(oldMinX, o.X), math.Min(oldMinY, o.Y)
		newMinX, newMinY = math.Min(newMinX, b.X), math.Min(newMinY, b.Y)
	}
	p.dx, p.dy = oldMinX-newMinX, oldMinY-newMinY
	for id, b := range boxes {
		boxes[id] = drawioapi.Box{X: b.X + p.dx, Y: b.Y + p.dy, Width: b.Width, Height: b.Height}
	}
}

// keepPinned puts every pinned cell back in its box, moves aside what the
// layout put under it and grows the groups that must hold it. It returns
// the cells whose edges lose their ELK route.
func (p *plan) keepPinned(boxes map[string]drawioapi.Box, layout *LayoutOptions, res *RelayoutResult) map[string]bool {
	moved := map[string]bool{}
	if len(p.pinned) == 0 {
		return moved
	}
	gap := layout.NodeSpacing
	if gap == 0 {
		gap = 40
	}
	for _, id := range p.pinned {
		if _, ok := boxes[id]; !ok {
			continue // under a pinned group: stays as drawn
		}
		old := p.oldAbs[id]
		cur := boxes[id]
		p.shift(id, old.X-cur.X, old.Y-cur.Y, boxes)
		moved[id] = true
	}
	// Move aside the free cells a pinned one covers: siblings in the same
	// layout parent, along the reading direction.
	horizontal := layout.Direction != "DOWN" && layout.Direction != "UP"
	for pass := 0; pass < 3; pass++ {
		changed := false
		for _, pid := range p.pinned {
			pb, ok := boxes[pid]
			if !ok {
				continue
			}
			for id, b := range boxes {
				if id == pid || p.pin[id] || p.ancestor(id, pid) || p.ancestor(pid, id) || p.parent[id] != p.parent[pid] || !intersectsBox(b, pb) {
					continue
				}
				if horizontal {
					p.shift(id, 0, pb.Y+pb.Height+gap-b.Y, boxes)
				} else {
					p.shift(id, pb.X+pb.Width+gap-b.X, 0, boxes)
				}
				moved[id] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// Groups grow to hold what was put back or moved (settle fits them
	// its own way).
	for _, id := range p.members {
		if !p.isGroup[id] || p.settling {
			continue
		}
		p.fit(id, boxes)
	}
	res.Report = append(res.Report, fmt.Sprintf("%d pinned cell(s) kept in place", len(p.pinned)))
	return moved
}

// ride places what was not laid out (the content of a group kept as
// drawn) by the move of its nearest placed ancestor: everything under one
// laid-out box moves with it.
func (p *plan) ride(boxes map[string]drawioapi.Box) {
	for _, id := range p.members {
		if _, ok := boxes[id]; ok {
			continue
		}
		for q := p.parent[id]; q != ""; q = p.parent[q] {
			if nb, ok := boxes[q]; ok {
				ob, b := p.oldAbs[q], p.oldAbs[id]
				boxes[id] = drawioapi.Box{X: b.X + nb.X - ob.X, Y: b.Y + nb.Y - ob.Y, Width: b.Width, Height: b.Height}
				break
			}
		}
	}
}

func (p *plan) shift(id string, dx, dy float64, boxes map[string]drawioapi.Box) {
	if b, ok := boxes[id]; ok {
		boxes[id] = drawioapi.Box{X: b.X + dx, Y: b.Y + dy, Width: b.Width, Height: b.Height}
	}
	for _, c := range p.children[id] {
		p.shift(c, dx, dy, boxes)
	}
}

// fit grows a group (children first) to hold its children with the
// room ELK leaves inside a group.
func (p *plan) fit(id string, boxes map[string]drawioapi.Box) { p.fitPadded(id, boxes, 45, 25) }

// fitPadded grows a group (children first) so each child sits at least
// top / side inside it; it never shrinks a group.
func (p *plan) fitPadded(id string, boxes map[string]drawioapi.Box, top, side float64) {
	b, ok := boxes[id]
	if !ok {
		return
	}
	for _, c := range p.children[id] {
		if p.isGroup[c] || len(p.children[c]) > 0 {
			p.fitPadded(c, boxes, top, side)
		}
		cb, ok := boxes[c]
		if !ok {
			continue
		}
		if p.settling {
			// Settling keeps a group's box and grows only the sides its
			// content crosses: lanes drawn flush with their pool stay so,
			// and a pool re-padded all round would climb over its
			// neighbour.
			minX, minY, maxX, maxY := b.X, b.Y, b.X+b.Width, b.Y+b.Height
			if cb.X < minX {
				minX = cb.X - side
			}
			if cb.Y < minY {
				minY = cb.Y - top
			}
			if cb.X+cb.Width > maxX {
				maxX = cb.X + cb.Width + side
			}
			if cb.Y+cb.Height > maxY {
				maxY = cb.Y + cb.Height + side
			}
			b = drawioapi.Box{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
			continue
		}
		minX, minY := math.Min(b.X, cb.X-side), math.Min(b.Y, cb.Y-top)
		maxX, maxY := math.Max(b.X+b.Width, cb.X+cb.Width+side), math.Max(b.Y+b.Height, cb.Y+cb.Height+side)
		b = drawioapi.Box{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
	}
	boxes[id] = b
}

// newAbs is the page position of a document parent after the layout: a
// laid-out member's new box, else its old one (a layer is at 0,0).
func (p *plan) newAbs(parent string, boxes map[string]drawioapi.Box) (float64, float64) {
	if b, ok := boxes[parent]; ok {
		return b.X, b.Y
	}
	c := p.byID[parent]
	if c == nil || !c.vertex {
		return 0, 0
	}
	if b, ok := p.oldAbs[parent]; ok {
		return b.X, b.Y
	}
	x, y := (&reader{byID: p.byID}).abs(c)
	return x, y
}

// dropCollinear removes the waypoints that do not turn: ELK splits an
// edge where it crosses a group's border.
func dropCollinear(pts []drawioapi.Point) []drawioapi.Point {
	var out []drawioapi.Point
	for i, q := range pts {
		if len(out) > 0 && math.Abs(out[len(out)-1].X-q.X) < 0.5 && math.Abs(out[len(out)-1].Y-q.Y) < 0.5 {
			continue // duplicate
		}
		if i+1 < len(pts) && len(out) > 0 {
			a, b := out[len(out)-1], pts[i+1]
			if math.Abs((q.X-a.X)*(b.Y-a.Y)-(q.Y-a.Y)*(b.X-a.X)) < 0.5 {
				continue
			}
		}
		out = append(out, q)
	}
	return out
}

func intersectsBox(a, b drawioapi.Box) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func contains(outer, inner drawioapi.Box) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.Width <= outer.X+outer.Width && inner.Y+inner.Height <= outer.Y+outer.Height
}

// legendAside moves the legend to the right of the laid-out drawing when
// the layout put something under it.
func (p *plan) legendAside(doc []byte, spans map[string]geoSpan, boxes map[string]drawioapi.Box) (edit, bool) {
	var legend *cell
	for _, c := range p.byID {
		// The legend's own cell: its parts are its children.
		if pc := p.byID[c.parent]; c.props[AttrRole] == RoleLegend && c.geo != nil && (pc == nil || !pc.vertex) {
			legend = c
			break
		}
	}
	if legend == nil {
		return edit{}, false
	}
	lx, ly := (&reader{byID: p.byID}).abs(legend)
	lb := drawioapi.Box{X: lx, Y: ly, Width: legend.geo.Width, Height: legend.geo.Height}
	hit := false
	maxX, minY := math.Inf(-1), math.Inf(1)
	for _, b := range boxes {
		hit = hit || intersectsBox(b, lb)
		maxX, minY = math.Max(maxX, b.X+b.Width), math.Min(minY, b.Y)
	}
	s, ok := spans[legend.id]
	if !hit || !ok {
		return edit{}, false
	}
	ox, oy := p.newAbs(legend.parent, boxes)
	return edit{s, setBox(doc[s.start:s.end], maxX+40-ox, minY-oy, 0, 0, false)}, true
}

// markBelow puts the production mark back under the laid-out drawing,
// left-aligned with it, wherever the layout left it.
func (p *plan) markBelow(doc []byte, spans map[string]geoSpan, boxes map[string]drawioapi.Box) (edit, bool) {
	var mark *cell
	for _, c := range p.byID {
		if c.props[AttrRole] == RoleMark && c.geo != nil {
			mark = c
			break
		}
	}
	if mark == nil || len(boxes) == 0 {
		return edit{}, false
	}
	s, ok := spans[mark.id]
	if !ok {
		return edit{}, false
	}
	minX, maxY := math.Inf(1), math.Inf(-1)
	for _, b := range boxes {
		minX, maxY = math.Min(minX, b.X), math.Max(maxY, b.Y+b.Height)
	}
	ox, oy := p.newAbs(mark.parent, boxes)
	return edit{s, setBox(doc[s.start:s.end], minX-ox, maxY+markGap-oy, 0, 0, false)}, true
}

// ─── byte-level rewriting ─────────────────────────────────────────────

type edit struct {
	span geoSpan
	with []byte
}

func apply(doc []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].span.start < edits[j].span.start })
	var out bytes.Buffer
	at := 0
	for _, e := range edits {
		out.Write(doc[at:e.span.start])
		out.Write(e.with)
		at = e.span.end
	}
	out.Write(doc[at:])
	return out.Bytes()
}

// scanGeometry finds each cell's mxGeometry element in the first page.
func scanGeometry(doc []byte) (map[string]geoSpan, error) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	spans := map[string]geoSpan{}
	var ids []string // enclosing cell ids
	inRoot := false
	depthAtGeo, depth := -1, 0
	var geoStart int
	var geoOwner string
	for {
		start := int(dec.InputOffset())
		tok, err := dec.RawToken()
		if err == io.EOF {
			return spans, nil
		}
		if err != nil {
			return nil, &drawioapi.DrawioError{Message: "invalid XML: " + err.Error()}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if !inRoot {
				inRoot = t.Name.Local == "root"
				continue
			}
			id := ""
			for _, a := range t.Attr {
				if a.Name.Local == "id" {
					id = a.Value
				}
			}
			if id == "" && len(ids) > 0 {
				id = ids[len(ids)-1]
			}
			ids = append(ids, id)
			if t.Name.Local == "mxGeometry" && depthAtGeo < 0 {
				depthAtGeo, geoStart, geoOwner = depth, start, id
			}
		case xml.EndElement:
			if inRoot && t.Name.Local == "root" {
				return spans, nil
			}
			if depth == depthAtGeo {
				if _, seen := spans[geoOwner]; !seen {
					spans[geoOwner] = geoSpan{geoStart, int(dec.InputOffset())}
				}
				depthAtGeo = -1
			}
			if inRoot && len(ids) > 0 {
				ids = ids[:len(ids)-1]
			}
			depth--
		}
	}
}

var (
	geoOpenRe = regexp.MustCompile(`^<mxGeometry\b[^>]*?(/?)>`)
	pointsRe  = regexp.MustCompile(`(?s)<Array\b[^>]*\bas="points"[^>]*?(?:/>|>.*?</Array>)`)
)

// setBox rewrites a vertex geometry's x, y (and, for a group, width and
// height); the rest of the element is kept.
func setBox(geo []byte, x, y, w, h float64, resize bool) []byte {
	m := geoOpenRe.FindSubmatchIndex(geo)
	if m == nil {
		return geo
	}
	tag := string(geo[:m[1]])
	tag = setAttr(tag, "x", x)
	tag = setAttr(tag, "y", y)
	if resize {
		tag = setAttr(tag, "width", w)
		tag = setAttr(tag, "height", h)
	}
	return append([]byte(tag), geo[m[1]:]...)
}

func setAttr(tag, name string, v float64) string {
	val := strconv.FormatFloat(math.Round(v), 'f', -1, 64)
	re := regexp.MustCompile(`\s` + name + `="[^"]*"`)
	if re.MatchString(tag) {
		return re.ReplaceAllString(tag, ` `+name+`="`+val+`"`)
	}
	return strings.Replace(tag, "<mxGeometry", `<mxGeometry `+name+`="`+val+`"`, 1)
}

// labelOffsetRe matches an edge label's offset: a relaid edge's label
// goes back to the middle of the edge.
var labelOffsetRe = regexp.MustCompile(`<mxPoint\b[^>]*\bas="offset"[^>]*/>`)

// labelOffsetOn is a label's offset from the middle of its edge: the
// path from the source's centre through the bend points to the target's.
func labelOffsetOn(at drawioapi.Point, src drawioapi.Box, bends []drawioapi.Point, tgt drawioapi.Box) *drawioapi.Point {
	path := []drawioapi.Point{{X: src.X + src.Width/2, Y: src.Y + src.Height/2}}
	path = append(path, bends...)
	path = append(path, drawioapi.Point{X: tgt.X + tgt.Width/2, Y: tgt.Y + tgt.Height/2})
	mid := drawioapi.PolylineMidpoint(path)
	return &drawioapi.Point{X: at.X - mid.X, Y: at.Y - mid.Y}
}

// setPoints replaces an edge geometry's waypoints (none removes them) and
// its label offset (the old one belonged to the old route; nil puts the
// label back in the middle of the edge).
func setPoints(geo []byte, pts []drawioapi.Point, offset *drawioapi.Point) []byte {
	geo = labelOffsetRe.ReplaceAll(geo, nil)
	var arr string
	round := func(v float64) string { return strconv.FormatFloat(math.Round(v), 'f', -1, 64) }
	if len(pts) > 0 {
		var b strings.Builder
		b.WriteString(`<Array as="points">`)
		for _, q := range pts {
			fmt.Fprintf(&b, `<mxPoint x="%s" y="%s" />`, round(q.X), round(q.Y))
		}
		b.WriteString(`</Array>`)
		arr = b.String()
	}
	if offset != nil {
		arr += fmt.Sprintf(`<mxPoint x="%s" y="%s" as="offset" />`, round(offset.X), round(offset.Y))
	}
	if loc := pointsRe.FindIndex(geo); loc != nil {
		return append(append(append([]byte(nil), geo[:loc[0]]...), arr...), geo[loc[1]:]...)
	}
	if arr == "" {
		return geo
	}
	m := geoOpenRe.FindSubmatchIndex(geo)
	if m == nil {
		return geo
	}
	if m[3] > m[2] { // self-closing: open it
		open := strings.TrimRight(string(geo[:m[2]]), " ") + ">"
		return []byte(open + arr + "</mxGeometry>")
	}
	return append(append(append([]byte(nil), geo[:m[1]]...), arr...), geo[m[1]:]...)
}

// Place moves the model's cells of a document this engine wrote to the
// given page boxes (by model id: nodes and zones), keeping each cell's
// size unless it is a zone, and marks the pinned ids ktw-pinned="1". The
// content of a zone is placed relative to the zone's new box. Cells with
// no box keep theirs.
func Place(data []byte, boxes map[string]drawioapi.Box, pinned []string) ([]byte, error) {
	cells, err := parseCells(data)
	if err != nil {
		return nil, err
	}
	spans, err := scanGeometry(data)
	if err != nil {
		return nil, err
	}
	pin := map[string]bool{}
	for _, id := range pinned {
		pin[id] = true
	}
	byID := map[string]*cell{}
	for _, c := range cells {
		byID[c.id] = c
	}
	r := &reader{byID: byID}
	// The new page position of every placed cell, by cell id.
	abs := map[string]drawioapi.Box{}
	for _, c := range cells {
		if b, ok := boxes[c.props[AttrID]]; ok && c.geo != nil && (c.props[AttrRole] == RoleNode || c.props[AttrRole] == RoleZone) {
			if c.props[AttrRole] == RoleNode {
				b.Width, b.Height = c.geo.Width, c.geo.Height
			}
			abs[c.id] = b
		}
	}
	var edits []edit
	for _, c := range cells {
		b, ok := abs[c.id]
		if !ok {
			continue
		}
		s, ok := spans[c.id]
		if !ok {
			continue
		}
		var ox, oy float64
		if p, ok := abs[c.parent]; ok {
			ox, oy = p.X, p.Y
		} else if p := byID[c.parent]; p != nil && p.vertex {
			ox, oy = r.abs(p)
		}
		edits = append(edits, edit{s, setBox(data[s.start:s.end], b.X-ox, b.Y-oy, b.Width, b.Height, c.props[AttrRole] == RoleZone)})
	}
	out := apply(data, edits)
	if len(pin) == 0 {
		return out, nil
	}
	// Pins: an attribute on the cell's UserObject.
	for _, c := range cells {
		if !pin[c.props[AttrID]] || c.props[AttrRole] != RoleNode || c.props[AttrPinned] == "1" {
			continue
		}
		// identity.go writes the role, then the id.
		marker := []byte(AttrRole + `="` + RoleNode + `" ` + AttrID + `="` + xmlAttrEscape(c.props[AttrID]) + `"`)
		if i := bytes.Index(out, marker); i >= 0 {
			ins := []byte(` ` + AttrPinned + `="1"`)
			out = append(out[:i+len(marker)], append(ins, out[i+len(marker):]...)...)
		}
	}
	return out, nil
}

func xmlAttrEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
