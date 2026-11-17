// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The Trame panel shows a template's parts with its included groups expanded
// and marked; an include not in reach leaves its own sections, with why.
func TestTemplatePartsExpandTheIncludedGroups(t *testing.T) {
	risks := docsapi.ContentTemplate{Name: "risks", Description: "r", Partial: true, Sections: []docsapi.TemplateSection{{ID: "risks", Title: "Risks", Intent: "i"}}}
	board := docsapi.ContentTemplate{Name: "board", Description: "b", Sections: []docsapi.TemplateSection{{ID: "ask", Title: "Ask", Intent: "i"}, {ID: "r", Include: "risks"}}}

	got := templateParts(board, []mbapi.Attributed[docsapi.ContentTemplate]{{Provider: "ktw-docs", Body: risks}})
	if got.ExpandError != "" || len(got.Parts) != 2 || got.Parts[1].Section.ID != "r.risks" || strings.Join(got.Parts[1].Groups, "/") != "risks" || len(got.Parts[0].Groups) != 0 {
		t.Fatalf("expanded: %+v", got)
	}

	alone := templateParts(board, nil)
	if alone.ExpandError == "" || len(alone.Parts) != 2 || alone.Parts[1].Section.Include != "risks" {
		t.Fatalf("not in reach: %+v", alone)
	}
}
