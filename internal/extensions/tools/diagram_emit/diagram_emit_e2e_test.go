// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

// ctxWithFakeCatalog returns a context whose catalog holds ktw-docs'
// shipped drawio content — the notations, viewpoints and template
// libraries a live ktw-docs publishes.
func ctxWithFakeCatalog(t *testing.T) context.Context {
	t.Helper()
	return drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
}

// TestE2E_AppWithXLS is the headline end-to-end smoke. Drives the
// diagram-emit tool's handler through the full chain — viewpoint
// resolution → validation → layout (skipped via NoLayout) → the app-flux
// template library. Asserts the rendered XML carries every signature of a
// correct render:
//
//   - fillColor=#d5e8d4   (binding: attributes.domain="reference" in the palette)
//   - strokeColor=#107C41 (binding: the xls marker)
//   - strokeWidth=3       (binding: the xls marker)
//   - label="XLS"         (the badge part, kept by its ktw-when)
//   - <b>Billing</b>        (the template's label)
//
// Also asserts: no errors, and only the lint's orphan warning.
func TestE2E_AppWithXLS(t *testing.T) {
	ctx := ctxWithFakeCatalog(t)

	// Build a 1-Application diagram input. The data fields mirror what
	// an LLM would emit: domain + markers + (display-only) description.
	diagramJSON := `{
		"viewpoint": "drawio-test:/viewpoint/app-flux",
		"title": "App flux smoke",
		"nodes": [{
			"id": "billing",
			"label": "Billing",
			"kind": "Application",
			"attributes": {
				"domain": "reference",
				"markers": ["xls"],
				"description": "Invoices and prices"
			}
		}]
	}`

	out, err := executeDiagramEmit(ctx, Input{
		Diagram:  mustLogical(t, diagramJSON),
		NoLayout: true,
	})
	if err != nil {
		t.Fatalf("executeDiagramEmit: %v", err)
	}

	if len(out.Errors) != 0 {
		t.Errorf("unexpected errors: %v", out.Errors)
	}
	// A one-node diagram is linted as an orphan; nothing else may warn.
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "orphan") {
		t.Errorf("want exactly the orphan warning, got %v", out.Warnings)
	}

	checks := []string{
		"fillColor=#d5e8d4",
		"strokeColor=#107C41",
		"strokeWidth=3",
		`label="XLS"`,
		"&lt;b&gt;Billing&lt;/b&gt;",
	}
	for _, want := range checks {
		if !strings.Contains(out.XML, want) {
			t.Errorf("XML missing %q\n--- got ---\n%s", want, out.XML)
		}
	}
}

// TestE2E_ValidationErrors asserts validation surfaces the unknown kind
// and the dangling edge as content errors while the render still runs.
func TestE2E_ValidationErrors(t *testing.T) {
	ctx := ctxWithFakeCatalog(t)

	diagramJSON := `{
		"viewpoint": "drawio-test:/viewpoint/app-flux",
		"nodes": [
			{"id": "a", "label": "A", "kind": "Application", "attributes": {"domain": "reference"}},
			{"id": "b", "label": "B", "kind": "Mystery"}
		],
		"edges": [{"id": "e1", "source": "a", "target": "ghost", "kind": "Flow"}]
	}`

	out, err := executeDiagramEmit(ctx, Input{Diagram: mustLogical(t, diagramJSON), NoLayout: true})
	if err != nil {
		t.Fatalf("executeDiagramEmit: %v", err)
	}
	if out.XML == "" {
		t.Error("XML should still be produced for recoverable content errors")
	}
	combined := strings.Join(out.Errors, " | ")
	for _, want := range []string{"Mystery", "ghost"} {
		if !strings.Contains(combined, want) {
			t.Errorf("errors should mention %q: %v", want, out.Errors)
		}
	}
}

// TestE2E_UnknownStencilSet asserts an unknown stencil set is a content
// error the LLM can fix, and nothing is rendered.
func TestE2E_UnknownStencilSet(t *testing.T) {
	ctx := ctxWithFakeCatalog(t)
	out, err := executeDiagramEmit(ctx, Input{
		Diagram:  mustLogical(t, `{"viewpoint": "ktw-docs:/viewpoint/nonexistent", "nodes": [{"id": "a", "label": "A", "kind": "Application"}]}`),
		NoLayout: true,
	})
	if err != nil {
		t.Fatalf("want a content error, got Go error %v", err)
	}
	if out.XML != "" || len(out.Errors) == 0 {
		t.Fatalf("want errors and no XML, got xml=%d bytes errors=%v", len(out.XML), out.Errors)
	}
}

// mustLogical decodes a LogicalDiagram literal the way the tool
// framework decodes the LLM's call.
func mustLogical(t *testing.T, raw string) *drawioapi.LogicalDiagram {
	t.Helper()
	var ld drawioapi.LogicalDiagram
	if err := json.Unmarshal([]byte(raw), &ld); err != nil {
		t.Fatalf("decode diagram: %v", err)
	}
	return &ld
}
