// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestGenerateManifest(t *testing.T) {
	c := NewEndpoint("test-svc", "Test service", "1.0.0")

	// Public slot
	pub := NewSlot("files", DefaultInstance, "File operations", nil)
	pub.Add(RawMethod("read", MethodOpts{Desc: "Read a file"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}))
	c.AddSlot(pub)

	// Internal slot — should be filtered out
	internal := NewSlot("debug", DefaultInstance, "Debug internals", SlotTags{TagSlotInternal: "true"})
	internal.Add(RawMethod("dump", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}))
	c.AddSlot(internal)

	manifest := GenerateManifest(c)

	if manifest.Name != "test-svc" {
		t.Errorf("Name = %q, want %q", manifest.Name, "test-svc")
	}

	// Should have 1 public slot (files). ktsp is also internal, debug is internal.
	publicCount := 0
	for _, s := range manifest.Slots {
		if s.Tags[TagSlotInternal] != "true" {
			publicCount++
		}
	}
	if publicCount != 1 {
		t.Errorf("public slots = %d, want 1", publicCount)
	}

	// Verify JSON serialization works
	data, err := GenerateManifestJSON(c)
	if err != nil {
		t.Fatalf("GenerateManifestJSON: %v", err)
	}
	var parsed ContainerInfo
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Name != "test-svc" {
		t.Errorf("parsed.Name = %q, want %q", parsed.Name, "test-svc")
	}
}

// After the Phase-A++ collapse, every slot has a typeName that doubles
// as the SlotType — the categorical "typed vs untyped" distinction is
// gone. Manifest contracts surface one entry per distinct typeName
// (excluding the built-in ktsp slot, which is internal).
func TestGenerateManifestSlotTypes(t *testing.T) {
	c := NewEndpoint("skill-svc", "Skill service", "1.0.0")

	skill := NewSlot("docs", DefaultInstance, "Document operations", nil)
	skill.Add(RawMethod("render", MethodOpts{Desc: "Render a document"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}))
	c.AddSlot(skill)

	plain := NewSlot("util", DefaultInstance, "Utility operations", nil)
	plain.Add(RawMethod("ping", MethodOpts{}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}))
	c.AddSlot(plain)

	manifest := GenerateManifest(c)

	// Both slots advertise as contracts now — SlotType IS typeName.
	types := make(map[SlotType]bool)
	for _, st := range manifest.SlotTypes {
		types[st.Type] = true
	}
	if !types["docs"] {
		t.Errorf("manifest.SlotTypes missing 'docs' contract (%v)", types)
	}
	if !types["util"] {
		t.Errorf("manifest.SlotTypes missing 'util' contract (%v)", types)
	}

	// Each slot's SlotType field mirrors its Name.
	for _, s := range manifest.Slots {
		if string(s.SlotType) != s.Name {
			t.Errorf("slot %q: SlotType=%q must mirror Name=%q",
				s.Name, s.SlotType, s.Name)
		}
	}
}
