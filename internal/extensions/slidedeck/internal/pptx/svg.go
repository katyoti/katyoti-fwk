// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// freeformIcons turns a slide's freeform shapes (`a:custGeom`, outside any
// placeholder, without text) into SVG pictograms, one per shape. Their colour
// is dropped: a theme draws an icon as a mask in its own colour, so only the
// shape matters (filled shapes are filled, outlines stroked).
func freeformIcons(tree *node, slide string) []Icon {
	var out []Icon
	var walk func(n *node, t *xform)
	walk = func(n *node, t *xform) {
		if n == nil {
			return
		}
		for _, k := range n.Kids {
			switch k.Name {
			case "grpSp":
				walk(k, groupXform(k, t))
			case "sp":
				if k.path("nvSpPr", "nvPr", "ph") != nil || strings.TrimSpace(k.child("txBody").texts()) != "" {
					continue
				}
				geom := k.path("spPr", "custGeom")
				if geom == nil {
					continue
				}
				xf := k.path("spPr", "xfrm")
				b, ok := boxOf(xf, t)
				if !ok || b.W < 4 || b.H < 4 {
					continue
				}
				if svg := customGeometrySVG(geom, k.path("spPr"), b, xf.attr("flipH") == "1", xf.attr("flipV") == "1"); svg != "" {
					out = append(out, Icon{Slide: slide, Name: fmt.Sprintf("%s-%d", strings.TrimSuffix(slide, ".xml"), len(out)+1), SVG: svg})
				}
			}
		}
	}
	walk(tree, nil)
	return out
}

// customGeometrySVG draws one freeform's paths, in a view box of the shape's
// own size in slide pixels.
func customGeometrySVG(geom, spPr *node, b Box, flipH, flipV bool) string {
	d := geometryPath(geom, b, flipH, flipV)
	if d == "" {
		return ""
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s"><path fill-rule="evenodd" %s d="%s"/></svg>`,
		f(b.W), f(b.H), f(b.W), f(b.H), paint(spPr, "#000"), d)
}

// paint is a freeform's fill (filled shapes) or stroke (outlines), in colour c.
func paint(spPr *node, c string) string {
	if spPr.child("noFill") != nil {
		w := maxf(1, spPr.path("ln").num("w", 12700)/EMUPerPx)
		return fmt.Sprintf(`fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"`, c, f(w))
	}
	return fmt.Sprintf(`fill="%s"`, c)
}

// vectorGroup reads a group drawn only with freeforms and no text — a logo
// drawn as vector shapes — into one SVG keeping each shape's colour, in a
// view box of the group's size in slide pixels.
func (m *Master) vectorGroup(g *node, outer *xform) (Box, string, bool) {
	gb, ok := boxOf(g.path("grpSpPr", "xfrm"), outer)
	if !ok || gb.W < 4 || gb.H < 4 {
		return Box{}, "", false
	}
	var paths strings.Builder
	var walk func(n *node, t *xform) bool
	walk = func(n *node, t *xform) bool {
		for _, k := range n.Kids {
			switch k.Name {
			case "grpSp":
				if !walk(k, groupXform(k, t)) {
					return false
				}
			case "sp":
				geom := k.path("spPr", "custGeom")
				if geom == nil || k.path("nvSpPr", "nvPr", "ph") != nil || strings.TrimSpace(k.child("txBody").texts()) != "" {
					return false
				}
				xf := k.path("spPr", "xfrm")
				b, ok := boxOf(xf, t)
				if !ok {
					return false
				}
				d := geometryPath(geom, b, xf.attr("flipH") == "1", xf.attr("flipV") == "1")
				if d == "" {
					continue
				}
				c := m.colour(k.child("spPr"))
				if c == "" {
					c = m.clr(k.path("style", "fillRef"))
				}
				fmt.Fprintf(&paths, `<path transform="translate(%s %s)" fill-rule="evenodd" %s d="%s"/>`, f(b.X-gb.X), f(b.Y-gb.Y), paint(k.child("spPr"), or(c, "#000")), d)
			case "nvGrpSpPr", "grpSpPr", "extLst":
			default:
				return false
			}
		}
		return true
	}
	if !walk(g, groupXform(g, outer)) || paths.Len() == 0 {
		return Box{}, "", false
	}
	return gb, fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">%s</svg>`, f(gb.W), f(gb.H), f(gb.W), f(gb.H), paths.String()), true
}

// geometryPath is a freeform's path data in the shape's own box.
func geometryPath(geom *node, b Box, flipH, flipV bool) string {
	var d strings.Builder
	for _, p := range geom.path("pathLst").children("path") {
		pw, ph := p.num("w", 0), p.num("h", 0)
		if pw == 0 {
			pw = b.W * EMUPerPx
		}
		if ph == 0 {
			ph = b.H * EMUPerPx
		}
		sx, sy := b.W/pw, b.H/ph
		pt := func(n *node) (float64, float64) {
			x, y := n.num("x", 0)*sx, n.num("y", 0)*sy
			if flipH {
				x = b.W - x
			}
			if flipV {
				y = b.H - y
			}
			return x, y
		}
		var cx, cy float64
		for _, c := range p.Kids {
			pts := c.children("pt")
			switch c.Name {
			case "moveTo":
				if len(pts) == 1 {
					cx, cy = pt(pts[0])
					fmt.Fprintf(&d, "M%s %s", f(cx), f(cy))
				}
			case "lnTo":
				if len(pts) == 1 {
					cx, cy = pt(pts[0])
					fmt.Fprintf(&d, "L%s %s", f(cx), f(cy))
				}
			case "cubicBezTo":
				if len(pts) == 3 {
					x1, y1 := pt(pts[0])
					x2, y2 := pt(pts[1])
					cx, cy = pt(pts[2])
					fmt.Fprintf(&d, "C%s %s %s %s %s %s", f(x1), f(y1), f(x2), f(y2), f(cx), f(cy))
				}
			case "quadBezTo":
				if len(pts) == 2 {
					x1, y1 := pt(pts[0])
					cx, cy = pt(pts[1])
					fmt.Fprintf(&d, "Q%s %s %s %s", f(x1), f(y1), f(cx), f(cy))
				}
			case "arcTo":
				// The current point is on the ellipse at stAng; the arc sweeps
				// swAng (60 000ths of a degree).
				wr, hr := c.num("wR", 0)*sx, c.num("hR", 0)*sy
				st := c.num("stAng", 0) / 60000 * math.Pi / 180
				sw := c.num("swAng", 0) / 60000 * math.Pi / 180
				ox, oy := cx-wr*math.Cos(st), cy-hr*math.Sin(st)
				ex, ey := ox+wr*math.Cos(st+sw), oy+hr*math.Sin(st+sw)
				large, sweep := 0, 0
				if math.Abs(sw) > math.Pi {
					large = 1
				}
				if sw > 0 {
					sweep = 1
				}
				fmt.Fprintf(&d, "A%s %s 0 %d %d %s %s", f(wr), f(hr), large, sweep, f(ex), f(ey))
				cx, cy = ex, ey
			case "close":
				d.WriteString("Z")
			}
		}
	}
	return d.String()
}

// f writes a coordinate with two decimals at most.
func f(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }
