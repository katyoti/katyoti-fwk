// SPDX-License-Identifier: Apache-2.0

package testing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib"
)

// RepoRoot walks up from the working directory to the module root.
func RepoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// ShippedCatalog is a catalog holding ktw-docs' shipped drawio content
// (app/ktw-docs/embed/drawio), decoded by the same decoders the provider
// runs at boot, under the ktw-docs provider.
func ShippedCatalog(t testing.TB) *Catalog {
	t.Helper()
	return CatalogFrom(t, NewCatalog(nil), "ktw-docs", filepath.Join(RepoRoot(t), "app", "ktw-docs", "embed", "drawio"))
}

// AppFluxViewpoint is the test viewpoint TestCatalog adds: the app-flux
// notation (testdata/app-flux), one node kind whose looks come from its
// attributes - the domain palette, the xls marker's badge, the flow type's
// colour, the interface codes' links.
const AppFluxViewpoint = "drawio-test:/viewpoint/app-flux"

// TestCatalog is ShippedCatalog plus the app-flux test notation, viewpoint
// and template library under the drawio-test provider: what a test drawing
// the variant mechanism reads, none of it shipped.
func TestCatalog(t testing.TB) *Catalog {
	t.Helper()
	return CatalogFrom(t, ShippedCatalog(t), "drawio-test", filepath.Join(RepoRoot(t), "internal", "extensions", "drawio", "testing", "testdata", "app-flux"))
}

// CatalogFrom adds to cat the drawio content of a folder laid out as
// contrib.ContributeDrawio reads it (notations/, viewpoints/,
// drawio-templates/; a missing folder is skipped), under provider, decoded
// by the same decoders, and returns cat.
func CatalogFrom(t testing.TB, cat *Catalog, provider, root string) *Catalog {
	t.Helper()
	if _, err := os.Stat(root); err != nil {
		return cat
	}
	// The pack read by the very providers a host publishes it through, from a
	// loader rooted at the folder.
	for _, p := range contrib.Providers(provider, loader.NewFSLoader(root), ".") {
		entries, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("read %s/%s: %v", root, p.ResourceType, err)
		}
		if drops := p.Drops(); len(drops) > 0 {
			t.Fatalf("%s: %s files not read: %+v", root, p.ResourceType, drops)
		}
		for _, c := range entries {
			cat.Add(contribapi.Contribution{
				Provider: provider, Type: c.Type, ID: c.ID,
				URI:    provider + ":/" + c.Type + "/" + c.ID,
				Layers: contribapi.InlineData(c.Data()),
			})
		}
	}
	return cat
}
