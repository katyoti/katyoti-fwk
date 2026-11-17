// SPDX-License-Identifier: Apache-2.0

// render.go — the pipeline: layout profile, node sizing from templates,
// ELK layout, page frame, then template instantiation for every zone,
// element, relationship and the legend.

package archdiagram

import (
	"context"
	"fmt"
	"strings"
	"time"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/template"
)

// fallbackW and fallbackH size an element no template draws.
const fallbackW, fallbackH = 180.0, 60.0

// Render lays out a diagram and draws it with its viewpoint's templates.
// It returns the draw.io XML and the problems met while filling templates
// (a binding or a condition that could not be evaluated, a kind with no
// template); the diagram still renders around them.
func Render(ctx context.Context, d *ArchDiagram, dr *drawioapi.Drawing, lib *template.Library, noLayout bool) ([]byte, []string, error) {
	if d == nil || dr == nil || lib == nil {
		return nil, nil, fmt.Errorf("render: nil diagram, drawing or library")
	}
	// The layout is the translator's projection (the diagram's hints over
	// the viewpoint's profile); defaults fill what both left empty.
	d.ApplyDefaults()
	for _, n := range d.Nodes {
		n.Width, n.Height = fallbackW, fallbackH
		if t := lib.Element(n.Type); t != nil && t.Width > 0 && t.Height > 0 {
			n.Width, n.Height = lib.Size(t, template.NewScope(n.ID, n.Label, n.Type, n.Data, lib.Body.Constants))
		}
	}
	// Edge labels are sized before the layout, which keeps room for them.
	for _, ed := range d.Edges {
		if t := lib.Relationship(ed.EdgeStyle); t != nil {
			ed.LabelW, ed.LabelH = drawioapi.LabelSize(lib.EdgeLabel(t, template.NewScope(ed.ID, ed.Label, ed.EdgeStyle, ed.Data, lib.Body.Constants)))
		}
	}
	if noLayout {
		// Positions are the caller's; zones still need their boxes.
		computeZoneBounds(d)
	} else if d.Layout.Lanes {
		if err := LayoutLanes(ctx, d); err != nil {
			return nil, nil, fmt.Errorf("layout: %w", err)
		}
	} else if err := RunELKLayout(ctx, d); err != nil {
		return nil, nil, fmt.Errorf("layout: %w", err)
	}

	e := &emitter{d: d, dr: dr, lib: lib}
	var page *template.Page
	name := dr.Viewpoint.Page
	if d.Page != nil && d.Page.Name != "" {
		name = d.Page.Name
	}
	if name != "" {
		if page = lib.Page(name); page == nil {
			e.problems = append(e.problems, fmt.Sprintf("viewpoint %s: template library %s has no page template %q", dr.Viewpoint.ID, lib.Body.ID, name))
		} else if page.Content != nil {
			shift(d, page.Content.X, page.Content.Y)
		}
	}
	return e.emit(page), e.problems, nil
}

// shift moves the whole drawing by (dx, dy): into a page template's
// content slot.
func shift(d *ArchDiagram, dx, dy float64) {
	for _, n := range d.Nodes {
		n.X += dx
		n.Y += dy
	}
	for _, z := range d.Zones {
		if z.Bounds != nil {
			z.Bounds.X += dx
			z.Bounds.Y += dy
		}
	}
	for _, e := range d.Edges {
		for i := range e.BendPoints {
			e.BendPoints[i].X += dx
			e.BendPoints[i].Y += dy
		}
		if e.LabelAt != nil {
			e.LabelAt.X += dx
			e.LabelAt.Y += dy
		}
	}
}

type emitter struct {
	d        *ArchDiagram
	dr       *drawioapi.Drawing
	lib      *template.Library
	problems []string
}

