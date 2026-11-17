// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A contributed engine reaches the GUI registry only when it can be used:
// something to accept (its widget page is a layer, fetched when needed).
func TestContributedEnginesKeepUsableDeclarations(t *testing.T) {
	items := []mbapi.Attributed[render.EngineDecl]{
		{Provider: "ktw-acme", URI: "ktw-acme:/display-engine/landscape", Body: render.EngineDecl{
			Label: "Landscape", Accepts: []string{"application/vnd.leanix.landscape+json"},
			Capabilities: render.EngineCapabilities{Edit: true, Export: []render.ExportFormat{{MediaType: "image/svg+xml", Label: "SVG"}}},
		}},
		{Provider: "ktw-b", URI: "ktw-b:/display-engine/no-types", Body: render.EngineDecl{Label: "nothing to accept"}},
	}
	got := contributedEngines(items)
	if len(got) != 1 {
		t.Fatalf("got %d engines, want 1: %+v", len(got), got)
	}
	e := got[0]
	if e.ID != "landscape" || e.Provider != "ktw-acme" || !e.Capabilities.Edit || len(e.Capabilities.Export) != 1 || e.Accepts[0] != "application/vnd.leanix.landscape+json" {
		t.Errorf("engine = %+v", e)
	}
}

// DocumentOpen types an inline document it is handed without a media type,
// and reads an absolute local file through the one resolver.
func TestDocumentOpenResolvesLocalFile(t *testing.T) {
	a := &App{}
	dir := t.TempDir()
	p := dir + "/flow.drawio"
	if err := writeFile(p, "<mxfile/>"); err != nil {
		t.Fatal(err)
	}
	r := a.DocumentOpen("", "", render.Document{Source: &render.Source{Path: p}})
	if r.Error != "" {
		t.Fatalf("DocumentOpen: %s", r.Error)
	}
	if r.Document.MediaType != render.MediaTypeDrawio || r.Document.Content != "<mxfile/>" || r.Document.Title != "flow.drawio" || r.Document.Source != nil {
		t.Errorf("document = %+v", r.Document)
	}
	png := dir + "/logo.png"
	if err := writeFile(png, "\x89PNG\r\n\x1a\n"); err != nil {
		t.Fatal(err)
	}
	r = a.DocumentOpen("", "", render.Document{Source: &render.Source{Path: png}})
	if r.Document.MediaType != "image/png" || r.Document.Encoding != "base64" {
		t.Errorf("png document = %+v", r.Document)
	}
	if r := a.DocumentOpen("", "", render.Document{Source: &render.Source{Path: "relative.md"}}); r.Error == "" {
		t.Error("a relative path without its node must be refused")
	}
}

func writeFile(p, s string) error { return os.WriteFile(p, []byte(s), 0o644) }

// An inline document without a media type is typed by its title (its file
// name), then by its bytes.
func TestDocumentOpenTypesInlineContent(t *testing.T) {
	a := &App{}
	if got := a.DocumentOpen("", "", render.Document{Title: "overview.page.json", Content: `{"type":"stack"}`}).Document.MediaType; got != render.MediaTypeUI {
		t.Errorf("page file = %q, want %q", got, render.MediaTypeUI)
	}
	if got := a.DocumentOpen("", "", render.Document{Title: "overview.md", Content: "text"}).Document.MediaType; got != render.MediaTypeMarkdown {
		t.Errorf("markdown file = %q", got)
	}
	if got := a.DocumentOpen("", "", render.Document{Content: "<mxfile/>"}).Document.MediaType; got != render.MediaTypeDrawio {
		t.Errorf("sniffed = %q", got)
	}
}

// Every export an engine offers from the server has a route, and the file it
// produces is named after the source with the target's extension.
func TestExportRoutes(t *testing.T) {
	for _, c := range []struct{ from, to, method, format string }{
		{render.MediaTypeMarkdown, render.MediaTypePDF, "render", "pdf"},
		{render.MediaTypeMarkdown, render.MediaTypeDOCX, "render", "docx"},
		{render.MediaTypeMarkdown, render.MediaTypePPTX, "render", "pptx"},
		{render.MediaTypeMarkdown + "; charset=utf-8", render.MediaTypeHTML, "render", "html"},
		{render.MediaTypeMermaid, render.MediaTypePNG, "diagram", "png"},
		{render.MediaTypeDrawio, render.MediaTypeSVG, "diagram", "svg"},
		{render.MediaTypeDrawio, render.MediaTypePDF, "diagram", "pdf"},
	} {
		r, ok := exportRouteFor(c.from, c.to)
		if !ok || r.method != c.method || r.format != c.format {
			t.Errorf("%s → %s = %+v %v", c.from, c.to, r, ok)
		}
	}
	if _, ok := exportRouteFor(render.MediaTypeHTML, render.MediaTypeDOCX); ok {
		t.Error("no route was declared from html to docx")
	}
	if got := exportTitle("report.md", render.MediaTypeMarkdown, render.MediaTypeDOCX); got != "report.docx" {
		t.Errorf("exportTitle = %q", got)
	}
	if got := exportTitle("", render.MediaTypeDrawio, render.MediaTypePNG); got != "document.png" {
		t.Errorf("exportTitle empty = %q", got)
	}
}
