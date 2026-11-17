// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// « Contenu lu » keeps its head in view while its rows scroll, draws a read
// row's open mark as display's external-link icon before its title, and opens
// only a read row (mock-up v6).
func TestReadTableHasStickyHeadAndOpenIcon(t *testing.T) {
	schema := readSchema(nil)
	types, _ := schema["types"].(map[string]any)
	root, _ := types["ReadItem"].(map[string]any)
	fields, _ := root["fields"].(map[string]any)
	open, _ := fields["open"].(map[string]any)
	if open["content"] != "icon" {
		t.Fatalf("open field content = %v, want icon", open["content"])
	}
	exts, _ := root["extensions"].([]any)
	if len(exts) != 1 {
		t.Fatalf("extensions = %v", exts)
	}
	ext, _ := exts[0].(map[string]any)
	if ext["type"] != "ui-table" || ext["stickyHeader"] != true {
		t.Fatalf("ui-table extension = %v, want stickyHeader true", ext)
	}

	row := map[string]any{}
	readColumns(row, documentFields{}, api.ConnectorDecl{}, "")
	if row["open"] != "external-link" {
		t.Fatalf("read row open = %v, want external-link", row["open"])
	}

	tree := sourceDetailTree(map[string]any{"items": []any{map[string]any{"title": "a"}}}, false)
	children, _ := tree["children"].([]any)
	var table render.Node
	for _, c := range children {
		if m, ok := c.(render.Node); ok && m["kind"] == "table" {
			table = m
		}
	}
	if table == nil {
		t.Fatal("no « Contenu lu » table")
	}
	act, _ := table["open"].(map[string]any)
	if act["when"] != "doc" {
		t.Fatalf("table open when = %v, want doc (a row not read does not open)", act["when"])
	}
}
