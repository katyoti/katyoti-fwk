// SPDX-License-Identifier: Apache-2.0

package toolproxy

import (
	"context"
	"fmt"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// fakeCatalog implements just enough of mbapi.Catalog for resolveTools: only
// Get is exercised. The embedded nil interface satisfies the rest of the method
// set at compile time (never called here).
type fakeCatalog struct {
	mbapi.Catalog
	m map[string]*mbapi.Manifest
}

func (f fakeCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	if man, ok := f.m[name]; ok {
		return man, nil
	}
	return nil, fmt.Errorf("not in catalog: %s", name)
}

func (f fakeCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	out := make([]*mbapi.Manifest, 0, len(f.m))
	for _, m := range f.m {
		out = append(out, m)
	}
	return out, nil
}

// TestResolveTools_Wildcard locks the regression: a glob method
// (`weather-service/*`) must enumerate every advertised method of the slot.
// Before the ParseToolPattern fix, ParseToolURI rejected `*` up front and the
// whole glob branch was unreachable, so wildcards resolved to nothing.
func TestResolveTools_Wildcard(t *testing.T) {
	cat := fakeCatalog{m: map[string]*mbapi.Manifest{"ktw-weather": sampleManifest()}}

	got := resolveTools(context.Background(), cat, []string{"ktw-weather:weather-service/*"})

	names := map[string]bool{}
	for _, m := range got {
		names[toolName(m.uri)] = true
	}
	if !names["get-forecast"] || !names["get-current"] {
		t.Errorf("wildcard must enumerate advertised methods; got %v", names)
	}
	if names["internal-reindex"] {
		t.Error("internal-tagged method must be excluded from wildcard expansion")
	}
	if len(got) != 2 {
		t.Errorf("want exactly 2 minted tools, got %d (%v)", len(got), names)
	}
	// The minted forecast tool must carry the typed schema from the manifest.
	for _, m := range got {
		if toolName(m.uri) == "get-forecast" && len(m.schema) == 0 {
			t.Error("wildcard-minted get-forecast lost its typed schema")
		}
	}
}

// TestResolveTools_ContributionsOnly pins why the catalog attaches SlotTypes: a
// manifest built from the contribution INDEX alone (skills/tasks present, no
// SlotTypes) resolves a tool glob to NOTHING — sidecar tools need the KTSP
// method registry. A catalog built from the CatalogService index listing
// (contributions only) gives a chat its sidecar's skills but none of its tools;
// ktw-agents therefore attaches SlotTypes via GetManifest at fetch time.
func TestResolveTools_ContributionsOnly(t *testing.T) {
	manifest := &mbapi.Manifest{Provider: "ktw-example"} // contributions-shaped, NO SlotTypes
	cat := fakeCatalog{m: map[string]*mbapi.Manifest{"ktw-example": manifest}}
	got := resolveTools(context.Background(), cat, []string{"ktw-example:example/*"})
	if len(got) != 0 {
		t.Fatalf("a SlotTypes-less manifest must resolve a tool glob to nothing; got %d", len(got))
	}
}

// TestResolveTools_ExactStillWorks guards that the shared split path didn't
// regress exact (non-glob) URIs.
func TestResolveTools_ExactStillWorks(t *testing.T) {
	cat := fakeCatalog{m: map[string]*mbapi.Manifest{"ktw-weather": sampleManifest()}}
	got := resolveTools(context.Background(), cat, []string{"ktw-weather:weather-service/get-current"})
	if len(got) != 1 || toolName(got[0].uri) != "get-current" {
		t.Fatalf("exact URI must mint one tool; got %d", len(got))
	}
}
