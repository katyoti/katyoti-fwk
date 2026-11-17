// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
)

// node is an XML element read once into a small tree: OOXML parts vary too
// much (extension lists, alternate content, vendor namespaces) for typed
// structs, and the importer only walks a few known paths. Names are matched
// by their local part: `p:sp` and `a:off` are `sp` and `off`.
type node struct {
	Name  string
	Space string
	Attr  []xml.Attr
	Kids  []*node
	Text  string
}

// parseXML reads a part into its root element.
func parseXML(b []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{Name: t.Name.Local, Space: t.Name.Space, Attr: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.Kids = append(p.Kids, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}
	if root == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return root, nil
}

// child is the first child named name, or nil.
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.Kids {
		if k.Name == name {
			return k
		}
	}
	return nil
}

// children are the children named name.
func (n *node) children(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for _, k := range n.Kids {
		if k.Name == name {
			out = append(out, k)
		}
	}
	return out
}

// path follows children by name: path("spPr", "xfrm", "off").
func (n *node) path(names ...string) *node {
	for _, name := range names {
		n = n.child(name)
	}
	return n
}

// find is the first descendant named name (depth first), or nil.
func (n *node) find(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.Kids {
		if k.Name == name {
			return k
		}
		if f := k.find(name); f != nil {
			return f
		}
	}
	return nil
}

// all are the descendants named name.
func (n *node) all(name string, out []*node) []*node {
	if n == nil {
		return out
	}
	for _, k := range n.Kids {
		if k.Name == name {
			out = append(out, k)
		}
		out = k.all(name, out)
	}
	return out
}

// attr is the attribute named name (local part), or "".
func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// rid is the element's relationship id (`r:id`): an element such as
// `p:sldMasterId` also has a plain `id`, so the namespace decides.
func (n *node) rid() string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Name.Local == "id" && a.Name.Space == nsRelationships {
			return a.Value
		}
	}
	return ""
}

// num is a numeric attribute, or def.
func (n *node) num(name string, def float64) float64 {
	v, err := strconv.ParseFloat(n.attr(name), 64)
	if err != nil {
		return def
	}
	return v
}

// texts concatenates the text of every `a:t` below n, paragraph by paragraph.
func (n *node) texts() string {
	var b bytes.Buffer
	for i, p := range n.all("p", nil) {
		if p.Space != nsDrawing {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteByte('\n')
		}
		for _, t := range p.all("t", nil) {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

const (
	nsDrawing       = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsRelationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)
