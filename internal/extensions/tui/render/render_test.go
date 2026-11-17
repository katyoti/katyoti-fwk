// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tui/render"
)

// TestRender_NoAST_FallsBackToJSON covers the common AutoKTSP case
// where output types have no ui-cli extension attached. The renderer
// must produce pretty-JSON identical to today's writeOutput path so
// the rollout is invisible to non-opted-in methods.
func TestRender_NoAST_FallsBackToJSON(t *testing.T) {
	var buf bytes.Buffer
	body := json.RawMessage(`{"name":"foo","count":3}`)
	if err := render.Render(nil, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"name": "foo"`) {
		t.Errorf("expected pretty JSON, got: %s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("expected trailing newline")
	}
}

// TestRender_TextMode renders an object's fields one-per-line.
func TestRender_TextMode(t *testing.T) {
	ast := &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Extensions: []schemaapi.Extension{{
			Kind:    schemaapi.ExtensionUICLI,
			Payload: map[string]any{"mode": "text", "primary": "name"},
		}},
		Fields: map[string]*schemaapi.Type{
			"name":  {Kind: schemaapi.KindString},
			"score": {Kind: schemaapi.KindNumber},
		},
	}
	body := json.RawMessage(`{"name":"alpha","score":42}`)

	var buf bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") || !strings.Contains(out, "alpha") {
		t.Errorf("expected name/alpha in output, got: %s", out)
	}
	if !strings.Contains(out, "score") || !strings.Contains(out, "42") {
		t.Errorf("expected score/42 in output, got: %s", out)
	}
}

// TestRender_TableMode renders an array of objects as a column-aligned
// table with header + separator.
func TestRender_TableMode(t *testing.T) {
	ast := &schemaapi.Type{
		Kind: schemaapi.KindArray,
		Extensions: []schemaapi.Extension{{
			Kind: schemaapi.ExtensionUICLI,
			Payload: map[string]any{
				"mode":    "table",
				"columns": []any{"name", "score"},
			},
		}},
		Items: &schemaapi.TypeRef{Inline: &schemaapi.Type{
			Kind: schemaapi.KindObject,
			Fields: map[string]*schemaapi.Type{
				"name":  {Kind: schemaapi.KindString},
				"score": {Kind: schemaapi.KindNumber},
			},
		}},
	}
	body := json.RawMessage(`[
		{"name":"alpha","score":42},
		{"name":"beta","score":7}
	]`)

	var buf bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") || !strings.Contains(out, "score") {
		t.Errorf("expected header in output, got: %s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "42") {
		t.Errorf("expected row data, got: %s", out)
	}
	if !strings.Contains(out, "----") {
		t.Errorf("expected separator line, got: %s", out)
	}
}

// TestRender_TableMode_AutoExtractArray handles the common AutoKTSP
// shape `{items: [...]}` where the response wraps the array in a
// single-field envelope. The renderer should look through.
func TestRender_TableMode_AutoExtractArray(t *testing.T) {
	body := json.RawMessage(`{"items":[
		{"name":"alpha","score":42},
		{"name":"beta","score":7}
	]}`)

	var buf bytes.Buffer
	ctx := render.RenderCtx{ModeOverride: "table"}
	if err := render.Render(nil, body, ctx, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Errorf("expected envelope's array to be rendered, got: %s", out)
	}
}

// TestRender_ModeOverride_JSON forces JSON regardless of ui-cli.
func TestRender_ModeOverride_JSON(t *testing.T) {
	ast := &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Extensions: []schemaapi.Extension{{
			Kind:    schemaapi.ExtensionUICLI,
			Payload: map[string]any{"mode": "text"},
		}},
	}
	body := json.RawMessage(`{"name":"alpha"}`)

	var buf bytes.Buffer
	ctx := render.RenderCtx{ModeOverride: "json"}
	if err := render.Render(ast, body, ctx, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"name": "alpha"`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
	if strings.Contains(out, " : ") {
		t.Errorf("text mode markers should not appear when JSON forced: %s", out)
	}
}

// TestRender_RichOnlyFields_DropPlain confirms that fields tagged
// rich-only get omitted when the renderer context is plain (non-TTY).
func TestRender_RichOnlyFields_DropPlain(t *testing.T) {
	ast := &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Extensions: []schemaapi.Extension{{
			Kind: schemaapi.ExtensionUICLI,
			Payload: map[string]any{
				"mode":             "text",
				"rich-only-fields": []any{"icon"},
			},
		}},
		Fields: map[string]*schemaapi.Type{
			"name": {Kind: schemaapi.KindString},
			"icon": {Kind: schemaapi.KindString},
		},
	}
	body := json.RawMessage(`{"name":"alpha","icon":"⭐"}`)

	// Plain context: rich-only fields dropped.
	var plain bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &plain); err != nil {
		t.Fatalf("Render plain: %v", err)
	}
	if strings.Contains(plain.String(), "icon") {
		t.Errorf("rich-only field should be dropped in plain mode, got: %s", plain.String())
	}
	if !strings.Contains(plain.String(), "alpha") {
		t.Errorf("expected name in output, got: %s", plain.String())
	}

	// TTY + color context: rich-only fields kept.
	var tty bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{IsTTY: true, Color: true}, &tty); err != nil {
		t.Fatalf("Render tty: %v", err)
	}
	if !strings.Contains(tty.String(), "icon") {
		t.Errorf("rich-only field should be present in TTY+color mode, got: %s", tty.String())
	}
}

