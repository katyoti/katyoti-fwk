// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestObjectFileRoundTrip pins the committed shape: one Object <-> exactly one
// YAML file, with identity read from CONTENT (not the filename), the object core
// as its own fields, and capability field-groups preserved verbatim — CWM stores
// them and never interprets them.
func TestObjectFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Deliberately a slug filename that does NOT match the id, to prove identity
	// comes from the content.
	path := filepath.Join(dir, ObjectsDirName, "g1-module-self-contained.yaml")

	want := api.Object{
		ID:     "cwm.g1-module-self-contained",
		Type:   "feature",
		Name:   "A module is self-contained",
		Parent: "cwm",
		Capabilities: map[string]map[string]any{
			"want": {
				"reachability": "closable",
			},
			"checkable": {
				"kind":  "command",
				"probe": "test -d modules/cwm/objects",
			},
		},
		Edges: []api.ObjectEdge{
			{Kind: "realizes", To: "internal/extensions/cwm"},
		},
	}

	if err := WriteObjectFile(path, want); err != nil {
		t.Fatalf("WriteObjectFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("object file not written: %v", err)
	}

	got, err := ReadObjectFile(path)
	if err != nil {
		t.Fatalf("ReadObjectFile: %v", err)
	}

	if got.ID != want.ID || got.Type != want.Type {
		t.Errorf("identity = %q/%q, want %q/%q", got.ID, got.Type, want.ID, want.Type)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("capabilities = %v, want 2 field-groups", got.Capabilities)
	}
	if got.Name != "A module is self-contained" || got.Parent != "cwm" {
		t.Errorf("core = name %q parent %q", got.Name, got.Parent)
	}
	if got.Capabilities["want"]["reachability"] != "closable" {
		t.Errorf("want.reachability = %v, want closable", got.Capabilities["want"]["reachability"])
	}
	if got.Capabilities["checkable"]["probe"] != want.Capabilities["checkable"]["probe"] {
		t.Errorf("checkable.probe did not round-trip: %v", got.Capabilities["checkable"]["probe"])
	}

	// Authored edges omit `from`; it is implied by the declaring object and
	// materialized on expansion.
	if len(got.Edges) != 1 || got.Edges[0].Kind != "realizes" {
		t.Fatalf("edges = %+v, want one realizes edge", got.Edges)
	}
	ge := got.GraphEdges()
	if len(ge) != 1 || ge[0].From != want.ID || ge[0].To != "internal/extensions/cwm" || string(ge[0].Kind) != "realizes" {
		t.Errorf("GraphEdges() = %+v, want from=%s to=internal/extensions/cwm type=realizes", ge, want.ID)
	}
}

// TestReadObjectFile_RejectsAnonymous pins that a file with no id is an error,
// not an anonymous object — identity is not optional.
func TestReadObjectFile_RejectsAnonymous(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-id.yaml")
	if err := os.WriteFile(path, []byte("type: feature\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ReadObjectFile(path); err == nil {
		t.Fatal("expected an error for an object file with no id")
	}
}
