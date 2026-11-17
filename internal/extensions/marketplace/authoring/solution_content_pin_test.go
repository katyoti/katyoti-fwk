// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// releaseWorld is a universe whose one member runs a binary outside the store
// (as a scenario case runs copies in its bin/), and a catalog that publishes a
// version of that member with the given sha256.
func releaseWorld(t *testing.T, publishedSum func(binSum string) string) (home, catalogPath string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "case", "bin", "ktw-x.exe")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("the very bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("the very bytes"))
	home = filepath.Join(dir, "case", "universe")
	if err := catalog.SaveManifest(home, &catalog.Manifest{Name: "scn-demo", DisplayName: "Demo",
		Sidecars: []orchapi.SidecarSpec{{Name: "ktw-x", Transport: "nats", Binary: bin}}}); err != nil {
		t.Fatal(err)
	}
	market := filepath.Join(dir, "market")
	catalogPath = filepath.Join(market, "katyoti-marketplace.yaml")
	cat := mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: []mktapi.Item{{
		Name: "ktw-x", Kind: mktapi.KindSidecar,
		Versions: []mktapi.ItemVersion{
			{Version: "2.0.0", Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: "00"}}},
			{Version: "1.4.0", Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: publishedSum(hex.EncodeToString(sum[:]))}}},
		},
	}}}
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(market, "infra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(market, "infra", "channel.yaml"), []byte("version: v7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, catalogPath
}

// A member outside the store is pinned to the catalog version that publishes
// its very bytes, and the release records the infra version published beside
// the catalog.
func TestAddRelease_PinsAMemberByContent(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return strings.ToUpper(s) })
	r, err := New().AddSolution(home, "v1", "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{Description: "The demo dossier"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Members) != 1 || r.Members[0].Version != "1.4.0" {
		t.Fatalf("members = %+v, want ktw-x@1.4.0 (the version with its bytes)", r.Members)
	}
	if r.Infra != "v7" {
		t.Fatalf("infra = %q, want v7", r.Infra)
	}
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cat.Items {
		if it.Name != "demo" {
			continue
		}
		if len(it.Versions) != 1 || it.Versions[0].Infra != "v7" {
			t.Fatalf("the release does not record its infra: %+v", it.Versions)
		}
		if it.Description != "The demo dossier" {
			t.Errorf("description = %q, want the one given", it.Description)
		}
		// An install derives each member's path from its own store: the
		// publisher's paths are not recorded.
		for _, sc := range it.Versions[0].Universes[0].Sidecars {
			if strings.ContainsAny(sc.Binary, `/\`) {
				t.Errorf("member %s recorded with the publisher's path %s", sc.Name, sc.Binary)
			}
		}
	}
}

// Bytes no catalog version publishes are refused: the release could not be
// reproduced.
func TestAddRelease_RefusesBytesNotPublished(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(string) string { return "ff" })
	_, err := New().AddSolution(home, "v1", "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{})
	if err == nil || !strings.Contains(err.Error(), "ktw-x") {
		t.Fatalf("err = %v, want a refusal naming ktw-x", err)
	}
}

// The solution's gui section is the one the spec gives (a scenario case
// declares it), else the universe's own.
func TestAddRelease_TheSpecGUIReplacesTheUniverses(t *testing.T) {
	home, catalogPath := releaseWorld(t, func(s string) string { return s })
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	m.GUI = &cenv.UniverseGUI{Hide: []string{"runs"}}
	if err := catalog.SaveManifest(home, m); err != nil {
		t.Fatal(err)
	}
	given := &cenv.UniverseGUI{Hide: []string{"planning-deck", "debug"}, First: []string{"archifactory"}}
	for _, tc := range []struct {
		version string
		spec    *cenv.UniverseGUI
		want    *cenv.UniverseGUI
	}{
		{"v1", given, given},
		{"v2", nil, &cenv.UniverseGUI{Hide: []string{"runs"}}},
	} {
		if _, err := New().AddSolution(home, tc.version, "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{GUI: tc.spec}); err != nil {
			t.Fatal(err)
		}
		cat, err := loadCatalogFile(catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range cat.Items {
			if it.Name == "demo" {
				if got := it.Versions[0].Universes[0].GUI; !reflect.DeepEqual(got, tc.want) {
					t.Errorf("%s: gui = %+v, want %+v", tc.version, got, tc.want)
				}
			}
		}
	}
}
