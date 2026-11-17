// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// DocumentText is how a terminal shows a document, which has no display
// engines: markdown through glamour; text, code, JSON and YAML as they are
// (JSON indented); any other media type as one line naming the title, the
// media type and the size, with a pointer to the GUI.
func DocumentText(doc render.Document, width int) string {
	body := doc.Text()
	mt := render.BaseMediaType(doc.MediaType)
	switch {
	case mt == render.MediaTypeMarkdown:
		return RenderMarkdownWidth(documentHeading(doc)+body, width)
	case mt == render.MediaTypeJSON:
		var v any
		if json.Unmarshal([]byte(body), &v) == nil {
			if b, err := json.MarshalIndent(v, "", "  "); err == nil {
				body = string(b)
			}
		}
		return documentTitleLine(doc) + body
	case doc.Encoding == "" && render.IsTextual(mt) &&
		mt != render.MediaTypeHTML && mt != render.MediaTypeUI && mt != render.MediaTypeDrawio &&
		!strings.HasSuffix(mt, "+json") && !strings.HasSuffix(mt, "+xml"):
		return documentTitleLine(doc) + body
	default:
		return documentPlaceholder(doc)
	}
}

func documentHeading(doc render.Document) string {
	if doc.Title == "" {
		return ""
	}
	return "# " + doc.Title + "\n\n"
}

func documentTitleLine(doc render.Document) string {
	if doc.Title == "" {
		return ""
	}
	return "── " + doc.Title + " ──\n"
}

// documentPlaceholder is the one line a document no terminal can draw
// becomes.
func documentPlaceholder(doc render.Document) string {
	title := doc.Title
	if title == "" {
		title = "document"
	}
	size := len(doc.Text())
	if doc.Encoding == "base64" {
		size = size * 3 / 4
	}
	return fmt.Sprintf("[%s — %s, %.1f KB — open in the GUI]", title, render.BaseMediaType(doc.MediaType), float64(size)/1024)
}
