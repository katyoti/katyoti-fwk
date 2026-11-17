// SPDX-License-Identifier: Apache-2.0

package quality

import (
	"math"
	"sort"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// A painted label: the cell it belongs to, its text and its estimated
// boxes (nominal for overlaps, pessimistic for overflows).
type paintedLabel struct {
	cell  drawioapi.DrawnCell
	order int
	text  string
	ink   drawioapi.Box
	wide  drawioapi.Box // OverflowSlack widths
}

// CheckLayout finds what reads badly on a drawing as draw.io paints it:
// labels painted over each other (text-overlap, the gated finding), the
// same text painted twice in one place (duplicate), labels out of their own
// shape (text-overflow) and filled shapes covering each other or a label
// (box-cover). Two boxes meet only when they intersect by more than
// LayoutTolerance both ways. Not counted: a label inside its own shape, a
// text cell over a shape, a shape inside another (equal boxes included),
// cells not painted; a label cell on an edge is listed as not measured.
func CheckLayout(r *drawioapi.ReadResult) []drawioapi.Finding {
	var findings []drawioapi.Finding
	var labels []paintedLabel
	var shapes []paintedLabel // filled shapes, label or not
	byID := map[string]drawioapi.DrawnCell{}
	for _, d := range r.Drawn {
		byID[d.ID] = d
	}
	for i, d := range r.Drawn {
		if d.Hidden {
			continue
		}
		text := drawioapi.PlainLabel(d.Label)
		if d.OnEdge {
			if text != "" {
				findings = append(findings, drawioapi.Finding{Kind: drawioapi.FindingNotMeasured, CellIDs: []string{d.ID}, Labels: []string{text}})
			}
			continue
		}
		st := drawioapi.ParseStyle(d.Style)
		if d.Vertex && isFilled(st) && d.Box.Width > 0 && d.Box.Height > 0 {
			shapes = append(shapes, paintedLabel{cell: d, order: i, text: text})
		}
		if text == "" {
			continue
		}
		p := paintedLabel{cell: d, order: i, text: text}
		if d.Edge {
			p.ink, p.wide = d.Box, d.Box
		} else {
			p.ink, _ = drawioapi.TextBox(d.Label, d.Style, d.Box, 1)
			p.wide, _ = drawioapi.TextBox(d.Label, d.Style, d.Box, drawioapi.OverflowSlack)
		}
		if p.ink.Width > 0 && p.ink.Height > 0 {
			labels = append(labels, p)
		}
	}

	// Labels over labels.
	for i := range labels {
		for j := i + 1; j < len(labels); j++ {
			a, b := labels[i], labels[j]
			meet, depth, ok := meets(a.ink, b.ink)
			if !ok {
				continue
			}
			f := drawioapi.Finding{Kind: drawioapi.FindingTextOverlap, Gated: true,
				CellIDs: []string{a.cell.ID, b.cell.ID}, Labels: []string{a.text, b.text}, Box: meet, Depth: depth}
			if a.text == b.text && sameBox(a.ink, b.ink) {
				f.Kind, f.Gated = drawioapi.FindingDuplicate, false
			}
			findings = append(findings, f)
		}
	}

	// Labels out of their own shape.
	for _, l := range labels {
		if l.cell.Edge {
			continue
		}
		st := drawioapi.ParseStyle(l.cell.Style)
		if st["overflow"] == "hidden" || st["overflow"] == "fill" || outsideByDesign(st) {
			continue
		}
		if out := outside(l.wide, l.cell.Box); out > drawioapi.LayoutTolerance {
			findings = append(findings, drawioapi.Finding{Kind: drawioapi.FindingTextOverflow,
				CellIDs: []string{l.cell.ID}, Labels: []string{l.text}, Box: l.wide, Depth: round(out)})
		}
	}

	// Filled shapes covering each other: neither holds the other.
	for i := range shapes {
		for j := i + 1; j < len(shapes); j++ {
			a, b := shapes[i], shapes[j]
			if contains(a.cell.Box, b.cell.Box) || contains(b.cell.Box, a.cell.Box) {
				continue
			}
			if meet, depth, ok := meets(a.cell.Box, b.cell.Box); ok {
				findings = append(findings, drawioapi.Finding{Kind: drawioapi.FindingBoxCover,
					CellIDs: []string{a.cell.ID, b.cell.ID}, Labels: nonEmpty(a.text, b.text), Box: meet, Depth: depth})
			}
		}
	}

	// A label under a later opaque shape that is not its own decoration.
	for _, l := range labels {
		if l.cell.Edge {
			continue
		}
		for _, s := range shapes {
			if s.order <= l.order || s.cell.ID == l.cell.ID || hollow(s.cell.Style) || translucent(s.cell.Style) {
				continue
			}
			if contains(l.cell.Box, s.cell.Box) || isAncestor(byID, s.cell.ID, l.cell.ID) {
				continue
			}
			if meet, depth, ok := meets(l.ink, s.cell.Box); ok {
				findings = append(findings, drawioapi.Finding{Kind: drawioapi.FindingBoxCover,
					CellIDs: []string{l.cell.ID, s.cell.ID}, Labels: nonEmpty(l.text, s.text), Box: meet, Depth: depth})
			}
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Gated != findings[j].Gated {
			return findings[i].Gated
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings
}

// countFindings adds the layout findings' counts to a report.
func countFindings(q *drawioapi.QualityReport, findings []drawioapi.Finding) {
	for _, f := range findings {
		switch f.Kind {
		case drawioapi.FindingTextOverlap:
			q.TextOverlaps++
		case drawioapi.FindingDuplicate:
			q.Duplicates++
		case drawioapi.FindingTextOverflow:
			q.TextOverflows++
		case drawioapi.FindingBoxCover:
			q.BoxCovers++
		}
	}
}

// meets is the intersection of two boxes when it exceeds the tolerance both
// ways, with its depth (the smaller extent).
func meets(a, b drawioapi.Box) (drawioapi.Box, float64, bool) {
	x0, y0 := math.Max(a.X, b.X), math.Max(a.Y, b.Y)
	x1, y1 := math.Min(a.X+a.Width, b.X+b.Width), math.Min(a.Y+a.Height, b.Y+b.Height)
	w, h := x1-x0, y1-y0
	if w <= drawioapi.LayoutTolerance || h <= drawioapi.LayoutTolerance {
		return drawioapi.Box{}, 0, false
	}
	return drawioapi.Box{X: round(x0), Y: round(y0), Width: round(w), Height: round(h)}, round(math.Min(w, h)), true
}

// sameBox reports two boxes equal within a pixel.
func sameBox(a, b drawioapi.Box) bool {
	return math.Abs(a.X-b.X) <= 1 && math.Abs(a.Y-b.Y) <= 1 &&
		math.Abs(a.Width-b.Width) <= 1 && math.Abs(a.Height-b.Height) <= 1
}

// outside is how far ink reaches out of cell, the largest side.
func outside(ink, cell drawioapi.Box) float64 {
	return math.Max(math.Max(cell.X-ink.X, ink.X+ink.Width-cell.X-cell.Width),
		math.Max(cell.Y-ink.Y, ink.Y+ink.Height-cell.Y-cell.Height))
}

// isFilled reports a vertex style that paints an opaque area: not a text
// cell, a group, a line or an image, and not fillColor=none (draw.io fills
// a shape white by default).
func isFilled(st map[string]string) bool {
	for _, k := range []string{"text", "group", "edgeLabel", "line", "image"} {
		if _, ok := st[k]; ok {
			return false
		}
	}
	switch st["shape"] {
	case "image", "line", "waypoint", "flexArrow", "link":
		return false
	}
	return st["fillColor"] != "none"
}

// hollow reports a shape painted as an outline around an empty middle (a
// ring, an arc, a frame): it covers nothing inside it.
func hollow(style string) bool {
	s := strings.ToLower(style)
	for _, k := range []string{"donut", "concellipse", "arc", "frame", "swimlane"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// translucent reports a shape painted see-through.
func translucent(style string) bool {
	st := drawioapi.ParseStyle(style)
	for _, k := range []string{"opacity", "fillOpacity"} {
		if v := st[k]; v != "" && v != "100" {
			return true
		}
	}
	return false
}

// outsideByDesign reports a label placed beside its shape by its style.
func outsideByDesign(st map[string]string) bool {
	return (st["labelPosition"] != "" && st["labelPosition"] != "center") ||
		(st["verticalLabelPosition"] != "" && st["verticalLabelPosition"] != "middle")
}

func isAncestor(byID map[string]drawioapi.DrawnCell, anc, id string) bool {
	for d, ok := byID[id]; ok; d, ok = byID[d.Parent] {
		if d.Parent == anc {
			return true
		}
		if d.Parent == d.ID {
			break
		}
	}
	return false
}

func nonEmpty(s ...string) []string {
	var out []string
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
