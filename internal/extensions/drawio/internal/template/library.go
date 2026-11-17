// SPDX-License-Identifier: Apache-2.0

package template

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// Template attribute names, set on template cells in the library file.
const (
	attrTemplate = "ktw-template" // on a template's top cell: "<kind>", "edge:<kind>" or "*"
	attrStyle    = "ktw-style"    // "<styleKey>=<binding>;…"
	attrWhen     = "ktw-when"     // an expression; the cell is kept only when it holds
	attrSlot     = "ktw-slot"     // on a page template cell: "content" or "legend"
	attrAutosize = "ktw-autosize" // "1": the element's height grows to fit its label
	pagePrefix   = "page:"        // a page named page:<name> is a page template
	edgePrefix   = "edge:"
	defaultKey   = "*"
)

// part is one cell of a template: the element as written (a UserObject,
// an object or a bare mxCell) and the mxCell that carries its graph
// attributes.
type part struct {
	el     *node
	cell   *node
	id     string
	parent string
}

func (p *part) geometry() *node {
	if p.cell == nil {
		return nil
	}
	return p.cell.child("mxGeometry")
}

// Template draws one element or relationship kind.
type Template struct {
	Key    string // the kind, "edge:<kind>" or "*"
	Edge   bool
	Width  float64
	Height float64
	root   *part
	parts  []*part // descendants of root, in document order
}

// Page is a page template: a frame around the drawing.
type Page struct {
	Name    string
	cells   []*part
	Content *drawioapi.Box // where the drawing goes; nil = page origin
	Legend  *drawioapi.Box // where the legend goes; nil = beside the drawing
}

// Library is a parsed template library.
type Library struct {
	Body      *drawioapi.TemplateLibraryBody
	templates map[string]*Template
	pages     map[string]*Page
}

// Parse reads a template library body's draw.io file.
func Parse(body *drawioapi.TemplateLibraryBody) (*Library, error) {
	lib := &Library{Body: body, templates: map[string]*Template{}, pages: map[string]*Page{}}
	data := []byte(strings.TrimSpace(body.Drawio))
	if len(data) == 0 {
		return nil, fmt.Errorf("template library %q: no draw.io content", body.ID)
	}
	if bytes.Contains(data[:min(len(data), 512)], []byte("<mxfile")) {
		expanded, err := drawioapi.DecompressDiagram(data)
		if err != nil {
			return nil, fmt.Errorf("template library %q: %w", body.ID, err)
		}
		data = expanded
	}
	tree, err := parseTree(data)
	if err != nil {
		return nil, fmt.Errorf("template library %q: %w", body.ID, err)
	}
	pages := []*node{tree}
	if tree.name == "mxfile" {
		pages = nil
		for _, d := range tree.children {
			if d.name == "diagram" {
				pages = append(pages, d)
			}
		}
	}
	for _, pg := range pages {
		root := findRoot(pg)
		if root == nil {
			continue
		}
		parts := collectParts(root)
		name := pg.get("name")
		if strings.HasPrefix(name, pagePrefix) {
			lib.pages[strings.TrimPrefix(name, pagePrefix)] = newPage(strings.TrimPrefix(name, pagePrefix), parts)
			continue
		}
		if err := lib.collectTemplates(parts); err != nil {
			return nil, fmt.Errorf("template library %q: %w", body.ID, err)
		}
	}
	return lib, nil
}

// Element returns the template of an element kind, the library's default
// template ("*") when the kind has none, or nil.
func (l *Library) Element(kind string) *Template {
	if t := l.templates[kind]; t != nil {
		return t
	}
	return l.templates[defaultKey]
}

// Relationship returns the template of a relationship kind, or nil.
func (l *Library) Relationship(kind string) *Template {
	if t := l.templates[edgePrefix+kind]; t != nil {
		return t
	}
	return l.templates[edgePrefix+defaultKey]
}

// Zone returns the template that draws a zone: `zone:<kind>` for a zone
// of that kind, else `zone:nested` for a zone inside another, else
// `zone` — or nil.
func (l *Library) Zone(kind string, nested bool) *Template {
	if t := l.templates["zone:"+kind]; kind != "" && t != nil {
		return t
	}
	if nested {
		if t := l.templates["zone:nested"]; t != nil {
			return t
		}
	}
	return l.templates["zone"]
}

// HasZoneKind reports whether the library draws zones of a kind.
func (l *Library) HasZoneKind(kind string) bool { return l.templates["zone:"+kind] != nil }

// Page returns a page template by name, or nil.
func (l *Library) Page(name string) *Page { return l.pages[name] }

// Keys lists the template keys the library defines.
func (l *Library) Keys() []string {
	out := make([]string, 0, len(l.templates))
	for k := range l.templates {
		out = append(out, k)
	}
	return out
}

func findRoot(n *node) *node {
	if n.name == "root" {
		return n
	}
	for _, c := range n.children {
		if r := findRoot(c); r != nil {
			return r
		}
	}
	return nil
}

func collectParts(root *node) []*part {
	var out []*part
	for _, el := range root.children {
		p := &part{el: el}
		if el.name == "mxCell" {
			p.cell = el
			p.id = el.get("id")
		} else {
			p.cell = el.child("mxCell")
			p.id = el.get("id")
		}
		if p.cell != nil {
			p.parent = p.cell.get("parent")
		}
		out = append(out, p)
	}
	return out
}

func (l *Library) collectTemplates(parts []*part) error {
	byParent := map[string][]*part{}
	for _, p := range parts {
		byParent[p.parent] = append(byParent[p.parent], p)
	}
	for _, p := range parts {
		key, ok := p.el.attr(attrTemplate)
		if !ok {
			continue
		}
		if _, dup := l.templates[key]; dup {
			return fmt.Errorf("template %q is defined twice", key)
		}
		t := &Template{Key: key, Edge: strings.HasPrefix(key, edgePrefix), root: p}
		var walk func(id string)
		walk = func(id string) {
			for _, c := range byParent[id] {
				t.parts = append(t.parts, c)
				walk(c.id)
			}
		}
		walk(p.id)
		if g := p.geometry(); g != nil {
			t.Width = num(g.get("width"))
			t.Height = num(g.get("height"))
		}
		l.templates[key] = t
	}
	return nil
}

func newPage(name string, parts []*part) *Page {
	pg := &Page{Name: name}
	for _, p := range parts {
		if p.id == "0" || p.parent == "0" || p.cell == nil {
			continue
		}
		switch p.el.get(attrSlot) {
		case "content":
			pg.Content = box(p)
		case "legend":
			pg.Legend = box(p)
		default:
			pg.cells = append(pg.cells, p)
		}
	}
	return pg
}

func box(p *part) *drawioapi.Box {
	g := p.geometry()
	if g == nil {
		return &drawioapi.Box{}
	}
	return &drawioapi.Box{X: num(g.get("x")), Y: num(g.get("y")), Width: num(g.get("width")), Height: num(g.get("height"))}
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
