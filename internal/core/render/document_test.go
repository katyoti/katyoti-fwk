// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

func TestMediaTypeForPath(t *testing.T) {
	for p, want := range map[string]string{
		"docs/overview.md":          render.MediaTypeMarkdown,
		"decks/q4-review.deck.md":   render.MediaTypeDeck,
		`C:\x\Diagram.DRAWIO`:       render.MediaTypeDrawio,
		"a/b.drawio.xml":            render.MediaTypeDrawio,
		"plain.xml":                 render.MediaTypeXML,
		"overview.page.json":        render.MediaTypeUI,
		"data.json":                 render.MediaTypeJSON,
		"flow.mmd":                  render.MediaTypeMermaid,
		"main.go":                   "text/x-go",
		"logo.svg":                  "image/svg+xml",
		"spec.pdf":                  render.MediaTypePDF,
		"Makefile":                  "",
		"archive.unknown-extension": "",
	} {
		if got := render.MediaTypeForPath(p); got != want {
			t.Errorf("MediaTypeForPath(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestMediaTypeForKind(t *testing.T) {
	for kind, want := range map[string]string{
		"markdown": render.MediaTypeMarkdown, "html": render.MediaTypeHTML,
		"mermaid": render.MediaTypeMermaid, "drawio": render.MediaTypeDrawio,
		"deck": render.MediaTypeDeck,
		"form": render.MediaTypeUI, "tree-table": render.MediaTypeUI,
		"page": render.MediaTypeUI, "extension": render.MediaTypeUI, "": "",
	} {
		if got := render.MediaTypeForKind(kind); got != want {
			t.Errorf("MediaTypeForKind(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestExtensionForRoundTrips(t *testing.T) {
	for mt, want := range map[string]string{
		render.MediaTypeMarkdown:         ".md",
		render.MediaTypeDrawio:           ".drawio",
		render.MediaTypeDeck:             ".deck.md",
		"text/markdown;variant=DECK":     ".deck.md",
		"text/html; charset=utf-8":       ".html",
		render.MediaTypeUI:               ".page.json",
		"text/x-unknown-lang":            ".txt",
		"application/x-something-binary": ".bin",
	} {
		if got := render.ExtensionFor(mt); got != want {
			t.Errorf("ExtensionFor(%q) = %q, want %q", mt, got, want)
		}
	}
}

func TestResolveOrder(t *testing.T) {
	cases := []struct {
		name, explicit, layer, path string
		content                     string
		want                        string
	}{
		{"explicit wins", "text/html", "text/markdown", "a.md", "", "text/html"},
		{"layer before extension", "", "application/yaml", "a.md", "", "application/yaml"},
		{"extension before sniff", "", "", "a.md", "<mxfile/>", render.MediaTypeMarkdown},
		{"sniff drawio", "", "", "noext", "  <mxfile host='x'>", render.MediaTypeDrawio},
		{"sniff json", "", "", "noext", `{"a":1}`, render.MediaTypeJSON},
		{"sniff markdown heading", "", "", "README", "# Title\n\ntext", render.MediaTypeMarkdown},
		{"sniff plain", "", "", "LICENSE", "Apache License", render.MediaTypePlain},
		{"nothing known", "", "", "", "", render.MediaTypeOctet},
	}
	for _, c := range cases {
		if got := render.Resolve(c.explicit, c.layer, c.path, []byte(c.content)); got != c.want {
			t.Errorf("%s: Resolve = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIsTextual(t *testing.T) {
	for mt, want := range map[string]bool{
		"text/x-go": true, render.MediaTypeJSON: true, render.MediaTypeYAML: true,
		render.MediaTypeUI: true, "image/svg+xml": true, render.MediaTypeDrawio: true,
		"image/png": false, render.MediaTypePDF: false, render.MediaTypeOctet: false,
	} {
		if got := render.IsTextual(mt); got != want {
			t.Errorf("IsTextual(%q) = %v, want %v", mt, got, want)
		}
	}
}

func TestDefDocument(t *testing.T) {
	// A component kind is a ui-component whose content is the component object.
	form := render.Def{Kind: "form", Schema: "ktw-demo:/data-type/book", Variant: "compact", Mode: "edit", Title: "Book"}.Document()
	if form.MediaType != render.MediaTypeUI || form.Title != "Book" {
		t.Fatalf("form document = %+v", form)
	}
	c, ok := form.Content.(map[string]any)
	if !ok || c["kind"] != "form" || c["schema"] != "ktw-demo:/data-type/book" || c["variant"] != "compact" || c["mode"] != "edit" {
		t.Errorf("form content = %#v", form.Content)
	}
	if _, has := c["title"]; has {
		t.Error("the title belongs to the envelope, not to the component")
	}

	// A document kind from a file is a source, typed by the kind.
	md := render.Def{Kind: "markdown", File: "docs/a.md"}.Document()
	if md.MediaType != render.MediaTypeMarkdown || md.Source == nil || md.Source.Path != "docs/a.md" || md.Content != nil {
		t.Errorf("markdown file document = %+v", md)
	}

	// drawio's inline {xml, filename} becomes the XML.
	dio := render.Def{Kind: "drawio", Data: map[string]any{"xml": "<mxfile/>", "filename": "x.drawio"}}.Document()
	if dio.MediaType != render.MediaTypeDrawio || dio.Content != "<mxfile/>" {
		t.Errorf("drawio document = %+v", dio)
	}

	// An explicit media type needs no kind; a file with neither is typed by its extension.
	pdf := render.Def{MediaType: render.MediaTypePDF, File: "x.pdf"}.Document()
	if pdf.MediaType != render.MediaTypePDF || pdf.Source.Path != "x.pdf" {
		t.Errorf("pdf document = %+v", pdf)
	}
	if got := (render.Def{File: "main.go"}).Document().MediaType; got != "text/x-go" {
		t.Errorf("file-only Def media type = %q", got)
	}
}

func TestDocumentValidate(t *testing.T) {
	if err := (render.Document{}).Validate(); err == nil {
		t.Error("a document without a media type must be refused")
	}
	both := render.Document{MediaType: "text/plain", Content: "x", Source: &render.Source{Path: "a"}}
	if err := both.Validate(); err == nil {
		t.Error("content and source together must be refused")
	}
	if err := (render.Document{MediaType: "image/png", Encoding: "hex"}).Validate(); err == nil {
		t.Error("an unknown encoding must be refused")
	}
	if err := (render.Document{MediaType: "text/markdown", Content: "# x"}).Validate(); err != nil {
		t.Errorf("valid document refused: %v", err)
	}
}

// A deck is a document kind: its markdown is the content, inline or from a
// {markdown} object, and a .deck.md file is its source.
func TestDefDocumentDeck(t *testing.T) {
	inline := render.Def{Kind: "deck", Data: map[string]any{"markdown": "---\ntitle: T\n---\n# One"}}.Document()
	if inline.MediaType != render.MediaTypeDeck || inline.Content != "---\ntitle: T\n---\n# One" {
		t.Fatalf("inline deck: %+v", inline)
	}
	file := render.Def{File: "decks/q4.deck.md"}.Document()
	if file.MediaType != render.MediaTypeDeck || file.Source == nil || file.Source.Path != "decks/q4.deck.md" {
		t.Fatalf("file deck: %+v", file)
	}
}
