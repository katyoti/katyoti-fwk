// SPDX-License-Identifier: Apache-2.0

// reader.go — a .drawio file read back into the model. A diagram this
// engine wrote carries its identity on every cell (identity.go) and reads
// back into the LogicalDiagram it came from, with what a person changed
// in draw.io; a foreign diagram reads into vertices, edges and
// containment.

package archdiagram

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

type rawPoint struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

type rawGeo struct {
	X      float64 `xml:"x,attr"`
	Y      float64 `xml:"y,attr"`
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	Array  *struct {
		Points []rawPoint `xml:"mxPoint"`
	} `xml:"Array"`
	// Named points: an edge's label offset (as="offset"), its loose ends.
	Named []struct {
		rawPoint
		As string `xml:"as,attr"`
	} `xml:"mxPoint"`
}

type rawInner struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Geo   *rawGeo    `xml:"mxGeometry"`
}

type rawCell struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Geo     *rawGeo    `xml:"mxGeometry"`
	Inner   *rawInner  `xml:"mxCell"`
}

// cell is one mxCell with its UserObject properties folded in.
type cell struct {
	id, parent, label, style string
	vertex, edge             bool
	hidden                   bool // visible="0"
	source, target           string
	geo                      *rawGeo
	props                    map[string]string
	order                    int // document order
}

// Read parses a .drawio document (plain or compressed, first page) and
// returns the model it draws.
func Read(data []byte) (*drawioapi.ReadResult, error) {
	cells, err := parseCells(data)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*cell, len(cells))
	for _, c := range cells {
		byID[c.id] = c
	}
	identity := false
	for _, c := range cells {
		if c.props[AttrRole] != "" {
			identity = true
			break
		}
	}
	r := &reader{byID: byID, cells: cells, res: &drawioapi.ReadResult{
		Diagram:    &drawioapi.LogicalDiagram{},
		Geometry:   map[string]drawioapi.Box{},
		EdgePoints: map[string][]drawioapi.Point{},
		EdgeLabels: map[string]drawioapi.Box{},
		Foreign:    !identity,
	}}
	if identity {
		r.readIdentity()
	} else {
		r.readForeign()
	}
	sort.Strings(r.res.Pinned)
	r.drawn()
	return r.res, nil
}

// drawn lists every painted cell with its box in page coordinates, for
// the layout check.
func (r *reader) drawn() {
	for _, c := range r.cells {
		if c.id == "0" || c.parent == "0" || c.parent == "" {
			continue // the root and the layers
		}
		d := drawioapi.DrawnCell{ID: c.id, Parent: c.parent, Label: c.label, Style: c.style, Vertex: c.vertex, Edge: c.edge}
		for p := c; p != nil; p = r.byID[p.parent] {
			if p.hidden || drawioapi.ParseStyle(p.style)["opacity"] == "0" {
				d.Hidden = true
				break
			}
			if p.parent == p.id {
				break
			}
		}
		switch {
		case c.edge:
			if id, ok := r.edgeOf[c.id]; ok {
				d.Box = r.res.EdgeLabels[id]
			}
		case c.vertex:
			if p := r.byID[c.parent]; p != nil && p.edge {
				d.OnEdge = true
			} else if c.geo != nil {
				x, y := r.abs(c)
				d.Box = drawioapi.Box{X: x, Y: y, Width: c.geo.Width, Height: c.geo.Height}
			}
		default:
			continue
		}
		r.res.Drawn = append(r.res.Drawn, d)
	}
}

type reader struct {
	byID  map[string]*cell
	cells []*cell
	res   *drawioapi.ReadResult
	// modelID maps a cell id to the model id it stands for.
	modelID map[string]string
	// zoneOf maps a zone's cell id to its model id.
	zoneOf map[string]string
	// edgeOf maps an edge's cell id to its model id.
	edgeOf map[string]string
}

