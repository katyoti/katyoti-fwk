// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// An other universe of a solution gets its own seed — published as an item of
// its own, made with that universe's members, pinned as its seed, never as the
// main universe's — and its display replaced by what the spec gives.
func TestAddSolution_AnOtherUniverseHasItsSeedAndDisplay(t *testing.T) {
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
	project := filepath.Join(t.TempDir(), "review-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gui := &cenv.UniverseGUI{Hide: []string{"runs"}}
	spec := SolutionSpec{Others: []OtherUniverse{{Name: "review", Home: other, Seed: SolutionSeed{Project: project},
		DisplayName: "Relecture", Icon: "R", GUI: gui, Tags: []string{"review"}}}}
	r, err := New().AddSolution(home, "v1", "demo", catalogPath, "", SolutionSeed{}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Universes, []string{"scn-demo", "review"}) || !reflect.DeepEqual(r.OtherSeeds, []string{"demo-review-seed@v1"}) {
		t.Errorf("result universes %v, seeds %v; want scn-demo, review and demo-review-seed@v1", r.Universes, r.OtherSeeds)
	}
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var sol, seed *mktapi.ItemVersion
	for i := range cat.Items {
		switch it := &cat.Items[i]; it.Name {
		case "demo":
			sol = &it.Versions[0]
		case "demo-review-seed":
			if it.Kind != mktapi.KindSeed {
				t.Errorf("demo-review-seed is a %s, want a seed", it.Kind)
			}
			seed = &it.Versions[0]
		}
	}
	if sol == nil || seed == nil {
		t.Fatalf("catalog items %+v: want the solution demo and the seed demo-review-seed", cat.Items)
	}
	if sol.Seed != nil {
		t.Errorf("the version's seed %+v: the other universe's seed is no seed of the main one", sol.Seed)
	}
	if err := sol.ValidateUniverses(); err != nil {
		t.Errorf("the published solution does not validate: %v", err)
	}
	u := sol.Universes[1]
	if u.Seed == nil || u.Seed.Name != "demo-review-seed" || u.Seed.Version != "v1" {
		t.Errorf("review's seed %+v, want demo-review-seed@v1", u.Seed)
	}
	if u.DisplayName != "Relecture" || u.Icon != "R" || !reflect.DeepEqual(u.GUI, &cenv.UniverseGUI{}) || !reflect.DeepEqual(u.Tags, []string{"review"}) {
		// A tagged universe is published without its gui.hide.
		t.Errorf("review's display %q %q %+v %v, want the spec's (its gui without hide: it has tags)", u.DisplayName, u.Icon, u.GUI, u.Tags)
	}
	if !reflect.DeepEqual(seed.Requires.Members, []mktapi.PinnedRef{{Name: "ktw-x", Version: "1.4.0"}}) {
		t.Errorf("the seed requires %+v, want review's members", seed.Requires.Members)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(catalogPath), filepath.FromSlash(index.SeedAssetPath("v1", "demo-review-seed")))); err != nil {
		t.Errorf("the seed's bytes are not published: %v", err)
	}
}
