// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// aboveInline is larger than the inline band, so the strip helpers actually
// drop it. Anything inside the band is kept by design and would make these
// tests pass without exercising the drop at all.
func aboveInline() []byte { return bytes.Repeat([]byte("y"), int(contribapi.BandInline)+1) }

func withBody(id string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "ktw-demo", Type: "task", ID: id}
	c.SetData(aboveInline())
	return c
}

// TestStripBodies_LeavesTheCatalogIntact is the regression guard for the defect
// that made 21 body-bearing resources return empty from GetBody.
//
// stripBodies serves the INDEX plane: it hands out entries without their bytes.
// Its own comment states the hazard — "the catalog hands out shared
// Contribution values from its in-memory snapshot — mutating in place would
// corrupt the cache, so we copy each entry before nilling Body" — and the copy
// did not achieve that, because `Layers` is a map and a struct copy shares it.
//
// So every ListResources quietly emptied the hub's live catalog, and the
// CONTENT plane went dark for anything above the inline band. Nothing noticed:
// the census proves what a binary PUBLISHES, and this is what a consumer READS.
func TestStripBodies_LeavesTheCatalogIntact(t *testing.T) {
	cache := []contribapi.Contribution{withBody("a"), withBody("b")}
	wantHash := cache[0].DataHash()

	out := stripBodies(cache)

	if len(out[0].Data()) != 0 {
		t.Error("the index plane must not carry bytes above the inline band")
	}
	for i, c := range cache {
		if len(c.Data()) == 0 {
			t.Fatalf("cache entry %d lost its body to a LIST — "+
				"the index plane destroyed the content plane", i)
		}
	}
	if cache[0].DataHash() != wantHash {
		t.Errorf("cache descriptor changed: %s -> %s", wantHash, cache[0].DataHash())
	}
	// The descriptor must survive the strip, or a consumer reads "no body"
	// and never asks GetBody for the one that exists.
	if out[0].DataHash() != wantHash {
		t.Errorf("stripped entry lost its descriptor: %q", out[0].DataHash())
	}
}

// TestStripManifestBodies_LeavesTheCatalogIntact — the same defect on the
// manifest-shaped path (GetManifest / ListManifests).
func TestStripManifestBodies_LeavesTheCatalogIntact(t *testing.T) {
	m := &mbapi.Manifest{
		Provider:      "ktw-demo",
		Contributions: []contribapi.Contribution{withBody("a")},
	}
	wantHash := m.Contributions[0].DataHash()

	out := stripManifestBodies(m)

	if len(out.Contributions[0].Data()) != 0 {
		t.Error("stripped manifest must not carry bytes above the inline band")
	}
	if len(m.Contributions[0].Data()) == 0 {
		t.Fatal("the SOURCE manifest lost its body to a strip — the hub's cache is shared")
	}
	if m.Contributions[0].DataHash() != wantHash {
		t.Errorf("source descriptor changed: %s", m.Contributions[0].DataHash())
	}
}

// TestStripBodies_RepeatedListsStayReadable: the hub strips on EVERY list. One
// list is not the failure mode — a GUI that lists, then reads, then lists
// again is. This fails if the drop is destructive at any point in that cycle.
func TestStripBodies_RepeatedListsStayReadable(t *testing.T) {
	cache := []contribapi.Contribution{withBody("a")}
	for i := 0; i < 3; i++ {
		stripBodies(cache)
		if len(cache[0].Data()) == 0 {
			t.Fatalf("body gone after list #%d", i+1)
		}
	}
}
