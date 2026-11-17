// SPDX-License-Identifier: Apache-2.0

// Package template parses a drawio-template library (a .drawio file whose
// cells are templates) and instantiates a template for one model element:
// placeholders filled, bindings evaluated, conditional cells kept or
// dropped, geometry placed.
package template

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strings"
)

// node is one XML element of a draw.io document, kept with its attribute
// order so a template is re-emitted the way it was authored.
type node struct {
	name     string
	attrs    []xml.Attr
	children []*node
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *node) get(name string) string {
	v, _ := n.attr(name)
	return v
}

func (n *node) set(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].Name.Local == name {
			n.attrs[i].Value = value
			return
		}
	}
	n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (n *node) del(name string) {
	out := n.attrs[:0]
	for _, a := range n.attrs {
		if a.Name.Local != name {
			out = append(out, a)
		}
	}
	n.attrs = out
}

func (n *node) child(name string) *node {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *node) clone() *node {
	c := &node{name: n.name, attrs: append([]xml.Attr(nil), n.attrs...)}
	for _, ch := range n.children {
		c.children = append(c.children, ch.clone())
	}
	return c
}

// parseTree decodes an XML document into a tree of nodes. Text content is
// dropped: draw.io keeps everything in attributes, except a compressed
// <diagram>, which is expanded before this is called.
func parseTree(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name.Local}
			for _, a := range t.Attr {
				n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: a.Name.Local}, Value: a.Value})
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.children = append(p.children, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, fmt.Errorf("xml: empty document")
	}
	return root, nil
}

// write renders n as XML, indented by depth levels of two spaces.
func (n *node) write(b *strings.Builder, depth int) {
	pad := strings.Repeat("  ", depth)
	b.WriteString(pad + "<" + n.name)
	for _, a := range n.attrs {
		b.WriteString(" " + a.Name.Local + `="` + html.EscapeString(a.Value) + `"`)
	}
	if len(n.children) == 0 {
		b.WriteString("/>\n")
		return
	}
	b.WriteString(">\n")
	for _, c := range n.children {
		c.write(b, depth+1)
	}
	b.WriteString(pad + "</" + n.name + ">\n")
}
