// SPDX-License-Identifier: Apache-2.0

package appcore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

func TestBuildEndpointManifest(t *testing.T) {
	c := ktsp.NewEndpoint("test-sidecar", "Test sidecar", "1.0.0")
	slot := ktsp.NewSlot("myslot", ktsp.DefaultInstance, "A test slot", nil)
	slot.Add(ktsp.RawMethod("do_thing", ktsp.MethodOpts{Desc: "Does a thing"}, nil))
	c.AddSlot(slot)

	endpoint := appcore.BuildEndpointManifest("test-sidecar", "1.0.0", "Test", nil, c)

	if endpoint.Name != "test-sidecar" {
		t.Errorf("Name = %q, want %q", endpoint.Name, "test-sidecar")
	}
	if len(endpoint.Transports) != 1 || endpoint.Transports[0] != "stdio" {
		t.Errorf("Transports = %v, want [stdio]", endpoint.Transports)
	}

	// After the Phase-A++ collapse, SlotType IS typeName.
	foundSlot := false
	for _, s := range endpoint.KTSP.Slots {
		if s.Name == "myslot" {
			foundSlot = true
			if string(s.SlotType) != s.Name {
				t.Errorf("myslot SlotType=%q must mirror Name=%q",
					s.SlotType, s.Name)
			}
		}
	}
	if !foundSlot {
		t.Error("KTSP section missing myslot")
	}

	// The endpoint advertises one contract per distinct typeName
	// (excluding the built-in ktsp slot).
	found := false
	for _, st := range endpoint.KTSP.SlotTypes {
		if st.Type == "myslot" {
			found = true
		}
	}
	if !found {
		t.Errorf("manifest.SlotTypes missing 'myslot' contract: %v", endpoint.KTSP.SlotTypes)
	}

	data, err := json.Marshal(endpoint)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded appcore.EndpointManifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != endpoint.Name {
		t.Errorf("roundtrip Name = %q, want %q", decoded.Name, endpoint.Name)
	}
}

// TestEndpointRegistryYAMLRoundTrip verifies that InputSchema/OutputSchema
// stored as `any` (decoded trees) serialize cleanly to YAML and decode back
// to the same structure. Regression guard for the json.RawMessage-as-[]byte
// bug where yaml.v3 rendered schemas as integer byte sequences.
func TestEndpointRegistryYAMLRoundTrip(t *testing.T) {
	// Build a manifest with a slot_type that carries a non-trivial nested schema.
	inputSchema := map[string]any{
		"type":     "object",
		"required": []any{"path"},
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Root folder to analyze",
			},
			"max_depth": map[string]any{
				"type":        "integer",
				"description": "Max directory depth. 0 = unlimited",
			},
		},
		"additionalProperties": false,
	}
	outputSchema := map[string]any{
		"type":     "object",
		"required": []any{"total_files"},
		"properties": map[string]any{
			"total_files": map[string]any{"type": "integer"},
			"root_path":   map[string]any{"type": "string"},
		},
	}

	manifest := appcore.EndpointManifest{
		Name:        "ktw-fixture",
		Version:     "dev",
		Description: "Round-trip fixture",
		Transports:  []string{"stdio"},
		KTSP: ktsp.ContainerInfo{
			Name:        "ktw-fixture",
			Description: "fixture",
			Version:     "dev",
			SlotTypes: []ktsp.SlotTypeContract{
				{
					Type:    "CodeService",
					Version: "1.0",
					Methods: []ktsp.MethodInfo{
						{
							Name:         "analyze-folder",
							Description:  "AnalyzeFolder operation",
							InputSchema:  inputSchema,
							OutputSchema: outputSchema,
							Tags:         ktsp.MethodTags{"cli.name": "analyze"},
						},
					},
				},
			},
			Slots: []ktsp.SlotInfo{
				{Name: "code", SlotType: "CodeService"},
			},
		},
	}

	reg := &appcore.UniverseSidecars{
		Sidecars: []appcore.SidecarSpec{
			{
				Name:      "ktw-fixture",
				Transport: "stdio",
				Binary:    "/tmp/fake",
				Manifest:  &manifest,
			},
		},
	}

	// Round-trip through yaml on disk (same path as production).
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "extensions.yaml")
	if err := reg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	// Guard against the json.RawMessage-as-byte-array regression: the bug
	// produced `inputschema:` lines containing long sequences of small
	// integers like `- 123`. If the file contains that shape, serialization
	// reverted to the broken path.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, "- 123\n") || strings.Contains(content, "- 123\r") {
		t.Errorf("YAML output contains byte-array sequence (regression of json.RawMessage-as-[]byte):\n%s", content)
	}
	if !strings.Contains(content, "Root folder to analyze") {
		t.Errorf("YAML output missing inputSchema description, got:\n%s", content)
	}
	t.Logf("YAML output (%d bytes):\n%s", len(content), content)

	// Load back and verify schemas are intact.
	loaded, err := appcore.LoadUniverseSidecarsFrom(path)
	if err != nil {
		t.Fatalf("LoadUniverseSidecarsFrom: %v", err)
	}
	entry := loaded.FindByName("ktw-fixture")
	if entry == nil {
		t.Fatal("fixture entry missing after reload")
	}
	if entry.Manifest == nil {
		t.Fatal("fixture manifest missing after reload")
	}
	if len(entry.Manifest.KTSP.SlotTypes) != 1 {
		t.Fatalf("SlotTypes count = %d, want 1", len(entry.Manifest.KTSP.SlotTypes))
	}
	loadedMethod := entry.Manifest.KTSP.SlotTypes[0].Methods[0]
	if loadedMethod.Name != "analyze-folder" {
		t.Errorf("method name = %q, want analyze-folder", loadedMethod.Name)
	}
	if loadedMethod.InputSchema == nil {
		t.Error("InputSchema lost during round-trip")
	}
	if loadedMethod.OutputSchema == nil {
		t.Error("OutputSchema lost during round-trip")
	}
	if loadedMethod.Tags["cli.name"] != "analyze" {
		t.Errorf("method tag cli.name = %q, want analyze", loadedMethod.Tags["cli.name"])
	}
}
