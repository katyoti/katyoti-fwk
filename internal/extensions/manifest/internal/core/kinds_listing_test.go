// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The list of lists: every declared kind with its description and its entry
// count, read from the kind declarations and the index — a kind with no entry
// listed at 0, and each count what a Query of that kind returns, after a
// reindex too.

// publishKinds builds the member's manifest declaring listed-kind and an
// empty kind, with the given entries of listed-kind, and saves it.
func publishKinds(t *testing.T, cache *Cache, entries ...contribapi.Contribution) {
	t.Helper()
	empty := kindProvider(contribapi.KindDeclaration{Name: "empty-kind", Description: "A kind nobody filled",
		Schema: "ktw-test:/data-type/y", Validate: "warn", WarnReason: "a test kind with no schema shipped",
		Layers: map[string]contribapi.LayerDecl{"data": {Default: true, Required: true}},
		Meta:   &contribapi.MetaDecl{Budget: 512}})
	listed := listedKind()
	listed.entries = append(listed.entries, empty.entries...)
	m, err := NewBuilder("ktw-test", "0").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{listed, &residencyProvider{typ: "listed-kind", entries: entries}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func listOfKinds(t *testing.T, q mbapi.Q) map[string]mbapi.KindListing {
	t.Helper()
	got, err := mbapi.ListKinds(context.Background(), q)
	if err != nil {
		t.Fatalf("ListKinds: %v", err)
	}
	out := map[string]mbapi.KindListing{}
	for _, k := range got {
		out[k.Name] = k
	}
	return out
}

func TestTheListOfKindsCountsEveryKindsEntries(t *testing.T) {
	ctx := context.Background()
	cache := NewCache(t.TempDir())
	publishKinds(t, cache,
		listedEntry("alpha", "Alpha", "The first entry"),
		listedEntry("beta", "Beta", "The second entry"))
	cat := listedCatalog(t, cache)
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}

	kinds, err := mbapi.KindsAt(ctx, q)
	if err != nil {
		t.Fatalf("KindsAt: %v", err)
	}
	got := listOfKinds(t, q)
	if len(got) != kinds.Len() {
		t.Errorf("listed %d kinds, the view declares %d", len(got), kinds.Len())
	}
	for _, name := range kinds.Names() {
		k, ok := got[name]
		if !ok {
			t.Errorf("kind %s is declared but not listed", name)
			continue
		}
		decl, _ := kinds.Get(name)
		if k.Description != decl.Description {
			t.Errorf("kind %s listed with description %q, declared %q", name, k.Description, decl.Description)
		}
		qk := q
		qk.Kind = name
		entries, err := mbapi.Query(ctx, qk)
		if err != nil {
			t.Fatalf("Query %s: %v", name, err)
		}
		if k.Count != len(entries) {
			t.Errorf("kind %s counted %d, a Query of it returns %d", name, k.Count, len(entries))
		}
	}
	if k := got["listed-kind"]; k.Count != 2 || k.Description != "A kind a person lists" {
		t.Errorf("listed-kind = %+v, want 2 entries and its description", k)
	}
	if k, ok := got["empty-kind"]; !ok || k.Count != 0 {
		t.Errorf("empty-kind = %+v (listed %v), want listed at 0", k, ok)
	}

	// One entry removed and reindexed: the count follows the index.
	publishKinds(t, cache, listedEntry("alpha", "Alpha", "The first entry"))
	if err := cat.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if err := cat.Reindex(ctx, nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	if k := listOfKinds(t, q)["listed-kind"]; k.Count != 1 {
		t.Errorf("after removing an entry listed-kind counts %d, want 1", k.Count)
	}

	entries, err := mbapi.ListKindEntries(ctx, q, "listed-kind")
	if err != nil {
		t.Fatalf("ListKindEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].URI != "ktw-test:/listed-kind/alpha" ||
		entries[0].Name != "Alpha" || entries[0].Description != "The first entry" {
		t.Errorf("listed-kind entries = %+v, want alpha with its name and description", entries)
	}
	if cat.bodies != 0 || cat.layers != 0 {
		t.Errorf("the list of lists fetched %d bodies and opened %d layers; the index alone answers it", cat.bodies, cat.layers)
	}
	if _, err := mbapi.ListKindEntries(ctx, q, "no-such-kind"); err == nil {
		t.Error("an undeclared kind listed its entries; want an error")
	}
}
