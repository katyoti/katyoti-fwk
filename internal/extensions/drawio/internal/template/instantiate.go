// SPDX-License-Identifier: Apache-2.0

package template

import (
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// Attr is one attribute the caller adds to an instance's top cell (the
// model identity).
type Attr struct{ Name, Value string }

// Placement is where and as what one template instance is drawn.
type Placement struct {
	CellID   string // id of the instance's top cell; parts get CellID-<part id>
	ParentID string // the cell the instance sits in
	// Vertex geometry, relative to ParentID.
	X, Y, Width, Height float64
	// Edge ends and bend points (page coordinates).
	SourceID, TargetID string
	Points             []drawioapi.Point
	// LabelOffset moves an edge's label from the middle of the edge.
	LabelOffset *drawioapi.Point
	// Identity is added to the top cell; PartRole to every other part.
	Identity []Attr
	PartRole []Attr
}

// Instance is a filled template, ready to write.
type Instance struct {
	cells []*node
	// Problems lists what could not be evaluated (a binding or a
	// ktw-when that failed); the cell still renders.
	Problems []string
}

// Write renders the instance's cells.
func (in *Instance) Write(b *strings.Builder, depth int) {
	for _, c := range in.cells {
		c.write(b, depth)
	}
}

// Instantiate fills t for one element or relationship. vars are extra
// placeholder values (a page's title and date).
func (l *Library) Instantiate(t *Template, scope Scope, at Placement, vars map[string]string) *Instance {
	f := &filler{lib: l, scope: scope, vars: vars, cache: map[string]string{}}
	in := &Instance{}
	ids := map[string]string{t.root.id: at.CellID}

	top := f.fill(t.root, at.CellID, at.ParentID)
	for _, a := range at.Identity {
		top.set(a.Name, a.Value)
	}
	cell := top.child("mxCell")
	if t.Edge {
		cell.set("source", at.SourceID)
		cell.set("target", at.TargetID)
		cell.children = []*node{edgeGeometry(at.Points, at.LabelOffset)}
	} else if g := cell.child("mxGeometry"); g != nil {
		g.set("x", fmtNum(at.X))
		g.set("y", fmtNum(at.Y))
		g.set("width", fmtNum(at.Width))
		g.set("height", fmtNum(at.Height))
	}
	in.cells = append(in.cells, top)

	dropped := map[string]bool{}
	for _, p := range t.parts {
		if dropped[p.parent] {
			dropped[p.id] = true
			continue
		}
		if expr, ok := p.el.attr(attrWhen); ok {
			keep, err := When(expr, scope)
			if err != nil {
				in.Problems = append(in.Problems, fmt.Sprintf("template %s: ktw-when %q: %v", t.Key, expr, err))
			}
			if !keep {
				dropped[p.id] = true
				continue
			}
		}
		id := at.CellID + "-" + sanitize(p.id)
		ids[p.id] = id
		n := f.fill(p, id, ids[p.parent])
		for _, a := range at.PartRole {
			n.set(a.Name, a.Value)
		}
		in.cells = append(in.cells, n)
	}
	in.Problems = append(in.Problems, f.problems...)
	return in
}

// Size is the box an element takes: the template's own size, grown to
// fit its label when the template sets ktw-autosize="1" (one line per
// <br>, <hr>, <div> or <p> break, lineHeight pixels each).
func (l *Library) Size(t *Template, scope Scope) (w, h float64) {
	w, h = t.Width, t.Height
	if t.root.el.get(attrAutosize) != "1" {
		return w, h
	}
	f := &filler{lib: l, scope: scope, cache: map[string]string{}}
	label := t.root.el.get("label")
	if t.root.el.name == "mxCell" {
		label = t.root.cell.get("value")
	}
	lines := 1 + len(lineBreak.FindAllString(f.label(label), -1))
	if need := float64(lines)*lineHeight + labelPadding; need > h {
		h = need
	}
	return w, h
}

const (
	lineHeight   = 15.0
	labelPadding = 14.0
)

var lineBreak = regexp.MustCompile(`(?i)<br\s*/?>|<hr\s*/?>|</div>|</p>`)

// SwatchStyle is the style a legend draws for a template: its top cell's
// style with the bindings evaluated on an empty element.
func (l *Library) SwatchStyle(t *Template) string {
	f := &filler{lib: l, scope: NewScope("", "", strings.TrimPrefix(t.Key, edgePrefix), nil, l.Body.Constants), cache: map[string]string{}}
	return f.style(t.root.cell.get("style"), t.root.el.get(attrStyle))
}

// InstantiatePage fills a page template's frame cells.
func (l *Library) InstantiatePage(pg *Page, vars map[string]string, role []Attr) *Instance {
	f := &filler{lib: l, scope: NewScope("", vars["title"], "", nil, l.Body.Constants), vars: vars, cache: map[string]string{}}
	in := &Instance{}
	for _, p := range pg.cells {
		parent := p.parent
		if parent != "1" {
			parent = "page-" + sanitize(parent)
		}
		n := f.fill(p, "page-"+sanitize(p.id), parent)
		for _, a := range role {
			n.set(a.Name, a.Value)
		}
		in.cells = append(in.cells, n)
	}
	in.Problems = f.problems
	return in
}

// filler fills one template's cells for one scope.
type filler struct {
	lib      *Library
	scope    Scope
	vars     map[string]string
	cache    map[string]string
	problems []string
}

// fill clones a part as a UserObject with a filled label, tooltip, link
// and style. Template-only attributes and sample properties are dropped:
// an instance carries the model's values, never the template's samples.
func (f *filler) fill(p *part, id, parent string) *node {
	cell := &node{name: "mxCell"}
	if p.cell != nil {
		cell = p.cell.clone()
	}
	label := p.el.get("label")
	if p.el.name == "mxCell" {
		label = p.cell.get("value")
	}
	out := &node{name: "UserObject"}
	out.set("label", f.label(label))
	for _, k := range []string{"tooltip", "link"} {
		if v, ok := p.el.attr(k); ok && p.el.name != "mxCell" {
			if s := strings.TrimSpace(f.text(v, false)); s != "" {
				out.set(k, s)
			}
		}
	}
	out.set("id", id)
	cell.del("id")
	cell.del("value")
	cell.set("parent", parent)
	cell.set("style", f.style(cell.get("style"), p.el.get(attrStyle)))
	out.children = []*node{cell}
	return out
}

var placeholder = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_.-]*)%`)

// label fills an HTML label: model values are escaped, binding values are
// trusted HTML; empty markup left by missing values is removed.
func (f *filler) label(s string) string {
	return cleanHTML(f.text(s, true))
}

// text fills placeholders in s.
func (f *filler) text(s string, htmlLabel bool) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		v, fromBinding := f.value(name)
		if htmlLabel && !fromBinding {
			return html.EscapeString(v)
		}
		return v
	})
}

// value resolves a placeholder name: a binding, else an attribute, else
// the element's own name / id / kind, else a page variable.
func (f *filler) value(name string) (string, bool) {
	if v, ok := f.cache[name]; ok {
		return v, true
	}
	if r, ok := f.lib.Body.Bindings[name]; ok {
		v, err := Evaluate(&r, f.scope)
		if err != nil {
			f.problems = append(f.problems, fmt.Sprintf("binding %q: %v", name, err))
		}
		s := text(v)
		f.cache[name] = s
		return s, true
	}
	if attrs, ok := f.scope["attributes"].(map[string]any); ok {
		if v, ok := attrs[name]; ok {
			return text(v), false
		}
	}
	switch name {
	case "name", "id", "kind":
		return text(f.scope[name]), false
	}
	if v, ok := f.vars[name]; ok {
		return v, false
	}
	return "", false
}

// style applies ktw-style overrides ("key=binding;…") to a style string.
func (f *filler) style(style, overrides string) string {
	if overrides == "" {
		return style
	}
	for _, pair := range strings.Split(overrides, ";") {
		key, binding, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || key == "" {
			continue
		}
		v, isBinding := f.value(strings.TrimSpace(binding))
		if !isBinding {
			f.problems = append(f.problems, fmt.Sprintf("ktw-style %s: no binding named %q", key, binding))
			continue
		}
		if v != "" {
			style = setStyle(style, key, v)
		}
	}
	return style
}

// setStyle sets key=value in a draw.io style string, keeping the order of
// the other entries.
func setStyle(style, key, value string) string {
	parts := strings.Split(strings.TrimSuffix(style, ";"), ";")
	found := false
	for i, p := range parts {
		if k, _, ok := strings.Cut(p, "="); ok && k == key {
			parts[i] = key + "=" + value
			found = true
		}
	}
	if !found {
		if len(parts) == 1 && parts[0] == "" {
			parts = parts[:0]
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ";") + ";"
}

var (
	emptyInline = regexp.MustCompile(`(?i)<(b|i|u|em|strong|span|font|a)(\s[^>]*)?>\s*</(b|i|u|em|strong|span|font|a)>`)
	brRun       = regexp.MustCompile(`(?i)(<br\s*/?>\s*){2,}`)
	brEdges     = regexp.MustCompile(`(?i)^(\s*<br\s*/?>)+|(<br\s*/?>\s*)+$`)
)

// cleanHTML removes the markup a missing value leaves behind: empty inline
// elements, repeated and trailing line breaks.
func cleanHTML(s string) string {
	for {
		t := emptyInline.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = t
	}
	s = brRun.ReplaceAllString(s, "<br>")
	return strings.TrimSpace(brEdges.ReplaceAllString(s, ""))
}

func edgeGeometry(points []drawioapi.Point, offset *drawioapi.Point) *node {
	g := &node{name: "mxGeometry", attrs: []xml.Attr{
		{Name: xml.Name{Local: "relative"}, Value: "1"},
		{Name: xml.Name{Local: "as"}, Value: "geometry"},
	}}
	if len(points) > 0 {
		arr := &node{name: "Array", attrs: []xml.Attr{{Name: xml.Name{Local: "as"}, Value: "points"}}}
		for _, p := range points {
			arr.children = append(arr.children, &node{name: "mxPoint", attrs: []xml.Attr{
				{Name: xml.Name{Local: "x"}, Value: fmtNum(p.X)},
				{Name: xml.Name{Local: "y"}, Value: fmtNum(p.Y)},
			}})
		}
		g.children = []*node{arr}
	}
	if offset != nil {
		g.children = append(g.children, &node{name: "mxPoint", attrs: []xml.Attr{
			{Name: xml.Name{Local: "x"}, Value: fmtNum(offset.X)},
			{Name: xml.Name{Local: "y"}, Value: fmtNum(offset.Y)},
			{Name: xml.Name{Local: "as"}, Value: "offset"},
		}})
	}
	return g
}

// EdgeLabel is the label an edge template draws for a scope, filled.
func (l *Library) EdgeLabel(t *Template, scope Scope) string {
	f := &filler{lib: l, scope: scope, cache: map[string]string{}}
	label := t.root.el.get("label")
	if t.root.el.name == "mxCell" {
		label = t.root.cell.get("value")
	}
	return f.label(label)
}

var badID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func sanitize(id string) string { return badID.ReplaceAllString(id, "-") }

func fmtNum(f float64) string { return fmt.Sprintf("%.0f", f) }
