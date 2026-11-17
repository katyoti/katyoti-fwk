// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// twoUniverses are a main universe wiring ktw-a and another wiring ktw-b, each
// a binary outside the store, and a catalog (t.TempDir) publishing their
// very bytes.
func twoUniverses(t *testing.T) (main, other, market string) {
	t.Helper()
	dir := t.TempDir()
	market = filepath.Join(dir, "market")
	var items []mktapi.Item
	universe := func(name, member string) string {
		bin := filepath.Join(dir, "bin", member+".exe")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte("bytes of "+member), 0o755); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte("bytes of " + member))
		items = append(items, mktapi.Item{Name: member, Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0",
			Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: hex.EncodeToString(sum[:])}}}}})
		home := filepath.Join(dir, name)
		if err := catalog.SaveManifest(home, &catalog.Manifest{Name: name, DisplayName: strings.ToUpper(name),
			Sidecars: []orchapi.SidecarSpec{{Name: member, Transport: "nats", Binary: bin}}}); err != nil {
			t.Fatal(err)
		}
		return home
	}
	main, other = universe("studio", "ktw-a"), universe("review", "ktw-b")
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(market, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(market, "katyoti-marketplace.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return main, other, market
}

// snapshot-universe publishes the universe snapshotted as the main one and
// each --other beside it, with its seed (--other-project) and its config-sets
// (--other-config-set).
func TestSnapshotUniversePublishesTheOthersBesideTheMain(t *testing.T) {
	main, other, market := twoUniverses(t)
	project := filepath.Join(t.TempDir(), "review-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := (&Service{}).SnapshotUniverse(context.Background(), api.SnapshotUniverseInput{
		Workspace: main, Version: "v1", Marketplace: market, Name: "duo",
		Others:          []string{"review=" + other},
		OtherProjects:   []string{"review=" + project},
		OtherConfigSets: []string{"review=review-base@v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(out.Universes, ",") != "studio,review" || strings.Join(out.OtherSeeds, ",") != "duo-review-seed@v1" {
		t.Errorf("universes %v, seeds %v; want studio (main), review and duo-review-seed@v1", out.Universes, out.OtherSeeds)
	}
	raw, err := os.ReadFile(filepath.Join(market, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cat mktapi.Catalog
	if err := config.DecodeBytes(raw, marshal.YAML, &cat); err != nil {
		t.Fatal(err)
	}
	for _, it := range cat.Items {
		if it.Name != "duo" {
			continue
		}
		us := it.Versions[0].SolutionUniverses()
		if len(us) != 2 || !us[0].Main || us[0].Name != "studio" || us[1].Main || us[1].Name != "review" {
			t.Fatalf("universes %+v; want studio (main) and review", us)
		}
		if us[1].Seed == nil || us[1].Seed.Name != "duo-review-seed" || len(us[1].ConfigSets) != 1 || us[1].ConfigSets[0].Name != "review-base" {
			t.Errorf("review %+v; want its seed and review-base", us[1])
		}
		return
	}
	t.Fatal("no solution duo in the catalog")
}

// A seed or a config-set for a universe no --other names, or an --other that
// is not <name>=<universe>, is refused before anything is published.
func TestSnapshotUniverseRefusesAnOtherItCannotPlace(t *testing.T) {
	main, other, market := twoUniverses(t)
	for want, in := range map[string]api.SnapshotUniverseInput{
		"no --other names the universe nope": {OtherProjects: []string{"nope=" + main}},
		"--other must be <name>=<value>":     {Others: []string{other}},
		"--other review is named twice":      {Others: []string{"review=" + other, "review=" + other}},
	} {
		in.Workspace, in.Version, in.Marketplace = main, "v1", market
		if _, err := (&Service{}).SnapshotUniverse(context.Background(), in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error naming %q, got %v", want, err)
		}
	}
}