func (r *reader) readIdentity() {
	d := r.res.Diagram
	if root := r.byID["0"]; root != nil {
		d.Title = root.props[AttrTitle]
		d.Viewpoint = root.props[AttrViewpoint]
		if raw := root.props[AttrLayout]; raw != "" {
			var h drawioapi.LayoutHints
			if json.Unmarshal([]byte(raw), &h) == nil {
				d.LayoutHints = &h
			}
		}
		if raw := root.props[AttrPage]; raw != "" {
			var p drawioapi.PageRef
			if json.Unmarshal([]byte(raw), &p) == nil {
				d.Page = &p
			}
		}
	}
	r.modelID = map[string]string{}
	r.zoneOf = map[string]string{}
	for _, c := range r.cells {
		if c.props[AttrRole] == RoleZone {
			id := c.props[AttrID]
			order, _ := strconv.Atoi(c.props[AttrOrder])
			d.Zones = append(d.Zones, drawioapi.LogicalZone{ID: id, Label: plainText(c.label), Order: order, Parent: c.props[AttrParentZone], Kind: c.props[AttrKind]})
			r.zoneOf[c.id] = id
			r.modelID[c.id] = id
			r.box(id, c)
		}
	}
	for _, c := range r.cells {
		switch c.props[AttrRole] {
		case RoleNode:
			id := c.props[AttrID]
			r.modelID[c.id] = id
			label := c.props[AttrLabel]
			if label == "" {
				label = plainText(c.label)
			}
			d.Nodes = append(d.Nodes, drawioapi.LogicalNode{
				ID: id, Label: label, Kind: c.props[AttrKind],
				Attributes: readAttributes(c.props), Zone: r.zoneOf[c.parent],
			})
			r.box(id, c)
			r.pin(id, c)
		case "":
			if c.vertex && c.id != "0" && c.id != "1" && c.parent != "0" && !r.insideLegend(c) {
				id := sanitizeID(c.id)
				r.modelID[c.id] = id
				d.Nodes = append(d.Nodes, drawioapi.LogicalNode{
					ID: id, Label: plainText(c.label), Kind: drawioapi.KindUnknown, Zone: r.zoneOf[c.parent],
				})
				r.box(id, c)
				r.pin(id, c)
				r.res.Report = append(r.res.Report, fmt.Sprintf("cell %q was added by hand: read as node %q of kind %q", c.id, id, drawioapi.KindUnknown))
			}
		}
	}
	for _, c := range r.cells {
		if !c.edge || r.insideLegend(c) {
			continue
		}
		role := c.props[AttrRole]
		if role != RoleEdge && role != "" {
			continue
		}
		src, sok := r.modelID[c.source]
		tgt, tok := r.modelID[c.target]
		if !sok || !tok {
			r.res.Report = append(r.res.Report, fmt.Sprintf("edge cell %q dropped: an end is not a node", c.id))
			continue
		}
		e := drawioapi.LogicalEdge{Source: src, Target: tgt}
		if role == RoleEdge {
			e.ID, e.Kind, e.Label, e.Attributes = c.props[AttrID], c.props[AttrKind], c.props[AttrLabel], readAttributes(c.props)
		} else {
			e.ID, e.Kind, e.Label = sanitizeID(c.id), drawioapi.KindUnknown, plainText(c.label)
			r.res.Report = append(r.res.Report, fmt.Sprintf("edge cell %q was added by hand: read as edge %q of kind %q", c.id, e.ID, drawioapi.KindUnknown))
		}
		d.Edges = append(d.Edges, e)
		r.points(e.ID, c)
	}
}

