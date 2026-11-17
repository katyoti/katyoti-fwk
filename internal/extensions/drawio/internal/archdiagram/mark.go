// SPDX-License-Identifier: Apache-2.0

// mark.go — the production mark (modules/doc-engine/, docs ProductionMark):
// one line of small text under everything drawn, left-aligned with it.
// Like the legend it carries a ktw-role (mark): drawn from the diagram, not
// part of its model, so a read-back skips it.

package archdiagram

import (
	"fmt"
	"math"
	"strings"
)

const (
	markGap    = 24.0
	markHeight = 20.0
	markWidth  = 420.0
)

// emitMark writes the mark's line below the drawing and its legend; nothing
// when the diagram carries no mark.
func (e *emitter) emitMark(b *strings.Builder, legendBottom float64) {
	if strings.TrimSpace(e.d.Mark) == "" {
		return
	}
	left, bottom := math.Inf(1), legendBottom
	for _, n := range e.d.Nodes {
		left = math.Min(left, n.X)
		bottom = math.Max(bottom, n.Y+n.Height)
	}
	for _, z := range e.d.Zones {
		if z.Bounds != nil {
			left = math.Min(left, z.Bounds.X)
			bottom = math.Max(bottom, z.Bounds.Y+z.Bounds.Height)
		}
	}
	if math.IsInf(left, 1) {
		left = 0
	}
	writeObjectCell(b, "production-mark", e.d.Mark, []attr{{AttrRole, RoleMark}},
		`style="text;html=1;align=left;verticalAlign=middle;fontSize=10;fontColor=#757575;" vertex="1" parent="1"`,
		fmt.Sprintf(`<mxGeometry x="%.0f" y="%.0f" width="%.0f" height="%.0f" as="geometry"/>`, left, bottom+markGap, markWidth, markHeight))
}
