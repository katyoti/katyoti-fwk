// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// carriedOverrideManifests are a member serving a persona and a project
// carrying an override of it (and one of an entry no member serves).
func carriedOverrideManifests() (*mbapi.Manifest, *mbapi.Manifest) {
	x := makeManifest("ktw-x", "1.0.0", "")
	persona := contribapi.Contribution{Provider: "ktw-x", Type: "persona", ID: "reviewer"}
	persona.SetLayer("prompt", []byte("You review."), "text/markdown")
	persona.SetLayer("i18n.fr", []byte("Tu relis."), "text/markdown")
	x.Contributions = []contribapi.Contribution{persona}
	ReHash(x)
	acme := makeManifest("acme", "folder", "")
	acme.OverrideEntries = []contribapi.OverrideEntry{
		overrideOf("ktw-x:/persona/reviewer", "persona", "reviewer-fr", false, map[string]string{"i18n.fr": "Vous relisez."}),
		overrideOf("ktw-x:/persona/gone", "persona", "gone", false, map[string]string{"prompt": "Nothing."}),
	}
	ReHash(acme)
	return x, acme
}

type overrideReader interface {
	ContributionAt(uri string) (contribapi.Contribution, bool)
	Get(ctx context.Context, name string) (*mbapi.Manifest, error)
	OverrideFindings() []string
}

// Both catalogs apply the override entries a member carries the same way: a
// reader on the in-memory catalog serves the overridden layer, the record and
// the finding the disk-backed catalog serves.
func TestBothCatalogsServeACarriedOverride(t *testing.T) {
	cache := NewCache(t.TempDir())
	x, acme := carriedOverrideManifests()
	if err := SeedCacheSave(t, cache, x); err != nil {
		t.Fatal(err)
	}
	if err := SeedCacheSave(t, cache, acme); err != nil {
		t.Fatal(err)
	}
	disk, err := NewCatalog(cache, MembersOf(cache, "ktw-x", "acme"))
	if err != nil {
		t.Fatal(err)
	}
	mx, macme := carriedOverrideManifests()
	mem := NewMemoryCatalog([]*mbapi.Manifest{mx, macme}, []string{"ktw-x", "acme"})

	check := func(name string, c overrideReader) {
		t.Helper()
		e, ok := c.ContributionAt("ktw-x:/persona/reviewer")
		if !ok {
			t.Fatalf("%s: reviewer not in the catalog", name)
		}
		if string(e.Layers["i18n.fr"].Inline) != "Vous relisez." || string(e.Layers["prompt"].Inline) != "You review." {
			t.Fatalf("%s: reviewer = fr %q prompt %q; want the project's French and the binary's prompt", name, e.Layers["i18n.fr"].Inline, e.Layers["prompt"].Inline)
		}
		got, err := c.Get(context.Background(), "ktw-x")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Overrides) != 1 || got.Overrides[0] != (contribapi.Override{URI: "ktw-x:/persona/reviewer", Layer: "i18n.fr", From: "project:acme"}) {
			t.Fatalf("%s: ktw-x's override record = %+v", name, got.Overrides)
		}
		f := c.OverrideFindings()
		if len(f) != 1 || !strings.Contains(f[0], "ktw-x:/persona/gone") || !strings.Contains(f[0], "no member serves") {
			t.Fatalf("%s: findings = %v; want the override of an entry no member serves", name, f)
		}
	}
	check("CatalogImpl", disk)
	check("MemoryCatalog", mem)

	// A rebuild over the same manifests (an incremental Put) keeps one record.
	mem.Put("acme", macme)
	check("MemoryCatalog after Put", mem)
}
