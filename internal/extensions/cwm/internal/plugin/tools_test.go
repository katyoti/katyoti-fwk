// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"strings"
	"testing"
)

// TestCWMTools_Surface asserts the agent-tool surface: 28 typed tools — the
// object tools and the edge tools —
// all cwm-*-prefixed, with unique names and non-empty JSON-Schema for inputs.
func TestCWMTools_Surface(t *testing.T) {
	tools := CWMTools()
	if got, want := len(tools), 28; got != want {
		t.Fatalf("CWMTools count = %d, want %d", got, want)
	}

	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		def := tool.ToolDef()

		if def.Name == "" {
			t.Errorf("tool has empty name: %#v", def)
			continue
		}
		if !strings.HasPrefix(def.Name, "cwm-") {
			t.Errorf("tool %q is not cwm-*-prefixed", def.Name)
		}
		if _, dup := seen[def.Name]; dup {
			t.Errorf("duplicate tool name %q", def.Name)
		}
		seen[def.Name] = struct{}{}

		if def.Description == "" {
			t.Errorf("tool %q has empty description", def.Name)
		}
		if len(def.InputSchema) == 0 {
			t.Errorf("tool %q has empty InputSchema", def.Name)
		}
	}
}

// TestCWMTools_ReadOnlyClassification asserts the read/write split
// matches the DoD: the named read tools advertise IsReadOnly()==true
// and the named write tools advertise IsReadOnly()==false.
func TestCWMTools_ReadOnlyClassification(t *testing.T) {
	readOnlyNames := map[string]bool{
		"cwm-edge-list":               true,
		"cwm-object-get":              true,
		"cwm-object-list":             true,
		"cwm-object-tree":             true,
		"cwm-object-search":           true,
		"cwm-object-resolve":          true,
		"cwm-object-type-get":         true,
		"cwm-object-allowed-children": true,
		"cwm-object-capabilities":     true,
		"cwm-object-named-file-list":  true,
		"cwm-object-named-file-read":  true,
		"cwm-object-file-list":        true,
		"cwm-object-file-read":        true,
		"cwm-object-error-list":       true,
	}
	writeNames := map[string]bool{
		"cwm-edge-create":              true,
		"cwm-edge-delete":              true,
		"cwm-object-create":            true,
		"cwm-object-update":            true,
		"cwm-object-add-capability":    true,
		"cwm-object-remove-capability": true,
		"cwm-object-move":              true,
		"cwm-object-split":             true,
		"cwm-object-named-file-write":  true,
		"cwm-object-file-write":        true,
		"cwm-object-file-attach":       true,
		"cwm-object-file-delete":       true,
		"cwm-object-delete":            true,
		"cwm-object-archive":           true,
	}

	for _, tool := range CWMTools() {
		name := tool.ToolDef().Name
		switch {
		case readOnlyNames[name]:
			if !tool.IsReadOnly() {
				t.Errorf("%q should be read-only", name)
			}
		case writeNames[name]:
			if tool.IsReadOnly() {
				t.Errorf("%q should NOT be read-only", name)
			}
		default:
			t.Errorf("tool %q not classified by the test (drift between DoD and code)", name)
		}
	}

	// The deletes are destructive.
	for _, tool := range CWMTools() {
		switch name := tool.ToolDef().Name; name {
		case "cwm-object-delete", "cwm-object-archive", "cwm-object-file-delete", "cwm-edge-delete":
			if !tool.IsDestructive() {
				t.Errorf("%s should be destructive", name)
			}
		}
	}
}
