// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A Go nil slice marshals to JSON `null`, but these methods' schemas declare
// their list fields as required ARRAYS. Sending null breaks any client that
// trusts the schema — it broke the GUI's edit form with
// "Cannot read properties of null (reading 'map')" for a universe that simply
// had no sidecars yet.
//
// Empty means empty. It does not mean absent. These tests pin that on the
// EMPTY-collection path, which is the one nobody exercises by hand.

// marshalField returns the raw JSON of one top-level field.
func marshalField(t *testing.T, v any, field string) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw, ok := m[field]
	if !ok {
		// Absent is legitimate for an omitempty/optional field. What is NEVER
		// legitimate is present-and-null where the schema promises an array.
		return ""
	}
	return string(raw)
}

func TestGetUniverse_EmptySidecarsMarshalsAsArrayNotNull(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	home := filepath.Join(tempDir, "universes", "bare")

	registry, err := catalog.LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	// A universe with NO sidecars — the case that produced null.
	if err := catalog.SaveManifest(home, &catalog.Manifest{
		Version: 1, Name: "bare", DisplayName: "Bare",
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(catalog.RegistryEntry{Name: "bare", HomePath: home}); err != nil {
		t.Fatal(err)
	}

	s := &Service{registry: registry}
	info, err := s.GetUniverse(context.Background(), api.GetUniverseInput{Name: "bare"})
	if err != nil {
		t.Fatalf("GetUniverse: %v", err)
	}
	if got := marshalField(t, info, "sidecars"); got == "null" {
		t.Errorf("sidecars marshalled as null; the schema declares a required array")
	} else if !strings.HasPrefix(got, "[") {
		t.Errorf("sidecars = %s, want a JSON array", got)
	}
}

func TestListProjects_EmptyMarshalsAsArrayNotNull(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())

	s := &Service{}
	// A universe with no projects.yaml at all — the emptiest possible case.
	out, err := s.ListProjects(context.Background(), api.ListProjectsInput{Universe: "nothing-attached"})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if got := marshalField(t, out, "projects"); got == "null" {
		t.Errorf("projects marshalled as null; the schema declares a required array")
	}
}

// The admin universe is the one every client reaches first, so its info must
// satisfy the same contract on a fresh machine.
func TestAdminUniverseInfo_NoNullCollections(t *testing.T) {
	tempDir := t.TempDir()
	registry, err := catalog.LoadRegistry(filepath.Join(tempDir, "universes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(tempDir, "universes", admin.UniverseName)
	if err := admin.Ensure(registry, home); err != nil {
		t.Fatal(err)
	}

	s := &Service{registry: registry}
	info, err := s.GetUniverse(context.Background(), api.GetUniverseInput{Name: admin.UniverseName})
	if err != nil {
		t.Fatalf("GetUniverse(admin): %v", err)
	}
	// sidecars is REQUIRED by the schema, so it must be present and an array.
	if got := marshalField(t, info, "sidecars"); !strings.HasPrefix(got, "[") {
		t.Errorf("sidecars = %q, want a JSON array", got)
	}
	// layeredHomes is optional (omitempty): absent is fine, null is not.
	if got := marshalField(t, info, "layeredHomes"); got == "null" {
		t.Errorf("layeredHomes marshalled as null; omit it or send an array")
	}
}

// The service owns DisplayName's default. It used to REQUIRE the field, which
// pushed the default into every caller — and the callers disagreed: `ktw-univers
// create foo` produced display "Foo" while `katyoti universe create foo`
// produced "foo". The same operation, two results, because the service that owns
// the concept declined to answer.
//
// A default that lives in callers is not a default; it is a convention nobody
// enforces.
func TestCreateUniverse_ServiceOwnsTheDisplayNameDefault(t *testing.T) {
	tempDir := t.TempDir()
	registry, err := catalog.LoadRegistry(filepath.Join(tempDir, "universes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{registry: registry}

	info, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:     "alpha",
		HomePath: filepath.Join(tempDir, "universes", "alpha"),
	})
	if err != nil {
		t.Fatalf("CreateUniverse with no DisplayName: %v", err)
	}
	if info.Summary.DisplayName != "Alpha" {
		t.Errorf("DisplayName = %q, want %q — the service must default it so every caller agrees",
			info.Summary.DisplayName, "Alpha")
	}

	// An explicit display name still wins: defaulting is a fallback, not a rule.
	info2, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "beta",
		DisplayName: "Custom Name",
		HomePath:    filepath.Join(tempDir, "universes", "beta"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if info2.Summary.DisplayName != "Custom Name" {
		t.Errorf("DisplayName = %q, want the explicit value", info2.Summary.DisplayName)
	}
}
