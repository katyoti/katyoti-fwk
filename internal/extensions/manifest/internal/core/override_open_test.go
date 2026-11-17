// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// overrideLayerOpener is what both catalogs answer: an entry by URI, a layer
// opened, the override findings.
type overrideLayerOpener interface {
	ContributionAt(uri string) (contribapi.Contribution, bool)
	OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error)
	OverrideFindings() []string
}

// overrideOpenManifests are a binary serving a persona and a project carrying
// two overrides of it: one in the inline band, one in the bulk band.
func overrideOpenManifests() (x, acme *mbapi.Manifest, bulk []byte) {
	x = makeManifest("ktw-x", "1.0.0", "")
	persona := contribapi.Contribution{Provider: "ktw-x", Type: "persona", ID: "reviewer"}
	persona.SetLayer("prompt", []byte("You review."), "text/markdown")
	persona.SetLayer("i18n.fr", []byte("Tu relis."), "text/markdown")
	x.Contributions = []contribapi.Contribution{persona}
	ReHash(x)
	bulk = bytes.Repeat([]byte("p"), contribapi.BandControl+1)
	big := overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-big", false, nil)
	big.Entry.SetLayer("prompt", bulk, "text/markdown")
	acme = makeManifest("acme", "folder", "")
	acme.OverrideEntries = []contribapi.OverrideEntry{
		overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-fr", false, map[string]string{"i18n.fr": "Vous relisez."}),
		big,
	}
	ReHash(acme)
	return x, acme, bulk
}

// openText opens one layer through the catalog's OpenLayer and reads it.
func openText(t *testing.T, c overrideLayerOpener, uri, layer string) string {
	t.Helper()
	rc, _, err := c.OpenLayer(context.Background(), uri, layer)
	if err != nil {
		t.Fatalf("open %s#%s: %v", uri, layer, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// An applied override layer opens to the override's own bytes, in either
// catalog: the inline band is served from the index. A bulk-band override
// layer is never applied (its bytes would be fetched by the base entry's
// URI, from the base's producer): it is a finding, and the layer opens to
// the base's bytes under the base's address.
func TestAnAppliedOverrideLayerOpensItsOwnBytes(t *testing.T) {
	cache := NewCache(t.TempDir())
	x, acme, _ := overrideOpenManifests()
	if err := SeedCacheSave(t, cache, x); err != nil {
		t.Fatal(err)
	}
	if err := SeedCacheSave(t, cache, acme); err != nil {
		t.Fatal(err)
	}
	impl, err := NewCatalog(cache, MembersOf(cache, "ktw-x", "acme"))
	if err != nil {
		t.Fatal(err)
	}
	mx, macme, _ := overrideOpenManifests()
	for name, c := range map[string]overrideLayerOpener{
		"catalog": impl,
		"memory":  NewMemoryCatalog([]*mbapi.Manifest{mx, macme}, nil),
	} {
		t.Run(name, func(t *testing.T) {
			if got := openText(t, c, "ktw-x:/persona/reviewer", "i18n.fr"); got != "Vous relisez." {
				t.Fatalf("the overridden French layer opens to %q; want the override's bytes", got)
			}
			if got := openText(t, c, "ktw-x:/persona/reviewer", "prompt"); got != "You review." {
				t.Fatalf("the prompt opens to %q; want the base's: a bulk-band override is not applied", got)
			}
			e, _ := c.ContributionAt("ktw-x:/persona/reviewer")
			if e.Layers["prompt"].Hash != contribapi.HashContent([]byte("You review.")) {
				t.Fatalf("the prompt's address = %s; want the base's", e.Layers["prompt"].Hash)
			}
			found := false
			for _, f := range c.OverrideFindings() {
				if strings.Contains(f, "acme:/persona/reviewer-big") && strings.Contains(f, "bulk band") {
					found = true
				}
			}
			if !found {
				t.Fatalf("findings = %v; want the bulk-band override refused", c.OverrideFindings())
			}
		})
	}
}

// The builder refuses a bulk-band override layer of an entry it serves as a
// drop naming the override, and applies the others.
func TestTheBuilderRefusesABulkBandOverrideLayer(t *testing.T) {
	persona := contribapi.Contribution{Provider: "ktw-x", Type: "persona", ID: "reviewer"}
	persona.SetLayer("prompt", []byte("You review."), "text/markdown")
	base := staticProvider{provider: "ktw-x", typ: "persona", entries: []contribapi.Contribution{persona}}
	big := overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-big", false, nil)
	big.Entry.SetLayer("prompt", bytes.Repeat([]byte("p"), contribapi.BandControl+1), "text/markdown")
	project := overrideEntryProvider{staticProvider: staticProvider{provider: "acme", typ: "persona"}, ovs: []contribapi.OverrideEntry{big}}
	m, err := NewBuilder("ktw-x", "dev").BuildFromProviders(context.Background(), []contribapi.ContributionProvider{base, project})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range m.Contributions {
		if c.ResolvedURI() == "ktw-x:/persona/reviewer" && c.Layers["prompt"].Hash != contribapi.HashContent([]byte("You review.")) {
			t.Fatalf("a bulk-band override layer was applied: %+v", c.Layers["prompt"])
		}
	}
	refused := false
	for _, d := range m.Drops {
		if d.Path == "acme:/persona/reviewer-big" && strings.Contains(d.Reason, "bulk band") {
			refused = true
		}
	}
	if !refused {
		t.Fatalf("drops = %+v; want the bulk-band override refused", m.Drops)
	}
}
