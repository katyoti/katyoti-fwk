// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// The solution's first guide is the one the spec gives (a scenario case
// declares it), else the universe's own.
func TestAddRelease_TheSpecFirstGuideReplacesTheUniverses(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return s })
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	m.FirstGuide = "x:/guide-recipe/own"
	if err := catalog.SaveManifest(home, m); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ version, spec, want string }{
		{"v1", "ktw-acme:/guide-recipe/welcome", "ktw-acme:/guide-recipe/welcome"},
		{"v2", "", "x:/guide-recipe/own"},
	} {
		if _, err := New().AddSolution(home, tc.version, "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{FirstGuide: tc.spec}); err != nil {
			t.Fatal(err)
		}
		cat, err := loadCatalogFile(catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range cat.Items {
			if it.Name == "demo" {
				if got := it.Versions[0].Universes[0].FirstGuide; got != tc.want {
					t.Errorf("%s: first guide = %q, want %q", tc.version, got, tc.want)
				}
			}
		}
	}
}
