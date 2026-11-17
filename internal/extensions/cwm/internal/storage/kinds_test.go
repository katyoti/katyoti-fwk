// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The type registry holds the EDGE types a pack declares (`edge-types/`); an
// object type is read by LoadObjectTypesLayered. Sources layer closer-wins.

const parentEdge = `name: parent_id
from: node
to: node
cardinality: N:1
`

func TestLoadTypesLayered_NoSources(t *testing.T) {
	reg, err := LoadTypesLayered()
	if err != nil {
		t.Fatalf("LoadTypesLayered(): %v", err)
	}
	if len(reg.EdgeTypes()) != 0 {
		t.Errorf("expected an empty registry; got %d edges", len(reg.EdgeTypes()))
	}
}

func TestLoadTypesLayered_EmbeddedSourceOnly(t *testing.T) {
	src := fstest.MapFS{"edge-types/parent_id.yaml": &fstest.MapFile{Data: []byte(parentEdge)}}
	reg, err := LoadTypesLayered(src)
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	if _, ok := reg.EdgeType(api.EdgeTypeParentID); !ok {
		t.Error("parent_id edge type not registered")
	}
}

// A cardinality outside 1:1 | 1:N | N:1 | M:N is REFUSED: the edge type is
// not registered (and the refusal logged), so a rule CWM cannot read never
// registers as no rule at all. Every allowed value, and none, registers.
func TestLoadTypesLayered_RefusesAnUnknownCardinality(t *testing.T) {
	src := fstest.MapFS{"edge-types/bad.yaml": &fstest.MapFile{Data: []byte("name: bad\nfrom: any\nto: any\ncardinality: \"2:3\"\n")}}
	for _, c := range []string{"1:1", "1:N", "N:1", "M:N", ""} {
		name := "ok-" + strings.ReplaceAll(c, ":", "-")
		src["edge-types/"+name+".yaml"] = &fstest.MapFile{Data: []byte("name: " + name + "\ncardinality: \"" + c + "\"\n")}
	}
	reg, err := LoadTypesLayered(src)
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	if _, ok := reg.EdgeType("bad"); ok {
		t.Error("an edge type with cardinality 2:3 was registered")
	}
	if got := len(reg.EdgeTypes()); got != 5 {
		t.Errorf("registered %d edge types, want the 5 valid ones: %v", got, reg.EdgeTypes())
	}
}

func TestLoadTypesLayered_DiskOverridesEmbedded(t *testing.T) {
	embedded := fstest.MapFS{"edge-types/serves.yaml": &fstest.MapFile{Data: []byte("name: serves\nfrom: any\nto: any\ninverse_name: a\n")}}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "edge-types"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "edge-types", "serves.yaml"), []byte("name: serves\nfrom: any\nto: any\ninverse_name: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadTypesLayered(embedded, os.DirFS(dir))
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	if e, _ := reg.EdgeType("serves"); e.InverseName != "b" {
		t.Errorf("inverse_name = %q, want b (the disk source overrides the embedded one)", e.InverseName)
	}
}

func TestLoadTypesLayered_MissingFolderIsSilent(t *testing.T) {
	reg, err := LoadTypesLayered(fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("no types here")}})
	if err != nil || len(reg.EdgeTypes()) != 0 {
		t.Fatalf("a source without edge-types/: %v, %d edges", err, len(reg.EdgeTypes()))
	}
}

func TestLoadTypesLayered_NilSourceAndUnderscoreSkipped(t *testing.T) {
	src := fstest.MapFS{
		"edge-types/parent_id.yaml": &fstest.MapFile{Data: []byte(parentEdge)},
		"edge-types/_index.yaml":    &fstest.MapFile{Data: []byte("should be skipped")},
		"edge-types/README.md":      &fstest.MapFile{Data: []byte("not yaml")},
	}
	reg, err := LoadTypesLayered(nil, src, nil)
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	if n := len(reg.EdgeTypes()); n != 1 {
		t.Errorf("edges = %d, want 1 — nil sources and _/README files are skipped", n)
	}
}

// A catalog snapshot's edge types replace the catalog's ones whole: a changed
// one is in force, one no longer named goes; the floor stays.
func TestTypeRegistry_ReplaceSwapsTheCatalogsEdgesKeepsTheFloor(t *testing.T) {
	reg, err := LoadTypesLayered(fstest.MapFS{"edge-types/contains.yaml": {Data: []byte(`{name: contains, from: any, to: any}`)}})
	if err != nil {
		t.Fatal(err)
	}
	reg.Merge([]api.EdgeTypeRegistration{{Name: "serves", InverseName: "served-by"}, {Name: "cites"}})
	reg.Replace([]api.EdgeTypeRegistration{{Name: "serves", InverseName: "changed"}, {Name: "verifies"}, {Name: "contains", InverseName: "SHADOW"}})
	if e, _ := reg.EdgeType("serves"); e.InverseName != "changed" {
		t.Errorf("serves after Replace = %q, want the changed declaration", e.InverseName)
	}
	if _, ok := reg.EdgeType("verifies"); !ok {
		t.Error("Replace did not add the new edge type 'verifies'")
	}
	if _, ok := reg.EdgeType("cites"); ok {
		t.Error("an edge type the catalog no longer names is still in force")
	}
	if e, ok := reg.EdgeType("contains"); !ok || e.InverseName == "SHADOW" {
		t.Errorf("the floor's 'contains' = %+v (present %v), want the floor's, untouched", e, ok)
	}
}

func TestTypeRegistry_Replace_ConcurrentWithReads(t *testing.T) {
	reg := &TypeRegistry{edges: map[api.EdgeType]api.EdgeTypeRegistration{}}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 300; i++ {
			reg.Replace([]api.EdgeTypeRegistration{{Name: "verifies"}})
		}
		close(done)
	}()
	for i := 0; i < 300; i++ {
		_, _ = reg.EdgeType("verifies")
		_ = reg.EdgeTypes()
	}
	<-done
}

// A source laid out before the rename (object_types/, edge_types/) is read by
// the floor loaders as the current one is.
func TestTheFloorReadsTheFormerFolderNames(t *testing.T) {
	src := fstest.MapFS{
		"object_types/memo.yaml": {Data: []byte("name: memo\n")},
		"edge_types/cites.yaml":  {Data: []byte("name: cites\nfrom: any\nto: any\n")},
	}
	types, err := LoadObjectTypesLayered(src)
	if err != nil || len(types) != 1 || types[0].Name != "memo" {
		t.Fatalf("object types = %+v, %v; want memo", types, err)
	}
	reg, err := LoadTypesLayered(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.EdgeType("cites"); !ok {
		t.Fatal("the edge type of the former folder is not loaded")
	}
}
