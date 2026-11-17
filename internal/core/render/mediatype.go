// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"net/http"
	"path"
	"strings"
)

// The media types display knows by name. IANA types where they exist, vendor
// types where they do not (decision).
const (
	MediaTypeMarkdown = "text/markdown"
	MediaTypeHTML     = "text/html"
	MediaTypeMermaid  = "text/vnd.mermaid"
	MediaTypePlain    = "text/plain"
	MediaTypeJSON     = "application/json"
	MediaTypeYAML     = "application/yaml"
	MediaTypeXML      = "application/xml"
	MediaTypePDF      = "application/pdf"
	MediaTypeDrawio   = "application/vnd.jgraph.mxfile"
	MediaTypeSVG      = "image/svg+xml"
	MediaTypePNG      = "image/png"
	MediaTypeDOCX     = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MediaTypePPTX     = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	// MediaTypeUI is a ui-component: the content is a component object that
	// names its own type (form, view, table, tree-table, wizard, page,
	// extension).
	MediaTypeUI = "application/vnd.ktw.ui+json"
	// MediaTypeDeck is a slide deck: markdown in the slidedeck grammar,
	// presented with reveal.js (modules/slidedeck/). It is markdown with
	// RFC 7763's `variant` parameter, so a surface without the deck engine
	// still shows it — as markdown.
	MediaTypeDeck = "text/markdown; variant=deck"
	// MediaTypeOctet is what a document is when nothing says otherwise.
	MediaTypeOctet = "application/octet-stream"
)

// extensions is THE file-extension table: the frontend has none. Longest
// suffix first where one extension ends another (.drawio.xml before .xml,
// .page.json before .json) — MediaTypeForPath tries compound suffixes first.
var extensions = []struct{ ext, mediaType string }{
	{".page.json", MediaTypeUI},
	{".drawio.xml", MediaTypeDrawio},
	{".drawio", MediaTypeDrawio},
	{".deck.md", MediaTypeDeck},
	{".md", MediaTypeMarkdown},
	{".markdown", MediaTypeMarkdown},
	{".html", MediaTypeHTML},
	{".htm", MediaTypeHTML},
	{".mmd", MediaTypeMermaid},
	{".mermaid", MediaTypeMermaid},
	{".txt", MediaTypePlain},
	{".log", MediaTypePlain},
	{".json", MediaTypeJSON},
	{".yaml", MediaTypeYAML},
	{".yml", MediaTypeYAML},
	{".xml", MediaTypeXML},
	{".css", "text/css"},
	{".js", "text/javascript"},
	{".mjs", "text/javascript"},
	{".ts", "text/x-typescript"},
	{".tsx", "text/x-tsx"},
	{".jsx", "text/x-jsx"},
	{".go", "text/x-go"},
	{".py", "text/x-python"},
	{".sh", "text/x-sh"},
	{".ps1", "text/x-powershell"},
	{".rs", "text/x-rust"},
	{".java", "text/x-java"},
	{".cs", "text/x-csharp"},
	{".sql", "text/x-sql"},
	{".toml", "text/x-toml"},
	{".csv", "text/csv"},
	{".png", "image/png"},
	{".jpg", "image/jpeg"},
	{".jpeg", "image/jpeg"},
	{".gif", "image/gif"},
	{".webp", "image/webp"},
	{".svg", "image/svg+xml"},
	{".pdf", MediaTypePDF},
	{".docx", MediaTypeDOCX},
	{".pptx", MediaTypePPTX},
	{".potx", "application/vnd.openxmlformats-officedocument.presentationml.template"},
	{".typ", "text/x-typst"},
	{".woff2", "font/woff2"},
	{".woff", "font/woff"},
	{".ttf", "font/ttf"},
	{".otf", "font/otf"},
}

// kinds maps a render.Def kind that names a DOCUMENT to its media type.
// Every other kind is a ui-component type.
var kinds = map[string]string{
	"markdown": MediaTypeMarkdown,
	"html":     MediaTypeHTML,
	"mermaid":  MediaTypeMermaid,
	"drawio":   MediaTypeDrawio,
	"deck":     MediaTypeDeck,
}

// MediaTypeForKind is the media type a Def kind displays as: a document kind's
// own type, MediaTypeUI for any component kind, "" for no kind.
func MediaTypeForKind(kind string) string {
	if kind == "" {
		return ""
	}
	if mt, ok := kinds[kind]; ok {
		return mt
	}
	return MediaTypeUI
}

