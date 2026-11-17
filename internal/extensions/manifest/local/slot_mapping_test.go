// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

func TestMapKTSPInfo_EmptyInput(t *testing.T) {
	st, sl := mapKTSPInfo(ktsp.ContainerInfo{})
	if len(st) != 0 || len(sl) != 0 {
		t.Errorf("empty ContainerInfo: got %d slot types + %d slots, want 0/0", len(st), len(sl))
	}
}

func TestMapKTSPInfo_PopulatesSlotsAndTypes(t *testing.T) {
	in := ktsp.ContainerInfo{
		Name:    "ktw-docs",
		Version: "1.0.0",
		SlotTypes: []ktsp.SlotTypeContract{
			{
				Type:    "DocService",
				Version: "1.0.0",
				Methods: []ktsp.MethodInfo{
					{Name: "Render", Description: "render a doc"},
					{Name: "Filter", Description: "apply filter"},
				},
			},
		},
		Slots: []ktsp.SlotInfo{
			{Name: "docs", SlotType: "DocService", Description: "main"},
			{Name: "previews", SlotType: "DocService", Description: "thumbnails"},
		},
	}
	gotTypes, gotSlots := mapKTSPInfo(in)

	if len(gotTypes) != 1 || gotTypes[0].Type != "DocService" {
		t.Fatalf("SlotTypes = %+v, want one DocService", gotTypes)
	}
	if gotTypes[0].Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", gotTypes[0].Version)
	}
	if len(gotTypes[0].Methods) != 2 {
		t.Errorf("Methods len = %d, want 2", len(gotTypes[0].Methods))
	}

	if len(gotSlots) != 2 {
		t.Fatalf("Slots = %d, want 2", len(gotSlots))
	}
	if gotSlots[0].Name != "docs" || gotSlots[0].Type != "DocService" {
		t.Errorf("Slots[0] = %+v", gotSlots[0])
	}
}

func TestMapKTSPInfo_MarshalsSchemas(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	in := ktsp.ContainerInfo{
		SlotTypes: []ktsp.SlotTypeContract{
			{
				Type: "Foo",
				Methods: []ktsp.MethodInfo{
					{Name: "Bar", InputSchema: schema},
				},
			},
		},
	}
	got, _ := mapKTSPInfo(in)
	if len(got) != 1 || len(got[0].Methods) != 1 {
		t.Fatalf("unexpected mapping shape: %+v", got)
	}
	raw := got[0].Methods[0].InputSchema
	if len(raw) == 0 {
		t.Fatal("InputSchema lost in mapping")
	}
	// Round-trip parse to verify the JSON is valid + structurally identical.
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("InputSchema decode: %v", err)
	}
	if decoded["type"] != "object" {
		t.Errorf("decoded[type] = %v, want object", decoded["type"])
	}
}

func TestMapKTSPInfo_NilSchemaOmitted(t *testing.T) {
	in := ktsp.ContainerInfo{
		SlotTypes: []ktsp.SlotTypeContract{
			{Type: "Foo", Methods: []ktsp.MethodInfo{{Name: "Bar"}}},
		},
	}
	got, _ := mapKTSPInfo(in)
	if got[0].Methods[0].InputSchema != nil {
		t.Errorf("nil schema should map to nil RawMessage, got %v", got[0].Methods[0].InputSchema)
	}
}

func TestTagsToSlice_Stable(t *testing.T) {
	in := ktsp.MethodTags{"mcp": "true", "cli": "true", "internal": "false"}
	out := tagsToSlice(in)
	// Sorted: [cli internal=false mcp]
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0] != "cli" {
		t.Errorf("out[0] = %q, want cli", out[0])
	}
	if out[1] != "internal=false" {
		t.Errorf("out[1] = %q, want internal=false", out[1])
	}
	if out[2] != "mcp" {
		t.Errorf("out[2] = %q, want mcp", out[2])
	}
}
