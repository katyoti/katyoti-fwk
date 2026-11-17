// SPDX-License-Identifier: Apache-2.0

// legend.go — the legend built from the kinds a diagram uses: one swatch
// per element kind and one sample line per relationship kind, in the
// order they first appear, labelled by the notation, drawn in the page
// template's legend slot or to the right of the drawing.

package archdiagram

import (
	"fmt"
	"math"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/template"
)

const (
	legendPadding  = 12.0
	legendRowH     = 30.0
	legendSwatchW  = 44.0
	legendSwatchH  = 22.0
	legendLabelW   = 170.0
	legendTitleH   = 26.0
	legendGapRight = 60.0
)

// legendEntry is one row: a kind's label and its template.
type legendEntry struct {
	label string
	edge  bool
	tpl   *template.Template
}

// legendEntries lists the kinds the diagram uses that a template draws,
// element kinds first, each once, in first-appearance order.
func (e *emitter) legendEntries() []legendEntry {
	var out []legendEntry
	seen := map[string]bool{}
	for _, n := range e.d.Nodes {
		if seen["n:"+n.Type] {
			continue
		}
		seen["n:"+n.Type] = true
		if t := e.lib.Element(n.Type); t != nil {
			out = append(out, legendEntry{label: labelOr(e.dr.Notation.Elements[n.Type].Label, n.Type), tpl: t})
		}
	}
	for _, ed := range e.d.Edges {
		if seen["e:"+ed.EdgeStyle] {
			continue
		}
		seen["e:"+ed.EdgeStyle] = true
		if t := e.lib.Relationship(ed.EdgeStyle); t != nil {
			out = append(out, legendEntry{label: labelOr(e.dr.Notation.Relationships[ed.EdgeStyle].Label, ed.EdgeStyle), edge: true, tpl: t})
		}
	}
	return out
}

func labelOr(label, kind string) string {
	if label != "" {
		return label
	}
	return kind
}

// drawingBounds returns the right edge and the top of everything drawn.
func drawingBounds(d *ArchDiagram) (right, top float64) {
	right, top = 0, math.Inf(1)
	for _, n := range d.Nodes {
		right = math.Max(right, n.X+n.Width)
		top = math.Min(top, n.Y)
	}
	for _, z := range d.Zones {
		if z.Bounds != nil {
			right = math.Max(right, z.Bounds.X+z.Bounds.Width)
			top = math.Min(top, z.Bounds.Y)
		}
	}
	if math.IsInf(top, 1) {
		top = 0
	}
	return right, top
}

// emitLegend writes the legend container and its rows. It emits nothing
// when the diagram uses fewer than two kinds: a one-kind legend explains
// nothing. Every legend cell carries ktw-role=legend: it is drawn from
// the model, not part of it. It answers the bottom of what it drew (0 for
// none).
func (e *emitter) emitLegend(b *strings.Builder, slot *drawioapi.Box) (bottom float64) {
	entries := e.legendEntries()
	if len(entries) < 2 {
		return 0
	}
	w := legendPadding*3 + legendSwatchW + legendLabelW
	h := legendTitleH + legendPadding + float64(len(entries))*legendRowH
	right, top := drawingBounds(e.d)
	x, y := right+legendGapRight, top
	if slot != nil {
		x, y = slot.X, slot.Y
	}
	role := []attr{{AttrRole, RoleLegend}}

	writeObjectCell(b, "legend", "Legend", role,
		fmt.Sprintf(`style="swimlane;startSize=%.0f;rounded=1;html=1;fillColor=#FAFAFA;strokeColor=#BDBDBD;fontStyle=1;fontSize=12;collapsible=0;" vertex="1" parent="1"`, legendTitleH),
		fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`, x, y, w, h))

	for i, en := range entries {
		rowY := legendTitleH + legendPadding/2 + float64(i)*legendRowH
		sx, sy := legendPadding, rowY+(legendRowH-legendSwatchH)/2
		style := e.lib.SwatchStyle(en.tpl)
		if en.edge {
			style = strings.Replace(style, "edgeStyle=orthogonalEdgeStyle;", "", 1)
			writeObjectCell(b, fmt.Sprintf("legend-%d-swatch", i), "", role,
				fmt.Sprintf(`style="%s" edge="1" parent="legend"`, xmlAttr(style)),
				fmt.Sprintf(`<mxGeometry relative="1" as="geometry"><mxPoint x="%.0f" y="%.0f" as="sourcePoint"/><mxPoint x="%.0f" y="%.0f" as="targetPoint"/></mxGeometry>`,
					sx, sy+legendSwatchH/2, sx+legendSwatchW, sy+legendSwatchH/2))
		} else {
			writeObjectCell(b, fmt.Sprintf("legend-%d-swatch", i), "", role,
				fmt.Sprintf(`style="%s" vertex="1" parent="legend"`, xmlAttr(style)),
				fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`, sx, sy, legendSwatchW, legendSwatchH))
		}
		writeObjectCell(b, fmt.Sprintf("legend-%d-label", i), en.label, role,
			`style="text;html=1;align=left;verticalAlign=middle;fontSize=11;" vertex="1" parent="legend"`,
			fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`, legendPadding*2+legendSwatchW, rowY, legendLabelW, legendRowH))
	}
	return y + h
}
