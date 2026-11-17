// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestObjectPlaceholderFieldsReadTheBodyFirst — a method-panel descriptor
// written against a node (`${node.space}`) resolves on the object: its
// capability fields, then the type's own fields on top.
func TestObjectPlaceholderFieldsReadTheBodyFirst(t *testing.T) {
	o := cwmapi.Object{ID: "m.dossier", Type: "item", Name: "Dossier",
		Capabilities: map[string]map[string]any{"publish": {"space": "from-capability"}}}
	body, _ := json.Marshal(map[string]any{"space": "AUP"})
	o.Name = "D1"
	o.SetFieldsJSON(body)

	got := objectPlaceholderFields(o)
	if got["space"] != "AUP" {
		t.Errorf("space = %v, want the body's value (the body wins a clash)", got["space"])
	}
	if got["name"] != "D1" || got["id"] != "m.dossier" { // one name: the object core's
		t.Errorf("capability fields and identity must be present: %v", got)
	}
}

// TestResolveProjectRelativeAnchorsOnTheHoldingProject — a module folder
// resolves under the project that holds it, not the sidecar's working
// directory; absolute and unfound paths are left.
func TestResolveProjectRelativeAnchorsOnTheHoldingProject(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(b, "test_results", "d1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveProjectRelative("test_results/d1", []string{a, b}); got != filepath.Join(b, "test_results", "d1") {
		t.Errorf("got %q, want it under the project that holds it", got)
	}
	abs := filepath.Join(a, "x")
	if got := resolveProjectRelative(abs, []string{b}); got != abs {
		t.Errorf("an absolute path must pass through: %q", got)
	}
	if got := resolveProjectRelative("nowhere/here", []string{a, b}); got != "nowhere/here" {
		t.Errorf("a path found under no root must pass through: %q", got)
	}
}
