// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"context"
	"strings"
	"testing"
)

// TestSchema_GeneratesNonEmpty asserts the typed tool's input schema is
// non-empty and carries the LogicalDiagram shape — guards against tag
// drift that would silently produce an empty schema.
func TestSchema_GeneratesNonEmpty(t *testing.T) {
	tool := NewDiagramEmitTool()
	def := tool.ToolDef()
	if def.Name != "diagram-emit" {
		t.Errorf("name: got %q", def.Name)
	}
	if len(def.InputSchema) == 0 {
		t.Fatal("empty input schema")
	}
	s := string(def.InputSchema)
	for _, want := range []string{"diagram", "viewpoint", "nodes", "kind", "attributes", "no_layout"} {
		if !strings.Contains(s, want) {
			t.Errorf("schema missing field %q: %s", want, s)
		}
	}
	if strings.Contains(s, "stencil") {
		t.Errorf("schema still carries a retired stencil-set field: %s", s)
	}
}

// TestExecute_MissingDiagram asserts the soft-error path: a missing
// diagram returns Output.Errors (not a Go error), so the LLM can recover
// with a corrected call.
func TestExecute_MissingDiagram(t *testing.T) {
	out, err := executeDiagramEmit(context.Background(), Input{})
	if err != nil {
		t.Errorf("want nil error, got %v", err)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "diagram") {
		t.Errorf("want diagram error, got %v", out.Errors)
	}
}
