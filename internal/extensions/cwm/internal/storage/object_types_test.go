// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// TestLoadObjectTypesFromShippedPack is the falsifiable half of D1: the five
// declarations ktw-methods ships must actually LOAD.
//
// It exists because the first cut of these files was inert — they sat in an
// `object-types/` folder (hyphen) as .json, which no loader reads, so
// CatalogTypes.Objects stayed empty and validation never fired. A test that
// only exercised a fixture would have stayed green through exactly that bug.
func TestLoadObjectTypesFromShippedPack(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	dir := filepath.Join(up, "app", "ktw-methods", "embed", "cwm")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("embed dir not reachable from test cwd: %s: %v", dir, err)
	}
	types, err := LoadObjectTypesLayered(os.DirFS(dir))
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	byName := map[string][]string{}
	says := map[string]bool{} // drawn, or fields of its own
	for _, tp := range types {
		byName[tp.Name] = tp.Capabilities
		says[tp.Name] = tp.Drawn() || tp.Fields != ""
	}
	for _, want := range []string{"feature", "use-case", "documentation"} { // the project types ship with CWM itself
		if _, ok := byName[want]; !ok {
			t.Errorf("shipped object-type %q not loaded (got %v)", want, keys(byName))
		}
	}
	// A declaration with no capabilities, no fields and not drawn names
	// something without saying what it is. (A drawn type with none is a pure
	// container: `group`.)
	for name, caps := range byName {
		if len(caps) == 0 && !says[name] {
			t.Errorf("object-type %q declares no capabilities, no fields and is not drawn", name)
		}
	}
}

func keys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A later source overrides an earlier one by name — the same closer-wins
// layering node types get, which is what makes a workspace overlay work.
func TestLoadObjectTypesLayered_CloserWins(t *testing.T) {
	floor := fstest.MapFS{
		"object-types/use-case.yaml": &fstest.MapFile{Data: []byte("name: use-case\nicon: floor\n")},
		"object-types/module.yaml":   &fstest.MapFile{Data: []byte("name: module\nicon: package\n")},
	}
	overlay := fstest.MapFS{
		"object-types/use-case.yaml": &fstest.MapFile{Data: []byte("name: use-case\nicon: overlay\n")},
	}
	types, err := LoadObjectTypesLayered(floor, overlay)
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("want 2 types after layering, got %d: %v", len(types), types)
	}
	for _, tp := range types {
		if tp.Name == "use-case" && tp.Icon != "overlay" {
			t.Errorf("overlay did not win: icon=%q", tp.Icon)
		}
	}
}

// One malformed file must not take the pack down — same resilience the
// node/item/edge loaders have. A nameless declaration is dropped too: it can
// never match an object's type field, but a non-empty declared set is exactly
// what switches validation ON, so counting it would be worse than dropping it.
func TestLoadObjectTypesLayered_SkipsBadEntries(t *testing.T) {
	src := fstest.MapFS{
		"object-types/good.yaml":    &fstest.MapFile{Data: []byte("name: good\ncapabilities: [container]\n")},
		"object-types/broken.yaml":  &fstest.MapFile{Data: []byte("name: [this is not a string\n")},
		"object-types/nameless.yml": &fstest.MapFile{Data: []byte("icon: ghost\n")},
		"object-types/_index.yaml":  &fstest.MapFile{Data: []byte("skipped by convention")},
		"object-types/README.md":    &fstest.MapFile{Data: []byte("not a type")},
	}
	types, err := LoadObjectTypesLayered(src)
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	if len(types) != 1 || types[0].Name != "good" {
		t.Fatalf("want only [good], got %v", types)
	}
}

// An absent object-types/ folder contributes nothing and is not an error —
// most packs ship none.
func TestLoadObjectTypesLayered_NoFolder(t *testing.T) {
	types, err := LoadObjectTypesLayered(fstest.MapFS{"node_types/x.yaml": &fstest.MapFile{Data: []byte("name: x\n")}})
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	if len(types) != 0 {
		t.Fatalf("want none, got %v", types)
	}
}

// The two compose types that used to `extends: compose-artifact` carry the
// containment they inherited, written out when the legacy types became object
// types: its `accept_parents` and its allowed children. Containment is
// ENFORCED, so a child that lost either would change what can be created under
// it; each keeps its own agent system text (nearest-wins held).
func TestShippedExtendingTypesKeepInheritance(t *testing.T) {
	up := filepath.Join("..", "..", "..", "..", "..")
	dir := filepath.Join(up, "app", "ktw-compose", "embed", "cwm")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("ktw-compose pack not reachable: %v", err)
	}
	types, err := LoadObjectTypesLayered(os.DirFS(dir))
	if err != nil {
		t.Fatalf("LoadObjectTypesLayered: %v", err)
	}
	byName := map[string]api.ObjectType{}
	for _, tp := range types {
		byName[tp.Name] = tp
	}
	parent, ok := byName["compose-artifact"]
	if !ok {
		t.Fatal("compose-artifact not in the pack — the fixture this guards is gone")
	}
	// A floor: if the parent stops declaring what the children inherit, this test
	// would pass while proving nothing.
	if len(parent.AllowedChildren) == 0 || len(parent.AcceptParents) == 0 {
		t.Fatal("compose-artifact no longer declares the containment its children inherit")
	}

	has := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}

	for _, child := range []string{"compose-skill", "compose-task"} {
		got, ok := byName[child]
		if !ok {
			t.Errorf("%s missing from the pack", child)
			continue
		}
		for _, want := range parent.AllowedChildren {
			if !has(got.AllowedChildren, want) {
				t.Errorf("%s lost inherited allowed child %q — containment is ENFORCED, "+
					"so this changes what can be created under it: %v", child, want, got.AllowedChildren)
			}
		}
		for _, want := range parent.AcceptParents {
			if !has(got.AcceptParents, want) {
				t.Errorf("%s lost inherited accept_parents %q: %v", child, want, got.AcceptParents)
			}
		}
		// Nearest-wins held: the child's OWN agent text is not overwritten by the
		// parent's. Both declare one, which is why the first version of this test
		// was wrong to expect the parent's.
		if agentText(got) == "" {
			t.Errorf("%s has no agent system text at all", child)
		}
		if agentText(got) == agentText(parent) {
			t.Errorf("%s took the parent's agent text over its own — nearest-wins broken", child)
		}
	}
}

// agentText is a type's agent system sections, joined.
func agentText(ot api.ObjectType) string {
	if ot.Agent == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range ot.Agent.System {
		b.WriteString(s.Text)
	}
	return b.String()
}
