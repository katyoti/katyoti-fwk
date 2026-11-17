// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// The solution's tags are the ones the spec gives (a scenario case declares
// them), else the universe's own.
func TestAddRelease_TheSpecTagsReplaceTheUniverses(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return s })
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	m.Tags = []string{"plain"}
	if err := catalog.SaveManifest(home, m); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version    string
		spec, want []string
	}{
		{"v1", []string{"archifactory"}, []string{"archifactory"}},
		{"v2", nil, []string{"plain"}},
	} {
		if _, err := New().AddSolution(home, tc.version, "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{Tags: tc.spec}); err != nil {
			t.Fatal(err)
		}
		cat, err := loadCatalogFile(catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range cat.Items {
			if it.Name == "demo" {
				if got := it.Versions[0].Universes[0].Tags; !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s: tags = %v, want %v", tc.version, got, tc.want)
				}
			}
		}
	}
}

// A universe published with tags is shown what its tags allow: its gui.hide is
// left out of the solution (first and levels kept), so a
// solution update replaces the installed universe's gui and its hide goes. A
// universe with no tags keeps its gui.hide.
func TestATaggedSolutionUniverseIsPublishedWithoutGuiHide(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return s })
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	m.GUI = &cenv.UniverseGUI{Hide: []string{"planning-deck", "runs"}, First: []string{"archifactory"}, Levels: map[string]string{"home:chat": "expert"}}
	if err := catalog.SaveManifest(home, m); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version  string
		tags     []string
		wantHide []string
	}{
		{"v1", []string{"archifactory"}, nil},
		{"v2", nil, []string{"planning-deck", "runs"}},
	} {
		if _, err := New().AddSolution(home, tc.version, "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{Tags: tc.tags}); err != nil {
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
			for _, v := range it.Versions {
				if v.Version != tc.version {
					continue
				}
				gui := v.Universes[0].GUI
				if gui == nil {
					t.Fatalf("%s: published with no gui - an update would keep the installed universe's own hide", tc.version)
				}
				if !reflect.DeepEqual(gui.Hide, tc.wantHide) {
					t.Errorf("%s (tags %v): gui.hide = %v, want %v", tc.version, tc.tags, gui.Hide, tc.wantHide)
				}
				if !reflect.DeepEqual(gui.First, []string{"archifactory"}) || gui.Levels["home:chat"] != "expert" {
					t.Errorf("%s: gui.first/levels not kept: %+v", tc.version, gui)
				}
			}
		}
	}
	// The universe's own manifest is never changed by a publish.
	if m, _ := catalog.LoadManifest(home); m == nil || len(m.GUI.Hide) != 2 {
		t.Errorf("the universe's own gui.hide was changed: %+v", m)
	}
}
