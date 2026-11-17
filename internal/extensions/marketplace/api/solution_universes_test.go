// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// An older catalog carried one universe in `universe:`: it decodes into the
// list, as its one main universe, from YAML and from JSON, and is never
// written back in the old field.
func TestAnOldSingleUniverseDecodesIntoTheList(t *testing.T) {
	check := func(lane string, v ItemVersion) {
		t.Helper()
		if v.Universe != nil {
			t.Errorf("%s: the old field is kept: %+v", lane, v.Universe)
		}
		if len(v.Universes) != 1 || !v.Universes[0].Main || v.Universes[0].DisplayName != "Demo" || len(v.Universes[0].Sidecars) != 1 {
			t.Fatalf("%s: universes %+v, want the one main universe Demo", lane, v.Universes)
		}
		us := v.SolutionUniverses()
		if len(us) != 1 || len(us[0].ConfigSets) != 1 || us[0].Seed == nil || us[0].Seed.Name != "demo-seed" {
			t.Errorf("%s: %+v; the version's config-sets and seed are the main universe's", lane, us)
		}
		if got := v.UniverseMembers(us[0]); len(got) != 2 {
			t.Errorf("%s: members %+v, want every member", lane, got)
		}
	}
	var y ItemVersion
	if err := yaml.Unmarshal([]byte(`version: "1"
includes: [{name: a, version: "1"}, {name: b, version: "1"}]
universe:
  displayName: Demo
  sidecars: [{name: a}]
configSets: [{name: cs, version: "1"}]
seed: {name: demo-seed, version: "1"}
`), &y); err != nil {
		t.Fatal(err)
	}
	check("yaml", y)
	var j ItemVersion
	if err := json.Unmarshal([]byte(`{"version":"1","includes":[{"name":"a","version":"1"},{"name":"b","version":"1"}],
"universe":{"displayName":"Demo","sidecars":[{"name":"a"}]},"configSets":[{"name":"cs","version":"1"}],"seed":{"name":"demo-seed","version":"1"}}`), &j); err != nil {
		t.Fatal(err)
	}
	check("json", j)
	out, err := yaml.Marshal(y)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "\nuniverse:") || !strings.Contains(string(out), "universes:") {
		t.Errorf("written back:\n%s\nwant universes:, never universe:", out)
	}
}

// A solution has exactly one main universe; of several, each is named once,
// takes pinned members only, and every member goes somewhere.
func TestASolutionHasExactlyOneMainUniverse(t *testing.T) {
	inc := []PinnedRef{{Name: "a", Version: "1"}, {Name: "b", Version: "1"}}
	sc := func(n string) []SolutionSidecar { return []SolutionSidecar{{Name: n}} }
	for _, tc := range []struct {
		name string
		v    ItemVersion
		want string // "" valid, else a part of the refusal
	}{
		{"none declared", ItemVersion{Includes: inc}, ""},
		{"one unmarked", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{}}}, ""},
		{"two, one main", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Main: true, Sidecars: sc("a")}, {Name: "y", Sidecars: sc("b")}}}, ""},
		{"two, no main", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Sidecars: sc("a")}, {Name: "y", Sidecars: sc("b")}}}, "exactly one main"},
		{"two mains", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Main: true, Sidecars: sc("a")}, {Name: "y", Main: true, Sidecars: sc("b")}}}, "exactly one main"},
		{"unnamed", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Main: true, Sidecars: sc("a")}, {Name: "y", Sidecars: sc("b")}}}, "names each one"},
		{"named twice", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Main: true, Sidecars: sc("a")}, {Name: "X", Sidecars: sc("b")}}}, "named twice"},
		{"unpinned sidecar", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Main: true, Sidecars: sc("a")}, {Name: "y", Sidecars: sc("c")}}}, "not a pinned member"},
		{"member nowhere", ItemVersion{Includes: inc, Universes: []SolutionUniverse{{Name: "x", Main: true, Sidecars: sc("a")}, {Name: "y", Sidecars: sc("a")}}}, "belongs to no universe"},
		{"two seeds", ItemVersion{Includes: inc, Seed: &PinnedRef{Name: "s1"}, Universes: []SolutionUniverse{{Main: true, Seed: &PinnedRef{Name: "s2"}}}}, "two seeds"},
	} {
		err := tc.v.ValidateUniverses()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: refused: %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// The main universe comes first, whatever its place in the list, and only it
// gets the version's config-sets; each other one gets its own members.
func TestTheMainUniverseComesFirst(t *testing.T) {
	v := ItemVersion{
		Includes:   []PinnedRef{{Name: "a", Version: "1"}, {Name: "b", Version: "1"}},
		ConfigSets: []PinnedRef{{Name: "main-cs", Version: "1"}},
		Universes: []SolutionUniverse{
			{Name: "y", Sidecars: []SolutionSidecar{{Name: "b"}}, ConfigSets: []PinnedRef{{Name: "y-cs", Version: "1"}}},
			{Name: "x", Main: true, Sidecars: []SolutionSidecar{{Name: "a"}}},
		},
	}
	us := v.SolutionUniverses()
	if len(us) != 2 || us[0].Name != "x" || !us[0].Main || us[1].Main {
		t.Fatalf("universes %+v, want x (main) first", us)
	}
	if len(us[0].ConfigSets) != 1 || us[0].ConfigSets[0].Name != "main-cs" || len(us[1].ConfigSets) != 1 || us[1].ConfigSets[0].Name != "y-cs" {
		t.Errorf("config-sets: main %+v, y %+v", us[0].ConfigSets, us[1].ConfigSets)
	}
	if m := v.UniverseMembers(us[1]); len(m) != 1 || m[0].Name != "b" {
		t.Errorf("y's members %+v, want b", m)
	}
}
