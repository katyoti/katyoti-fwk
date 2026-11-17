// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A solution is made of the universe snapshotted — its main one — and the
// other universes the spec names, each captured from its home: a member two
// universes share is pinned once.
func TestAddSolution_ComposesTheOtherUniverses(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return s })
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(home), "review")
	m.Name, m.DisplayName = "review", "Review"
	if err := catalog.SaveManifest(other, m); err != nil {
		t.Fatal(err)
	}
	spec := SolutionSpec{Others: []OtherUniverse{{Name: "review", Home: other}}}
	if _, err := New().AddSolution(home, "v1", "demo", catalogPath, "", SolutionSeed{}, spec); err != nil {
		t.Fatal(err)
	}
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cat.Items {
		if it.Name != "demo" {
			continue
		}
		v := it.Versions[0]
		if len(v.Includes) != 1 {
			t.Errorf("includes %+v, want ktw-x pinned once", v.Includes)
		}
		if len(v.Universes) != 2 || !v.Universes[0].Main || v.Universes[0].Name != "scn-demo" ||
			v.Universes[1].Main || v.Universes[1].Name != "review" || v.Universes[1].DisplayName != "Review" {
			t.Errorf("universes %+v; want scn-demo (main) and review", v.Universes)
		}
		return
	}
	t.Fatal("no solution demo in the catalog")
}
