// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// TestStoreConsumesTypesFromOtherSidecar is the core proof of the
// unify-cwm-type-delivery capability: an object or edge type contributed by
// a DIFFERENT sidecar binary (provider "ktw-fake", not the store host) is
// resolved from the aggregate catalog and lands in the store's TypeRegistry —
// the SkillManager pattern applied to CWM types. Plus the floor-wins invariant:
// a remote contribution never shadows a structural floor type.
func TestStoreConsumesTypesFromOtherSidecar(t *testing.T) {
	mustBody := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	// A catalog whose ONLY manifest belongs to a different sidecar ("ktw-fake")
	// and carries an object type and an edge type, plus an edge type colliding
	// with a floor one to exercise floor-wins.
	manifest := &mbapi.Manifest{
		Provider: "ktw-fake",
		Contributions: []contribapi.Contribution{
			{Provider: "ktw-fake", Type: "object-type", ID: "fake-object", Layers: contribapi.InlineData(mustBody(api.ObjectType{Name: "fake-object", Label: "Fake"}))},
			{Provider: "ktw-fake", Type: "edge-type", ID: "fake-edge", Layers: contribapi.InlineData(mustBody(api.EdgeTypeRegistration{Name: api.EdgeType("fake-edge"), From: "a", To: "b"}))},
			// Collides with a floor edge type — the floor must win.
			{Provider: "ktw-fake", Type: "edge-type", ID: "serves", Layers: contribapi.InlineData(mustBody(api.EdgeTypeRegistration{Name: "serves", InverseName: "HIJACKED"}))},
		},
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{manifest}, []string{"ktw-fake"})

	set := LoadCWMTypesFromCatalog(context.Background(), cat)
	if len(set.Objects) != 1 || set.Objects[0].Name != "fake-object" || len(set.Edges) != 2 {
		t.Fatalf("LoadCWMTypesFromCatalog: objects=%+v edges=%d, want fake-object and 2 edges", set.Objects, len(set.Edges))
	}

	// The floor registry, as the store host ships it, then the catalog on top.
	reg, err := storage.LoadTypesLayered()
	if err != nil {
		t.Fatalf("LoadTypesLayered: %v", err)
	}
	reg.Merge([]api.EdgeTypeRegistration{{Name: "serves", InverseName: "served-by"}})
	reg.Merge(set.Edges)

	if _, ok := reg.EdgeType(api.EdgeType("fake-edge")); !ok {
		t.Error("remote edge-type 'fake-edge' not in the registry after the catalog is consumed")
	}
	if e, _ := reg.EdgeType("serves"); e.InverseName != "served-by" {
		t.Errorf("floor-wins violated: serves = %+v", e)
	}
}