func (r *reader) readForeign() {
	d := r.res.Diagram
	r.modelID = map[string]string{}
	r.zoneOf = map[string]string{}
	hasChildren := map[string]bool{}
	for _, c := range r.cells {
		if c.vertex {
			hasChildren[c.parent] = true
		}
	}
	for _, c := range r.cells {
		if !c.vertex || c.parent == "0" {
			continue
		}
		if hasChildren[c.id] || isContainerStyle(c.style) {
			id := sanitizeID(c.id)
			d.Zones = append(d.Zones, drawioapi.LogicalZone{ID: id, Label: plainText(c.label), Order: len(d.Zones)})
			r.zoneOf[c.id] = id
			r.modelID[c.id] = id
			r.box(id, c)
		}
	}
	unknown := 0
	for _, c := range r.cells {
		if !c.vertex || c.parent == "0" || r.zoneOf[c.id] != "" {
			continue
		}
		id := sanitizeID(c.id)
		r.modelID[c.id] = id
		d.Nodes = append(d.Nodes, drawioapi.LogicalNode{
			ID: id, Label: plainText(c.label), Kind: drawioapi.KindUnknown, Zone: r.nearestZone(c),
		})
		r.box(id, c)
		r.pin(id, c)
		unknown++
	}
	for _, c := range r.cells {
		if !c.edge {
			continue
		}
		src, sok := r.modelID[c.source]
		tgt, tok := r.modelID[c.target]
		if !sok || !tok || r.zoneOf[c.source] != "" || r.zoneOf[c.target] != "" {
			r.res.Report = append(r.res.Report, fmt.Sprintf("edge cell %q left out of the model: an end is missing or is a group (the model has no edge to a zone)", c.id))
			continue
		}
		id := sanitizeID(c.id)
		d.Edges = append(d.Edges, drawioapi.LogicalEdge{ID: id, Source: src, Target: tgt, Kind: drawioapi.KindUnknown, Label: plainText(c.label)})
		r.points(id, c)
	}
	if unknown > 0 {
		r.res.Report = append(r.res.Report, fmt.Sprintf("foreign diagram: %d node(s) read with kind %q (no notation claims them)", unknown, drawioapi.KindUnknown))
	}
}

// nearestZone is the model id of the closest container above c.
func (r *reader) nearestZone(c *cell) string {
	for p := r.byID[c.parent]; p != nil; p = r.byID[p.parent] {
		if z := r.zoneOf[p.id]; z != "" {
			return z
		}
	}
	return ""
}

func (r *reader) insideLegend(c *cell) bool {
	for p := c; p != nil; p = r.byID[p.parent] {
		if role := p.props[AttrRole]; role == RoleLegend || role == RoleDecoration || role == RoleMark {
			return true
		}
		if p.parent == p.id {
			break
		}
	}
	return false
}

// abs is c's top-left corner in page coordinates: a vertex's geometry is
// relative to its parent vertex.
func (r *reader) abs(c *cell) (float64, float64) {
	var x, y float64
	for p := c; p != nil && p.id != "0" && p.id != "1" && p.parent != "0"; p = r.byID[p.parent] {
		if p.geo != nil && p.vertex {
			x += p.geo.X
			y += p.geo.Y
		}
	}
	return x, y
}

func (r *reader) box(id string, c *cell) {
	if c.geo == nil {
		return
	}
	x, y := r.abs(c)
	r.res.Geometry[id] = drawioapi.Box{X: x, Y: y, Width: c.geo.Width, Height: c.geo.Height}
}

func (r *reader) points(id string, c *cell) {
	if r.edgeOf == nil {
		r.edgeOf = map[string]string{}
	}
	r.edgeOf[c.id] = id
	if c.geo != nil && c.geo.Array != nil {
		for _, p := range c.geo.Array.Points {
			r.res.EdgePoints[id] = append(r.res.EdgePoints[id], drawioapi.Point{X: p.X, Y: p.Y})
		}
	}
	r.edgeLabel(id, c)
}

// edgeLabel places an edge's label box: its estimated size, centred on
// the middle of the edge (end centres and bend points) plus the offset.
func (r *reader) edgeLabel(id string, c *cell) {
	w, h := drawioapi.LabelSize(c.label)
	if w == 0 {
		return
	}
	src, tgt := r.modelID[c.source], r.modelID[c.target]
	sb, sok := r.res.Geometry[src]
	tb, tok := r.res.Geometry[tgt]
	if !sok || !tok {
		return
	}
	path := []drawioapi.Point{{X: sb.X + sb.Width/2, Y: sb.Y + sb.Height/2}}
	path = append(path, r.res.EdgePoints[id]...)
	path = append(path, drawioapi.Point{X: tb.X + tb.Width/2, Y: tb.Y + tb.Height/2})
	mid := drawioapi.PolylineMidpoint(path)
	if c.geo != nil {
		for _, p := range c.geo.Named {
			if p.As == "offset" {
				mid.X, mid.Y = mid.X+p.X, mid.Y+p.Y
			}
		}
	}
	r.res.EdgeLabels[id] = drawioapi.Box{X: mid.X - w/2, Y: mid.Y - h/2, Width: w, Height: h}
}

