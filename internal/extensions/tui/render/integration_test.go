// SPDX-License-Identifier: Apache-2.0

package render_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tui/render"
)

// Output type used by the integration test below. Mirrors the shape of
// real AutoKTSP output structs (single array field + ui-cli on the
// array). Living in the test package keeps the renderer free of any
// dep on concrete output types.
type integrationListOutput struct {
	Scripts []integrationScriptInfo `json:"scripts" ui-cli:"mode=table,columns=name;provider;ext,primary=name,sort=provider;name"`
}

type integrationScriptInfo struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Ext      string `json:"ext"`
}

// TestIntegration_GoStruct_To_AST_To_Renderer proves the full chain
// works: a Go output struct with a `ui-cli` struct tag → reflect.TypeASTForType
// → schemaapi.Type AST → render.Render → ASCII table on stdout.
//
// This is the demo path 3L delivers: data-type authors opt in by
// tagging a Go field; AutoKTSP's eager-init phase captures the AST;
// the renderer dispatches accordingly at command-run time.
func TestIntegration_GoStruct_To_AST_To_Renderer(t *testing.T) {
	// Step 1 — reflect.TypeASTForType extracts the AST from the Go type,
	// including the ui-cli field tag.
	ast, err := corereflect.TypeASTForType(reflect.TypeOf(integrationListOutput{}))
	if err != nil {
		t.Fatalf("TypeASTForType: %v", err)
	}

	// Step 2 — body JSON in the real envelope shape.
	body := json.RawMessage(`{
		"scripts":[
			{"name":"hello","provider":"ktw-docs","ext":".sh"},
			{"name":"build","provider":"ktw-docs","ext":".ts"},
			{"name":"deploy","provider":"ktw-example","ext":".py"}
		]
	}`)

	// Step 3 — Render. The renderer must:
	//   (a) recognize the single-array-field envelope and descend into
	//       the Scripts field's AST,
	//   (b) read the ui-cli extension off the field,
	//   (c) emit a table with the declared columns + sort order.
	var buf bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	// Expectations: header row + separator + 3 data rows sorted by
	// (provider, name).
	for _, want := range []string{"name", "provider", "ext", "----", "hello", "build", "deploy"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in table output, got:\n%s", want, out)
		}
	}

	// Sort assertion: provider=ktw-docs rows precede provider=ktw-example.
	docsIdx := strings.Index(out, "ktw-docs")
	leanixIdx := strings.Index(out, "ktw-example")
	if docsIdx < 0 || leanixIdx < 0 {
		t.Fatalf("provider names missing from output:\n%s", out)
	}
	if docsIdx > leanixIdx {
		t.Errorf("expected ktw-docs rows before ktw-example rows (sort=provider;name); got order:\n%s", out)
	}

	// Within ktw-docs rows: name=build precedes name=hello (alphabetical).
	buildIdx := strings.Index(out, "build")
	helloIdx := strings.Index(out, "hello")
	if buildIdx < 0 || helloIdx < 0 {
		t.Fatalf("name values missing from output:\n%s", out)
	}
	if buildIdx > helloIdx {
		t.Errorf("expected name=build before name=hello (secondary sort); got order:\n%s", out)
	}
}

// TestIntegration_PaginatedEnvelope confirms the SearchOutput-style
// shape (one array field + N scalar metadata fields) flows correctly
// through unwrapSingleArrayField + extractSingleArray.
func TestIntegration_PaginatedEnvelope(t *testing.T) {
	type result struct {
		Rank int    `json:"rank"`
		ID   string `json:"id"`
	}
	type paged struct {
		Results   []result `json:"results" ui-cli:"mode=table,columns=rank;id"`
		Total     int      `json:"total"`
		Truncated bool     `json:"truncated,omitempty"`
	}

	ast, err := corereflect.TypeASTForType(reflect.TypeOf(paged{}))
	if err != nil {
		t.Fatalf("TypeASTForType: %v", err)
	}
	body := json.RawMessage(`{
		"results":[{"rank":1,"id":"abc"},{"rank":2,"id":"def"}],
		"total":2,
		"truncated":false
	}`)
	var buf bytes.Buffer
	if err := render.Render(ast, body, render.RenderCtx{}, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "abc") || !strings.Contains(out, "def") {
		t.Errorf("expected paginated rows, got:\n%s", out)
	}
	// Should NOT contain the scalar metadata in the table output —
	// the renderer descended past the envelope.
	if strings.Contains(out, "true") || strings.Contains(out, "false") {
		t.Errorf("renderer leaked scalar metadata into table output:\n%s", out)
	}
}
