// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSplitObjectID(t *testing.T) {
	for _, tc := range []struct {
		in, mod, local string
		wantErr        bool
	}{
		{"compose.CMP-010", "compose", "CMP-010", false},
		{"cwm.p1d-call-sites-migrated", "cwm", "p1d-call-sites-migrated", false},
		{"a.b.c", "a", "b.c", false}, // first dot wins; local may contain dots
		{"unqualified", "", "", true},
		{"", "", "", true},
		{".leading", "", "", true},
		{"trailing.", "", "", true},
	} {
		mod, local, err := SplitObjectID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("SplitObjectID(%q) should fail loudly, got %q/%q", tc.in, mod, local)
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("SplitObjectID(%q) error should wrap ErrInvalidInput: %v", tc.in, err)
			}
			continue
		}
		if err != nil || mod != tc.mod || local != tc.local {
			t.Errorf("SplitObjectID(%q) = %q/%q err=%v, want %q/%q", tc.in, mod, local, err, tc.mod, tc.local)
		}
	}
}

// TestQualifyObjectIDIsIdempotent pins that qualifying twice is safe — the
// migration applies it at several layers and must not produce cwm.cwm.x.
func TestQualifyObjectIDIsIdempotent(t *testing.T) {
	once := QualifyObjectID("cwm", "p1")
	if once != "cwm.p1" {
		t.Fatalf("qualify = %q", once)
	}
	if twice := QualifyObjectID("cwm", once); twice != "cwm.p1" {
		t.Errorf("qualify twice = %q, want cwm.p1", twice)
	}
	// The module object itself is NEVER self-qualified — `cwm` lives in
	// modules/cwm/objects/cwm.yaml, not cwm.cwm.yaml. Caught by the p1d runtime
	// smoke, which saw a module created as "smoke-module.smoke-module".
	if self := QualifyObjectID("cwm", "cwm"); self != "cwm" {
		t.Errorf("module self-id = %q, want cwm", self)
	}
	if got := NewContainerObject("m", "m", "module", "", "M", "").ID; got != "m" {
		t.Errorf("NewContainerObject module id = %q, want m", got)
	}
}

// TestNewObjectsRoundTripThroughCapabilities pins that the constructors produce
// objects the typed accessors can read — the contract consumers rely on.
func TestNewObjectsRoundTripThroughCapabilities(t *testing.T) {
	// A MODULE object is never self-qualified: its id IS the module name, and
	// it lives at modules/compose/objects/compose.yaml.
	c := NewContainerObject("compose", "compose", "module", "", "Compose", "files-only composer")
	if c.ID != "compose" {
		t.Errorf("module object id = %q, want the bare module name", c.ID)
	}
	got, ok := c, true
	if !ok || got.Name != "Compose" || got.Description != "files-only composer" {
		t.Errorf("container = %+v ok=%v", got, ok)
	}

	r := NewRecordObject("compose", "CMP-010", "use-case", "compose.compose", "routes a skill request", json.RawMessage(`{"severity":"core"}`))
	if r.ID != "compose.CMP-010" {
		t.Errorf("record id = %q", r.ID)
	}
	if r.ParentID() != "compose.compose" {
		t.Errorf("record parent = %q", r.ParentID())
	}
	content, ok := r, true
	if !ok || content.Name != "routes a skill request" {
		t.Errorf("content = %+v ok=%v", content, ok)
	}
	var body map[string]any
	if err := json.Unmarshal(content.FieldsJSON(), &body); err != nil || body["severity"] != "core" {
		t.Errorf("body lost: %v %v", body, err)
	}
}

// TestModuleOfObjectID pins that a module object (bare id) resolves to itself,
// while a child resolves to its owning module.
func TestModuleOfObjectID(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		wantErr  bool
	}{
		{"cwm", "cwm", false},    // the module object itself
		{"cwm.p1", "cwm", false}, // a child
		{"compose.CMP-010", "compose", false},
		{"", "", true},
		{".bad", "", true},
	} {
		got, err := ModuleOfObjectID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ModuleOfObjectID(%q) should fail, got %q", tc.in, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ModuleOfObjectID(%q) = %q err=%v, want %q", tc.in, got, err, tc.want)
		}
	}
}
