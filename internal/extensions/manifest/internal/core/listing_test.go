// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Listing a kind reads the index alone: each entry comes back with its URI,
// its name and the description its producer wrote once in its data layer —
// derived into meta by the builder — while no layer is opened and no body
// fetched.

// countingCatalog is the hub catalog, counting every read of content: the
// body lane and the layer lane.
type countingCatalog struct {
	*CatalogImpl
	bodies, layers int
}

func (c *countingCatalog) GetBody(ctx context.Context, uri string) ([]byte, error) {
	c.bodies++
	return c.CatalogImpl.GetBody(ctx, uri)
}

func (c *countingCatalog) OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	c.layers++
	return c.CatalogImpl.OpenLayer(ctx, uri, layer)
}

// listedKind declares a kind whose entries derive name and description.
func listedKind() *residencyProvider {
	return kindProvider(contribapi.KindDeclaration{Name: "listed-kind", Description: "A kind a person lists",
		Schema: "ktw-test:/data-type/x", Validate: "warn", WarnReason: "a test kind with no schema shipped",
		Layers: map[string]contribapi.LayerDecl{"data": {Default: true, Required: true}},
		Meta:   &contribapi.MetaDecl{Budget: 512, From: "data", Fields: []string{"name", "description"}}})
}

// listedEntry is one entry of listed-kind; its producer writes the media type
// only, the picker fields are in its data.
func listedEntry(id, name, description string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "ktw-test", Type: "listed-kind", ID: id}
	c.SetData([]byte(`{"name":"` + name + `","description":"` + description + `"}`))
	c.Metadata = &contribapi.ContributionMeta{MediaType: "application/json"}
	return c
}

// publishListed builds the member's manifest from its providers and saves it
// to the catalog's cache, as a member's publication does.
func publishListed(t *testing.T, cache *Cache, entries ...contribapi.Contribution) {
	t.Helper()
	m, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{listedKind(), &residencyProvider{typ: "listed-kind", entries: entries}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func listKind(t *testing.T, cat *countingCatalog) map[string]contribapi.Contribution {
	t.Helper()
	got, err := mbapi.Query(context.Background(), mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "listed-kind", Catalog: cat})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	out := map[string]contribapi.Contribution{}
	for _, c := range got {
		out[c.ResolvedURI()] = c
	}
	return out
}

func listedCatalog(t *testing.T, cache *Cache) *countingCatalog {
	t.Helper()
	cat, err := NewCatalog(cache, MembersOf(cache, "ktw-test"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	return &countingCatalog{CatalogImpl: cat}
}

func TestListingAKindOpensNoLayer(t *testing.T) {
	cache := NewCache(t.TempDir())
	publishListed(t, cache,
		listedEntry("alpha", "Alpha", "The first entry"),
		listedEntry("beta", "Beta", "The second entry"))
	cat := listedCatalog(t, cache)

	got := listKind(t, cat)
	want := map[string][2]string{
		"ktw-test:/listed-kind/alpha": {"Alpha", "The first entry"},
		"ktw-test:/listed-kind/beta":  {"Beta", "The second entry"},
	}
	if len(got) != len(want) {
		t.Fatalf("listed %d entries, want %d: %v", len(got), len(want), got)
	}
	for uri, w := range want {
		c, ok := got[uri]
		if !ok || c.Metadata == nil || c.Metadata.Name != w[0] || c.Metadata.Description != w[1] {
			t.Errorf("%s listed as %+v, want name %q and description %q from the index", uri, c.Metadata, w[0], w[1])
		}
	}
	if cat.bodies != 0 || cat.layers != 0 {
		t.Errorf("listing a kind fetched %d bodies and opened %d layers; the index alone answers it", cat.bodies, cat.layers)
	}
}

func TestAListedDescriptionFollowsItsEntryAfterReindex(t *testing.T) {
	cache := NewCache(t.TempDir())
	publishListed(t, cache, listedEntry("alpha", "Alpha", "Before the change"))
	cat := listedCatalog(t, cache)
	if c := listKind(t, cat)["ktw-test:/listed-kind/alpha"]; c.Metadata == nil || c.Metadata.Description != "Before the change" {
		t.Fatalf("first listing: %+v", c.Metadata)
	}

	// The producer changes the description in its data layer; the member
	// publishes again and the catalog reindexes.
	publishListed(t, cache, listedEntry("alpha", "Alpha", "After the change"))
	if err := cat.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	c := listKind(t, cat)["ktw-test:/listed-kind/alpha"]
	if c.Metadata == nil || c.Metadata.Description != "After the change" {
		t.Errorf("after the reindex the entry is listed with %+v, want the description its data now carries", c.Metadata)
	}
	if cat.bodies != 0 || cat.layers != 0 {
		t.Errorf("listing fetched %d bodies and opened %d layers", cat.bodies, cat.layers)
	}
}