func (e *emitter) emit(page *template.Page) []byte {
	var b strings.Builder
	writeHeader(&b, e.d)
	if page != nil {
		vars := map[string]string{
			"title": e.d.Title,
			"date":  time.Now().Format("2006-01-02"),
		}
		if e.d.Page != nil {
			for k, v := range e.d.Page.Values {
				vars[k] = v
			}
		}
		in := e.lib.InstantiatePage(page, vars, []template.Attr{{Name: AttrRole, Value: RoleDecoration}})
		e.problems = append(e.problems, in.Problems...)
		in.Write(&b, 4)
	}
	zoneCells := e.emitZones(&b)
	constants := e.lib.Body.Constants

	for _, n := range e.d.Nodes {
		parent, x, y := "1", n.X, n.Y
		if zc, ok := zoneCells[n.Zone]; ok {
			parent = zc
			for _, z := range e.d.Zones {
				if z.ID == n.Zone && z.Bounds != nil {
					x -= z.Bounds.X
					y -= z.Bounds.Y
				}
			}
		}
		identity := identityAttrs(RoleNode, n.ID, n.Type, n.Label, n.Data)
		if n.Link != "" {
			identity = append(identity, attr{"link", n.Link})
		}
		t := e.lib.Element(n.Type)
		if t == nil {
			e.problems = append(e.problems, fmt.Sprintf("node %q: template library %s has no template for kind %q nor a default (*)", n.ID, e.lib.Body.ID, n.Type))
			writeObjectCell(&b, NodeCellID(n.ID), n.Label, identity,
				fmt.Sprintf(`style="rounded=0;whiteSpace=wrap;html=1;fillColor=#FFFFFF;strokeColor=#999999;" vertex="1" parent="%s"`, xmlAttr(parent)),
				fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`, x, y, n.Width, n.Height))
			continue
		}
		in := e.lib.Instantiate(t, template.NewScope(n.ID, n.Label, n.Type, n.Data, constants), template.Placement{
			CellID: NodeCellID(n.ID), ParentID: parent, X: x, Y: y, Width: n.Width, Height: n.Height,
			Identity: toTemplateAttrs(identity),
			PartRole: []template.Attr{{Name: AttrRole, Value: RoleDecoration}},
		}, nil)
		e.problems = append(e.problems, in.Problems...)
		in.Write(&b, 4)
	}

	nodeSet := make(map[string]bool, len(e.d.Nodes))
	for _, n := range e.d.Nodes {
		nodeSet[n.ID] = true
	}
	for _, ed := range e.d.Edges {
		if !nodeSet[ed.Source] || !nodeSet[ed.Target] {
			continue
		}
		identity := identityAttrs(RoleEdge, ed.ID, ed.EdgeStyle, ed.Label, ed.Data)
		points := make([]drawioapi.Point, 0, len(ed.BendPoints))
		for _, p := range ed.BendPoints {
			points = append(points, drawioapi.Point{X: p.X, Y: p.Y})
		}
		t := e.lib.Relationship(ed.EdgeStyle)
		if t == nil {
			e.problems = append(e.problems, fmt.Sprintf("edge %q: template library %s has no template for relationship %q", ed.ID, e.lib.Body.ID, ed.EdgeStyle))
			writeObjectCell(&b, EdgeCellID(ed.ID), ed.Label, identity,
				fmt.Sprintf(`style="edgeStyle=orthogonalEdgeStyle;rounded=1;html=1;endArrow=classic;strokeColor=#546E7A;" edge="1" source="%s" target="%s" parent="1"`,
					xmlAttr(NodeCellID(ed.Source)), xmlAttr(NodeCellID(ed.Target))),
				`<mxGeometry relative="1" as="geometry"/>`)
			continue
		}
		in := e.lib.Instantiate(t, template.NewScope(ed.ID, ed.Label, ed.EdgeStyle, ed.Data, constants), template.Placement{
			CellID: EdgeCellID(ed.ID), ParentID: "1",
			SourceID: NodeCellID(ed.Source), TargetID: NodeCellID(ed.Target), Points: points,
			LabelOffset: e.labelOffset(ed),
			Identity:    toTemplateAttrs(identity),
			PartRole:    []template.Attr{{Name: AttrRole, Value: RoleDecoration}},
		}, nil)
		e.problems = append(e.problems, in.Problems...)
		in.Write(&b, 4)
	}

	var slot *drawioapi.Box
	if page != nil {
		slot = page.Legend
	}
	e.emitMark(&b, e.emitLegend(&b, slot))
	b.WriteString(drawioFooter)
	return []byte(b.String())
}

// labelOffset is where the layout put an edge's label, relative to the
// middle of the edge (where draw.io puts it by default).
func (e *emitter) labelOffset(ed *Edge) *drawioapi.Point {
	if ed.LabelAt == nil {
		return nil
	}
	var path []drawioapi.Point
	if len(ed.BendPoints) >= 2 {
		for _, p := range ed.BendPoints {
			path = append(path, drawioapi.Point{X: p.X, Y: p.Y})
		}
	} else {
		for _, n := range e.d.Nodes {
			if n.ID == ed.Source {
				path = append([]drawioapi.Point{{X: n.X + n.Width/2, Y: n.Y + n.Height/2}}, path...)
			}
			if n.ID == ed.Target {
				path = append(path, drawioapi.Point{X: n.X + n.Width/2, Y: n.Y + n.Height/2})
			}
		}
	}
	if len(path) < 2 {
		return nil
	}
	mid := drawioapi.PolylineMidpoint(path)
	return &drawioapi.Point{X: ed.LabelAt.X - mid.X, Y: ed.LabelAt.Y - mid.Y}
}

func toTemplateAttrs(in []attr) []template.Attr {
	out := make([]template.Attr, len(in))
	for i, a := range in {
		out[i] = template.Attr{Name: a.name, Value: a.value}
	}
	return out
}

// zoneStyle draws a zone the library has no zone template for.
const zoneStyle = "rounded=1;whiteSpace=wrap;html=1;container=1;collapsible=0;fillColor=#f5f5f5;strokeColor=#666666;dashed=1;dashPattern=8 4;fontSize=13;fontStyle=1;verticalAlign=top;align=center;spacingTop=5;arcSize=8;"

// emitZones draws each zone container around its members, a zone inside
// its parent zone. The library's `zone` template draws a top-level zone
// and its `zone:nested` template (else `zone`) a zone inside another. It
// returns the cell id of every zone it drew, by zone id.
func (e *emitter) emitZones(b *strings.Builder) map[string]string {
	d := e.d
	cells := make(map[string]string, len(d.Zones))
	byID := map[string]*Zone{}
	for _, z := range d.Zones {
		byID[z.ID] = z
	}
	var emit func(z *Zone)
	emit = func(z *Zone) {
		if _, done := cells[z.ID]; done || z.Bounds == nil {
			return
		}
		parent, x, y := "1", z.Bounds.X, z.Bounds.Y
		if p := byID[z.Parent]; p != nil {
			emit(p)
			if pc, ok := cells[p.ID]; ok {
				parent, x, y = pc, x-p.Bounds.X, y-p.Bounds.Y
			}
		}
		cell := ZoneCellID(z.ID)
		cells[z.ID] = cell
		obj := []attr{{AttrRole, RoleZone}, {AttrID, z.ID}, {AttrOrder, fmt.Sprint(z.Order)}}
		if z.Parent != "" {
			obj = append(obj, attr{AttrParentZone, z.Parent})
		}
		if z.Kind != "" {
			obj = append(obj, attr{AttrKind, z.Kind})
		}
		t := e.lib.Zone(z.Kind, z.Parent != "")
		if z.Kind != "" && !e.lib.HasZoneKind(z.Kind) {
			e.problems = append(e.problems, fmt.Sprintf("zone %q: template library %s has no zone:%s template", z.ID, e.lib.Body.ID, z.Kind))
		}
		if t == nil {
			writeObjectCell(b, cell, z.Label, obj,
				fmt.Sprintf(`style="%s" vertex="1" parent="%s"`, zoneStyle, xmlAttr(parent)),
				fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`,
					x, y, z.Bounds.Width, z.Bounds.Height))
			return
		}
		in := e.lib.Instantiate(t, template.NewScope(z.ID, z.Label, z.Kind, nil, e.lib.Body.Constants), template.Placement{
			CellID: cell, ParentID: parent, X: x, Y: y, Width: z.Bounds.Width, Height: z.Bounds.Height,
			Identity: toTemplateAttrs(obj),
			PartRole: []template.Attr{{Name: AttrRole, Value: RoleDecoration}},
		}, nil)
		e.problems = append(e.problems, in.Problems...)
		in.Write(b, 4)
	}
	for _, z := range d.Zones {
		emit(z)
	}
	return cells
}