func (r *reader) pin(id string, c *cell) {
	if c.props[AttrPinned] == "1" || strings.Contains(";"+c.style, ";locked=1") {
		r.res.Pinned = append(r.res.Pinned, id)
	}
}

// readAttributes rebuilds the model attributes: the lossless JSON copy,
// with any scalar property a person changed in draw.io's Edit Data
// winning over it.
func readAttributes(props map[string]string) map[string]any {
	attrs := map[string]any{}
	if raw := props[AttrAttributes]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &attrs)
	}
	for k, v := range props {
		if reservedProps[k] || strings.HasPrefix(k, "ktw-") || !xmlName.MatchString(k) {
			continue
		}
		if old, ok := attrs[k]; ok && fmt.Sprint(old) == v {
			continue
		}
		attrs[k] = v
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func isContainerStyle(style string) bool {
	s := ";" + style
	return strings.Contains(s, ";swimlane") || strings.Contains(s, ";container=1") || strings.Contains(s, ";group")
}

var (
	tagRe   = regexp.MustCompile(`(?i)<br\s*/?>|</(div|p|li)>`)
	anyTag  = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
	idBad   = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// plainText turns an HTML cell label into text.
func plainText(label string) string {
	s := tagRe.ReplaceAllString(label, " ")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// sanitizeID makes a draw.io cell id a valid model id.
func sanitizeID(id string) string {
	s := idBad.ReplaceAllString(id, "-")
	if s == "" {
		s = "cell"
	}
	return s
}

// parseCells returns the cells of the first page, in document order.
func parseCells(data []byte) ([]*cell, error) {
	trimmed := bytes.TrimSpace(data)
	if bytes.Contains(trimmed[:min(len(trimmed), 512)], []byte("<mxfile")) {
		expanded, err := drawioapi.DecompressDiagram(trimmed)
		if err != nil {
			return nil, err
		}
		trimmed = expanded
	}
	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	// Find the first <root> (the first page's cells).
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, &drawioapi.DrawioError{Message: "no <root> element: not a draw.io diagram"}
		}
		if err != nil {
			return nil, &drawioapi.DrawioError{Message: "invalid XML: " + err.Error()}
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "root" {
			break
		}
	}
	var cells []*cell
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, &drawioapi.DrawioError{Message: "invalid XML: " + err.Error()}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var rc rawCell
			if err := dec.DecodeElement(&rc, &t); err != nil {
				return nil, &drawioapi.DrawioError{Message: "invalid cell: " + err.Error()}
			}
			c := toCell(&rc)
			c.order = len(cells)
			cells = append(cells, c)
		case xml.EndElement:
			if t.Name.Local == "root" {
				return cells, nil
			}
		}
	}
}

func toCell(rc *rawCell) *cell {
	c := &cell{props: map[string]string{}}
	cellAttrs := rc.Attrs
	c.geo = rc.Geo
	if rc.XMLName.Local != "mxCell" {
		// UserObject / object: its attributes are the properties; the
		// wrapped mxCell carries the graph attributes.
		for _, a := range rc.Attrs {
			c.props[a.Name.Local] = a.Value
		}
		c.id = c.props["id"]
		c.label = c.props["label"]
		cellAttrs = nil
		if rc.Inner != nil {
			cellAttrs = rc.Inner.Attrs
			c.geo = rc.Inner.Geo
		}
	}
	for _, a := range cellAttrs {
		switch a.Name.Local {
		case "id":
			c.id = a.Value
		case "parent":
			c.parent = a.Value
		case "value":
			c.label = a.Value
		case "style":
			c.style = a.Value
		case "vertex":
			c.vertex = a.Value == "1"
		case "edge":
			c.edge = a.Value == "1"
		case "source":
			c.source = a.Value
		case "target":
			c.target = a.Value
		case "visible":
			c.hidden = a.Value == "0"
		}
	}
	return c
}
