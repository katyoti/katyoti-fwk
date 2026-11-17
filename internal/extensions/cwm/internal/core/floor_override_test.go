// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A project's override entry of a host CWM type wins over the floor: the
// catalog's declaration of a floor edge type, carrying a project's override
// (its member's Overrides name it from project:<token>), is the type the
// store serves; the same declaration with no override never shadows the
// floor; the override gone, the floor's is in force again.
func TestAProjectOverrideOfAHostTypeWinsOverTheFloor(t *testing.T) {
	body := `{"name":"parent_id","from":"node","to":"node","cardinality":"N:1","description":"from the project"}`
	entry := contribapi.Contribution{Provider: "ktw-example", Type: "edge-type", ID: "parent_id",
		Layers: contribapi.InlineData(json.RawMessage(body))}
	member := func(overridden bool) *mbapi.Manifest {
		m := &mbapi.Manifest{Provider: "ktw-example", Contributions: []contribapi.Contribution{entry}}
		if overridden {
			m.Overrides = []contribapi.Override{{URI: entry.ResolvedURI(), Layer: "source", From: contribapi.ProjectOrigin("acme")}}
		}
		return m
	}
	served := func(overridden bool) (*Service, string) {
		d := api.DeclarationsFromManifests([]*mbapi.Manifest{member(overridden)}, nil)
		cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
		cfg.CatalogTypes = CatalogTypeSet{Edges: d.Edges, Sources: d.Sources, Members: d.Members, Named: d.Named, Overridden: d.Overridden}
		svc, err := New(stdctx.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = svc.Close() })
		et, _ := svc.Storage.Types().EdgeType(api.EdgeTypeParentID)
		return svc, et.Description
	}
	if _, got := served(false); got == "from the project" {
		t.Fatal("a catalog declaration with no project override shadowed the floor type")
	}
	svc, got := served(true)
	if got != "from the project" {
		t.Fatalf("parent_id served = %q; want the project's override", got)
	}
	// The override gone at a refresh: the floor's type again.
	d := api.DeclarationsFromManifests([]*mbapi.Manifest{member(false)}, nil)
	svc.RefreshCatalogTypes(CatalogTypeSet{Edges: d.Edges, Sources: d.Sources, Members: d.Members, Named: d.Named, Overridden: d.Overridden})
	if et, _ := svc.Storage.Types().EdgeType(api.EdgeTypeParentID); et.Description == "from the project" {
		t.Fatal("the override is gone and the store still serves it")
	}
}

// A universe override (From: universe) is no project override: it never
// marks the declaration overridden.
func TestOnlyAProjectOriginMarksADeclarationOverridden(t *testing.T) {
	entry := contribapi.Contribution{Provider: "ktw-example", Type: "edge-type", ID: "rel",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"rel","from":"any","to":"any"}`))}
	m := &mbapi.Manifest{Provider: "ktw-example", Contributions: []contribapi.Contribution{entry},
		Overrides: []contribapi.Override{{URI: entry.ResolvedURI(), Layer: "source", From: "universe"}}}
	if d := api.DeclarationsFromManifests([]*mbapi.Manifest{m}, nil); len(d.Overridden) != 0 {
		t.Fatalf("overridden = %v; want none for a universe override", d.Overridden)
	}
}