// TestRender_TreeMode walks a recursive structure.
func TestRender_TreeMode(t *testing.T) {
	ast := &schemaapi.Type{
		Kind: schemaapi.KindObject,
		Extensions: []schemaapi.Extension{{
			Kind: schemaapi.ExtensionUICLI,
			Payload: map[string]any{
				"mode":             "tree",
				"primary":          "name",
				"tree-child-field": "children",
			},
		}},
	}
	body := json.RawMessage(`{
		"name":"root",
		"children":[
			{"name":"alpha","children":[]},
			{"name":"beta","children":[
				{"name":"gamma","children":[]}
			]}
		]
	}`)

	var buf bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"root", "alpha", "beta", "gamma", "├──", "└──"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in tree output, got: %s", want, out)
		}
	}
}

// TestRender_TableMode_EmptyArray confirms graceful handling of empty
// result sets (the "(no rows)" sentinel matches today's hand-coded
// outputs).
func TestRender_TableMode_EmptyArray(t *testing.T) {
	body := json.RawMessage(`[]`)
	var buf bytes.Buffer
	ctx := render.RenderCtx{ModeOverride: "table"}
	if err := render.Render(nil, body, ctx, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "no rows") {
		t.Errorf("expected '(no rows)' sentinel, got: %s", buf.String())
	}
}

// TestUICLIFromType_DSLPayload confirms the typed view extracts cleanly
// from a DSL-parsed Extension payload (where slice values are []any).
func TestUICLIFromType_DSLPayload(t *testing.T) {
	ast := &schemaapi.Type{
		Extensions: []schemaapi.Extension{{
			Kind: schemaapi.ExtensionUICLI,
			Payload: map[string]any{
				"mode":             "table",
				"columns":          []any{"a", "b"},
				"rich-only-fields": []any{"c"},
				"primary":          "a",
			},
		}},
	}
	cli, ok := schemaapi.UICLIFromType(ast)
	if !ok {
		t.Fatal("UICLIFromType returned !ok")
	}
	if cli.Mode != schemaapi.UICLIModeTable {
		t.Errorf("Mode = %q, want table", cli.Mode)
	}
	if len(cli.Columns) != 2 || cli.Columns[0] != "a" || cli.Columns[1] != "b" {
		t.Errorf("Columns = %v, want [a, b]", cli.Columns)
	}
	if cli.Primary != "a" {
		t.Errorf("Primary = %q, want a", cli.Primary)
	}
	if len(cli.RichOnlyFields) != 1 || cli.RichOnlyFields[0] != "c" {
		t.Errorf("RichOnlyFields = %v, want [c]", cli.RichOnlyFields)
	}
}

// TestUICLIFromType_StructTagPayload confirms the typed view works
// when payload was populated via the Go-struct-tag reflector
// (string-form comma-separated values).
func TestUICLIFromType_StructTagPayload(t *testing.T) {
	ast := &schemaapi.Type{
		Extensions: []schemaapi.Extension{{
			Kind: schemaapi.ExtensionUICLI,
			Payload: map[string]any{
				"mode":    "table",
				"columns": "a, b, c",
				"primary": "a",
			},
		}},
	}
	cli, ok := schemaapi.UICLIFromType(ast)
	if !ok {
		t.Fatal("UICLIFromType returned !ok")
	}
	if len(cli.Columns) != 3 {
		t.Errorf("Columns = %v, want 3 entries", cli.Columns)
	}
}
