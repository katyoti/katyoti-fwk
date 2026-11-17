// SPDX-License-Identifier: Apache-2.0

// identity.go — how a model thing is recorded on its drawio cell, so a
// diagram can be read back (see reader.go). A cell that stands for a
// model thing is a UserObject with:
//
//	ktw-role        node | edge | zone | diagram
//	ktw-id          the model id
//	ktw-kind        the stencil-set kind (nodes and edges)
//	ktw-label       the model's own label (the cell label is the rendered one)
//	ktw-attributes  the model attributes, JSON-encoded (lossless)
//	<key>="<value>" each scalar attribute again as a plain property, so
//	                draw.io's Edit Data and %key% placeholders see it
//
// The diagram itself (title, viewpoint, layout hints) is recorded on the
// root cell "0". Decorations, the legend and the production mark carry
// ktw-role only: they are drawn from the model, not part of it.

package archdiagram

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The roles a cell can play.
const (
	RoleDiagram    = "diagram"
	RoleNode       = "node"
	RoleEdge       = "edge"
	RoleZone       = "zone"
	RoleDecoration = "decoration"
	RoleLegend     = "legend"
	RoleMark       = "mark"
)

// Identity attribute names.
const (
	AttrRole       = "ktw-role"
	AttrID         = "ktw-id"
	AttrKind       = "ktw-kind"
	AttrLabel      = "ktw-label"
	AttrAttributes = "ktw-attributes"
	AttrPinned     = "ktw-pinned"
	AttrOrder      = "ktw-order"
	AttrTitle      = "ktw-title"
	AttrViewpoint  = "ktw-viewpoint"
	AttrLayout     = "ktw-layout-hints"
	AttrPage       = "ktw-page"
	AttrParentZone = "ktw-parent-zone"
)

// NodeCellID, EdgeCellID and ZoneCellID derive a stable cell id from a
// model id; the prefixes keep the three namespaces apart.
func NodeCellID(id string) string { return "n-" + id }
func EdgeCellID(id string) string { return "e-" + id }
func ZoneCellID(id string) string { return "z-" + id }

// attr is one XML attribute, kept in write order.
type attr struct{ name, value string }

// xmlName is what may stand as a plain property name.
var xmlName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// reservedProps are UserObject attributes draw.io or this module own.
var reservedProps = map[string]bool{
	"id": true, "label": true, "placeholders": true, "link": true, "tooltip": true,
	"style": true, "parent": true, "vertex": true, "edge": true, "source": true, "target": true,
}

// identityAttrs builds the identity attributes of a node or edge cell.
func identityAttrs(role, id, kind, label string, attributes map[string]any) []attr {
	out := []attr{{AttrRole, role}, {AttrID, id}}
	if kind != "" {
		out = append(out, attr{AttrKind, kind})
	}
	if label != "" {
		out = append(out, attr{AttrLabel, label})
	}
	if len(attributes) == 0 {
		return out
	}
	if raw, err := json.Marshal(attributes); err == nil {
		out = append(out, attr{AttrAttributes, string(raw)})
	}
	keys := make([]string, 0, len(attributes))
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !xmlName.MatchString(k) || reservedProps[k] || strings.HasPrefix(k, "ktw-") {
			continue
		}
		switch v := attributes[k].(type) {
		case string:
			out = append(out, attr{k, v})
		case bool, float64, float32, int, int64:
			out = append(out, attr{k, fmt.Sprint(v)})
		}
	}
	return out
}

// writeObjectCell writes a UserObject wrapping one mxCell. cellAttrs is
// the raw attribute text of the mxCell (style, vertex/edge, parent…),
// geometry the raw child element.
func writeObjectCell(b *strings.Builder, id, label string, obj []attr, cellAttrs, geometry string) {
	b.WriteString(fmt.Sprintf(`        <UserObject label="%s"`, xmlAttr(label)))
	for _, a := range obj {
		b.WriteString(fmt.Sprintf(` %s="%s"`, a.name, xmlAttr(a.value)))
	}
	b.WriteString(fmt.Sprintf(" id=\"%s\">\n", xmlAttr(id)))
	b.WriteString("          <mxCell " + cellAttrs + ">\n")
	b.WriteString("            " + geometry + "\n")
	b.WriteString("          </mxCell>\n")
	b.WriteString("        </UserObject>\n")
}

// writeHeader writes the mxfile envelope and the two root cells; cell "0"
// records the diagram's own identity.
func writeHeader(b *strings.Builder, d *ArchDiagram) {
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<mxfile>
  <diagram name="Page-1">
    <mxGraphModel dx="1422" dy="794" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="4000" pageHeight="3000">
      <root>
`)
	root := []attr{{AttrRole, RoleDiagram}}
	if d.Title != "" {
		root = append(root, attr{AttrTitle, d.Title})
	}
	if d.Viewpoint != "" {
		root = append(root, attr{AttrViewpoint, d.Viewpoint})
	}
	if d.Hints != nil {
		if raw, err := json.Marshal(d.Hints); err == nil && string(raw) != "{}" {
			root = append(root, attr{AttrLayout, string(raw)})
		}
	}
	if d.Page != nil {
		if raw, err := json.Marshal(d.Page); err == nil && string(raw) != "{}" {
			root = append(root, attr{AttrPage, string(raw)})
		}
	}
	b.WriteString(`        <UserObject label=""`)
	for _, a := range root {
		b.WriteString(fmt.Sprintf(` %s="%s"`, a.name, xmlAttr(a.value)))
	}
	b.WriteString(" id=\"0\">\n          <mxCell/>\n        </UserObject>\n")
	b.WriteString("        <mxCell id=\"1\" parent=\"0\"/>\n")
}
