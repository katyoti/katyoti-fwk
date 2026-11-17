// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/quality"
)

// PAGES — a .drawio file holds one page or several (one <diagram> each).
// Pages splits a file into its pages, each a file of its own; Assemble puts
// named pages into one file, each page's id its name slugged, so a cell links
// to it (data:page/id,<id>). Read, score and the layout check run on every
// page and name the page of what they find.

// Page is one page of a drawing: its id, its name, and the page alone as a
// .drawio file (decompressed).
type Page struct {
	ID   string
	Name string
	XML  []byte
}

// NamedPage is a page to assemble: its name and a .drawio file whose first
// page it is — Emit's output, or a page of cells a caller placed.
type NamedPage struct {
	Name string
	XML  []byte
}

type pageFile struct {
	XMLName  xml.Name   `xml:"mxfile"`
	Attrs    []xml.Attr `xml:",any,attr"`
	Diagrams []pageDiag `xml:"diagram"`
}

type pageDiag struct {
	ID    string `xml:"id,attr,omitempty"`
	Name  string `xml:"name,attr,omitempty"`
	Inner []byte `xml:",innerxml"`
}

// Pages splits a .drawio document (plain or compressed, or the diagram a
// .drawio.png or .drawio.svg embeds) into its pages, in order. A bare
// <mxGraphModel> is one page.
func Pages(data []byte) ([]Page, error) {
	raw, err := drawioapi.ExtractDrawioXML(data)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if !bytes.Contains(trimmed[:min(len(trimmed), 512)], []byte("<mxfile")) {
		return []Page{{XML: trimmed}}, nil
	}
	expanded, err := drawioapi.DecompressDiagram(trimmed)
	if err != nil {
		return nil, err
	}
	var f pageFile
	if err := xml.Unmarshal(expanded, &f); err != nil {
		return nil, &drawioapi.DrawioError{Message: "invalid XML: " + err.Error()}
	}
	out := make([]Page, 0, len(f.Diagrams))
	for _, d := range f.Diagrams {
		one, err := xml.Marshal(pageFile{Diagrams: []pageDiag{d}})
		if err != nil {
			return nil, err
		}
		out = append(out, Page{ID: d.ID, Name: d.Name, XML: one})
	}
	return out, nil
}

// Assemble puts named pages into one .drawio file, in order: each page the
// first page of its file, its <diagram> named and given the id PageID(name).
// Two pages with the same id are refused: a link could not tell them apart.
func Assemble(pages ...NamedPage) ([]byte, error) {
	if len(pages) == 0 {
		return nil, &drawioapi.DrawioError{Message: "no page to assemble"}
	}
	out := pageFile{Attrs: []xml.Attr{{Name: xml.Name{Local: "host"}, Value: "ktw-drawio"}}}
	seen := map[string]bool{}
	for _, p := range pages {
		id := PageID(p.Name)
		if id == "" {
			return nil, &drawioapi.DrawioError{Message: fmt.Sprintf("page %q has no name to make its id", p.Name)}
		}
		if seen[id] {
			return nil, &drawioapi.DrawioError{Message: fmt.Sprintf("two pages have the id %q", id)}
		}
		seen[id] = true
		got, err := Pages(p.XML)
		if err != nil {
			return nil, fmt.Errorf("page %q: %w", p.Name, err)
		}
		inner := got[0].XML
		if bytes.Contains(inner[:min(len(inner), 512)], []byte("<mxfile")) {
			var f pageFile
			if err := xml.Unmarshal(inner, &f); err != nil || len(f.Diagrams) == 0 {
				return nil, &drawioapi.DrawioError{Message: fmt.Sprintf("page %q: no <diagram>", p.Name)}
			}
			inner = f.Diagrams[0].Inner
		}
		out.Diagrams = append(out.Diagrams, pageDiag{ID: id, Name: p.Name, Inner: inner})
	}
	return xml.Marshal(out)
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// PageID is the stable id of a page named name: its name in lower case,
// every run of other characters a dash.
func PageID(name string) string {
	return strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// pageLabel is how a finding names its page: its name, else its id.
func pageLabel(p Page) string {
	if p.Name != "" {
		return p.Name
	}
	return p.ID
}

// PageQuality is one page's quality report.
type PageQuality struct {
	Page    string                   `json:"page"`
	Quality *drawioapi.QualityReport `json:"quality"`
}

// ScorePages measures every page of a .drawio document, in order.
func ScorePages(data []byte) ([]PageQuality, error) {
	pages, err := Pages(data)
	if err != nil {
		return nil, err
	}
	out := make([]PageQuality, 0, len(pages))
	for _, p := range pages {
		r, err := archdiagram.Read(p.XML)
		if err != nil {
			return nil, fmt.Errorf("page %q: %w", pageLabel(p), err)
		}
		out = append(out, PageQuality{Page: pageLabel(p), Quality: quality.Score(r)})
	}
	return out, nil
}
