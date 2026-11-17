// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

func TestDocumentTextDegradesByMediaType(t *testing.T) {
	cases := []struct {
		name      string
		doc       render.Document
		contains  []string
		placehold bool
	}{
		{"markdown keeps its text", render.Document{MediaType: "text/markdown", Title: "Report", Content: "Hello **world**"}, []string{"Report", "world"}, false},
		{"json is indented", render.Document{MediaType: "application/json", Content: `{"a":1}`}, []string{"{\n  \"a\": 1\n}"}, false},
		{"code as is", render.Document{MediaType: "text/x-go", Title: "main.go", Content: "package main"}, []string{"main.go", "package main"}, false},
		{"html is a placeholder", render.Document{MediaType: "text/html", Title: "Dash", Content: "<p>x</p>"}, []string{"Dash", "text/html", "open in the GUI"}, true},
		{"drawio is a placeholder", render.Document{MediaType: render.MediaTypeDrawio, Title: "Flow", Content: "<mxfile/>"}, []string{"Flow", "application/vnd.jgraph.mxfile"}, true},
		{"ui component is a placeholder", render.Document{MediaType: render.MediaTypeUI, Title: "form a1"}, []string{"form a1", "application/vnd.ktw.ui+json"}, true},
		{"binary is a placeholder", render.Document{MediaType: "image/png", Title: "logo", Content: "iVBORw0KGgo=", Encoding: "base64"}, []string{"logo", "image/png"}, true},
	}
	for _, c := range cases {
		got := DocumentText(c.doc, 80)
		for _, want := range c.contains {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %q does not contain %q", c.name, got, want)
			}
		}
		if isPlaceholder := strings.HasPrefix(got, "["); isPlaceholder != c.placehold {
			t.Errorf("%s: placeholder = %v, want %v (%q)", c.name, isPlaceholder, c.placehold, got)
		}
	}
}
