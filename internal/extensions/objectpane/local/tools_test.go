// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"testing"

	objectpanelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
)

// THE AGENT SEES THESE TOOLS. Tools() registers exactly the expected names,
// each with a description and an object-typed JSON input schema, read-only
// only when the tool writes nothing.
func TestToolsRegisterTheirNamesDescriptionsAndSchemas(t *testing.T) {
	want := []struct {
		name     string
		readOnly bool
	}{
		{"object-transition-options", true},
	}
	tools := objectpanelocal.Tools()
	if len(tools) != len(want) {
		t.Fatalf("Tools() = %d tools, want %d", len(tools), len(want))
	}
	for i, w := range want {
		def := tools[i].ToolDef()
		if def.Name != w.name {
			t.Errorf("tool %d = %q, want %q", i, def.Name, w.name)
		}
		if def.Description == "" {
			t.Errorf("%s: empty description", def.Name)
		}
		var schema struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
			t.Fatalf("%s: input schema is not JSON: %v", def.Name, err)
		}
		if schema.Type != "object" || len(schema.Properties) == 0 {
			t.Errorf("%s: input schema type %q with %d properties, want an object with properties", def.Name, schema.Type, len(schema.Properties))
		}
		if got := tools[i].IsReadOnly(); got != w.readOnly {
			t.Errorf("%s: read-only = %v, want %v", def.Name, got, w.readOnly)
		}
	}
}
