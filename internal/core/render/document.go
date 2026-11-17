// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Document is the envelope every producer hands to display: what the content
// is (its media type), and the content itself or where to read it. The media
// type alone picks the display engine; no consumer switches on a format or a
// kind string (modules/display/document-display.md).
//
// A ui-component document (MediaTypeUI) carries the component object as its
// Content — the fields of a Def, whose Kind names the component type.
type Document struct {
	MediaType string         `json:"mediaType" yaml:"mediaType" jsonschema:"description=What the content is and so which engine shows it: text/markdown | text/html | text/vnd.mermaid | application/vnd.jgraph.mxfile | application/vnd.ktw.ui+json | application/json | image/png | application/pdf | …"`
	Title     string         `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Tab label and default file name on save"`
	Content   any            `json:"content,omitempty" yaml:"content,omitempty" jsonschema:"description=Inline content: a string for a textual media type or a JSON value for a +json one. Exclusive with source"`
	Encoding  string         `json:"encoding,omitempty" yaml:"encoding,omitempty" jsonschema:"description=Empty for UTF-8 text; base64 for inline binary content (image or pdf)"`
	Source    *Source        `json:"source,omitempty" yaml:"source,omitempty" jsonschema:"description=Where the content is when it is not inline. Exclusive with content"`
	Meta      map[string]any `json:"meta,omitempty" yaml:"meta,omitempty" jsonschema:"description=Free metadata"`
}

// Source says where a document's content is read from.
type Source struct {
	Path  string `json:"path,omitempty" yaml:"path,omitempty" jsonschema:"description=A file: absolute or relative to the workspace"`
	URI   string `json:"uri,omitempty" yaml:"uri,omitempty" jsonschema:"description=A resource URI provider:/type/path"`
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty" jsonschema:"description=The contribution layer of uri to read; empty for the kind's default layer"`
}

// Text returns the content as text: the string itself, or the JSON of a
// structured value. Empty when there is no inline content.
func (d Document) Text() string {
	switch c := d.Content.(type) {
	case nil:
		return ""
	case string:
		return c
	case []byte:
		return string(c)
	default:
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return fmt.Sprint(c)
		}
		return string(b)
	}
}

// componentFields are the Def fields a ui-component document's content
// carries; the rest (MediaType, File, Title) belong to the envelope or
// the host.
func (d Def) component() map[string]any {
	raw, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"mediaType", "file", "title"} {
		delete(m, k)
	}
	return m
}

// Document turns a render definition into the envelope the display entry
// point takes. A Def naming a media type, or a document kind (markdown,
// drawio, …), is that document, its content from Data or its source
// from File; any other kind is a ui-component whose content is the Def's
// component fields.
func (d Def) Document() Document {
	mt := d.MediaType
	if mt == "" {
		mt = MediaTypeForKind(d.Kind)
	}
	if mt == "" && d.File != "" {
		mt = MediaTypeForPath(d.File)
	}
	doc := Document{MediaType: mt, Title: d.Title}
	if BaseMediaType(mt) == MediaTypeUI {
		doc.Content = d.component()
		return doc
	}
	if d.File != "" {
		doc.Source = &Source{Path: d.File}
		return doc
	}
	doc.Content = documentContent(mt, d.Data)
	return doc
}

// documentContent unwraps the object shapes a document kind still takes
// inline: drawio's {xml, filename}, whose content is the XML, and a deck's
// {markdown}, whose content is the markdown.
func documentContent(mt string, v any) any {
	field := ""
	switch {
	case BaseMediaType(mt) == MediaTypeDrawio:
		field = "xml"
	case sameMediaType(mt, MediaTypeDeck):
		field = "markdown"
	default:
		return v
	}
	if m, ok := v.(map[string]any); ok {
		if x, ok := m[field].(string); ok {
			return x
		}
	}
	return v
}

// Validate refuses an envelope with no media type, or with both inline
// content and a source.
func (d Document) Validate() error {
	if strings.TrimSpace(d.MediaType) == "" {
		return fmt.Errorf("document: mediaType is required")
	}
	if d.Content != nil && d.Source != nil {
		return fmt.Errorf("document: content and source are exclusive")
	}
	if d.Encoding != "" && d.Encoding != "base64" {
		return fmt.Errorf("document: encoding %q (want empty or base64)", d.Encoding)
	}
	return nil
}