// MediaTypeForPath is the media type the file extension of p says, or ""
// when the extension is unknown.
func MediaTypeForPath(p string) string {
	lp := strings.ToLower(path.Base(strings.ReplaceAll(p, `\`, "/")))
	for _, e := range extensions {
		if strings.HasSuffix(lp, e.ext) {
			return e.mediaType
		}
	}
	return ""
}

// ExtensionFor is the file extension a document of this media type saves
// with: the first extension of the table, ".txt" for an unknown textual type,
// ".bin" otherwise.
func ExtensionFor(mediaType string) string {
	base := BaseMediaType(mediaType)
	if base == MediaTypeDrawio {
		return ".drawio" // the table lists .drawio.xml first only so it wins over .xml
	}
	// A type with parameters that the table names exactly (a deck) keeps its
	// own extension; otherwise the base type decides.
	for _, e := range extensions {
		if strings.Contains(e.mediaType, ";") && sameMediaType(e.mediaType, mediaType) {
			return e.ext
		}
	}
	for _, e := range extensions {
		if e.mediaType == base {
			return e.ext
		}
	}
	if IsTextual(base) {
		return ".txt"
	}
	return ".bin"
}

// sameMediaType compares two media types ignoring case and the spaces
// around parameters.
func sameMediaType(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "")) }
	return norm(a) == norm(b)
}

// Sniff guesses a media type from the bytes, for a file whose extension says
// nothing: draw.io and ui-component JSON first, then net/http's sniffer.
func Sniff(b []byte) string {
	head := bytes.TrimSpace(b)
	if len(head) > 512 {
		head = head[:512]
	}
	switch {
	case bytes.HasPrefix(head, []byte("<mxfile")), bytes.HasPrefix(head, []byte("<mxGraphModel")):
		return MediaTypeDrawio
	case bytes.HasPrefix(head, []byte("{")), bytes.HasPrefix(head, []byte("[")):
		return MediaTypeJSON
	}
	mt := BaseMediaType(http.DetectContentType(b))
	if mt == MediaTypePlain && len(head) > 0 && head[0] == '#' {
		return MediaTypeMarkdown
	}
	return mt
}

// Resolve fixes a document's media type from what is known, in the decided
// order: the explicit value, the contribution layer's declared type, the
// file extension, the bytes.
func Resolve(explicit, layerMediaType, p string, content []byte) string {
	for _, mt := range []string{explicit, layerMediaType, MediaTypeForPath(p)} {
		if strings.TrimSpace(mt) != "" {
			return mt
		}
	}
	if len(content) > 0 {
		return Sniff(content)
	}
	return MediaTypeOctet
}

// BaseMediaType strips parameters and lower-cases: "Text/HTML; charset=utf-8"
// → "text/html".
func BaseMediaType(mt string) string {
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = mt[:i]
	}
	return strings.ToLower(strings.TrimSpace(mt))
}

// IsTextual reports whether a media type's content is text: text/*, JSON,
// YAML, XML, their +json / +xml structured suffixes, and draw.io (XML).
func IsTextual(mediaType string) bool {
	b := BaseMediaType(mediaType)
	switch {
	case strings.HasPrefix(b, "text/"),
		b == MediaTypeJSON, b == MediaTypeYAML, b == MediaTypeXML, b == "application/x-yaml", b == MediaTypeDrawio,
		strings.HasSuffix(b, "+json"), strings.HasSuffix(b, "+xml"):
		return true
	}
	return false
}

// Named is every media type display names, by its Go constant's name without
// the MediaType prefix — what the GUI's generated media-types module exports.
func Named() map[string]string {
	return map[string]string{
		"Markdown": MediaTypeMarkdown, "HTML": MediaTypeHTML, "Mermaid": MediaTypeMermaid,
		"Plain": MediaTypePlain, "JSON": MediaTypeJSON, "YAML": MediaTypeYAML, "XML": MediaTypeXML,
		"PDF": MediaTypePDF, "Drawio": MediaTypeDrawio, "UI": MediaTypeUI, "Octet": MediaTypeOctet,
		"Deck": MediaTypeDeck, "SVG": MediaTypeSVG, "PNG": MediaTypePNG, "DOCX": MediaTypeDOCX, "PPTX": MediaTypePPTX,
	}
}

// DocumentKinds is the kind → media type table of the Def kinds that name a
// document (a copy: callers cannot change the table).
func DocumentKinds() map[string]string {
	out := make(map[string]string, len(kinds))
	for k, v := range kinds {
		out[k] = v
	}
	return out
}
