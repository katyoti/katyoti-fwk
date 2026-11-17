// SPDX-License-Identifier: Apache-2.0

package core

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A case published as a solution carries the gui its test-case.yaml declares
// (what `katyoti solution install` writes into Univers.yaml), and its seed
// holds the universe's own tasks, never the simulated user's the tool
// installed into the case.
func TestASnapshotCarriesTheCaseGUIAndNotTheSimulatedUser(t *testing.T) {
	gui := &cenv.UniverseGUI{Hide: []string{"planning-deck", "runs", "dev-console"}, First: []string{"archifactory"}}
	c := &api.Case{Name: "archi", Title: "ArchiFactory", Dir: t.TempDir(),
		Universe: api.CaseUniverse{Sidecars: []string{"ktw-x"}, DisplayName: "ArchiFactory", GUI: gui}}
	e := NewEngine(c, t.TempDir(), "", io.Discard)

	// The case as install leaves it: a member running a copy in bin/, the
	// universe's tasks (its own and the simulated user's), a project.
	bin := filepath.Join(e.RT.Bin(), "ktw-x.exe")
	write(t, bin, "the very bytes")
	if err := catalog.SaveManifest(e.RT.Universe(), &catalog.Manifest{Name: "scn-archi", DisplayName: "Scenario archi",
		Sidecars: []orchapi.SidecarSpec{{Name: "ktw-x", Transport: "nats", Binary: bin}}}); err != nil {
		t.Fatal(err)
	}
	tasks := filepath.Join(application.EnvOf(e.RT.Universe()).HomeDir(), "tasks")
	write(t, filepath.Join(tasks, SimUserTask+".md"), "---\nname: scenario-user\n---\n")
	write(t, filepath.Join(tasks, "own.md"), "---\nname: own\n---\n")
	write(t, filepath.Join(e.RT.Project(), "README.md"), "# Bienvenue\n")

	// A catalog publishing the member's very bytes.
	market := t.TempDir()
	catalogPath := filepath.Join(market, "katyoti-marketplace.yaml")
	sum := sha256.Sum256([]byte("the very bytes"))
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: []mktapi.Item{{
		Name: "ktw-x", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0",
			Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: hex.EncodeToString(sum[:])}}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	write(t, catalogPath, string(data))

	if _, err := e.Snapshot(SnapshotOptions{Version: "v1", Marketplace: catalogPath}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(market, "solutions", "archi", "v1", "solution.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var sol struct {
		Universes []mktapi.SolutionUniverse `yaml:"universes"`
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &sol); err != nil {
		t.Fatal(err)
	}
	if len(sol.Universes) != 1 || !reflect.DeepEqual(sol.Universes[0].GUI, gui) {
		t.Fatalf("the solution's universes = %+v, want one with the case's gui %+v", sol.Universes, gui)
	}
	if sol.Universes[0].DisplayName != "ArchiFactory" {
		t.Errorf("display name = %q, want ArchiFactory", sol.Universes[0].DisplayName)
	}

	files := seedFiles(t, filepath.Join(market, filepath.FromSlash(index.SeedAssetPath("v1", "archi-seed"))))
	if !files["universe/tasks/own.md"] {
		t.Errorf("the universe's own task is not in the seed: %v", files)
	}
	for f := range files {
		if strings.Contains(f, SimUserTask) {
			t.Errorf("the seed publishes the simulated user's task: %s", f)
		}
	}
	// The case itself is untouched: its sandbox still runs the simulated user.
	if _, err := os.Stat(filepath.Join(tasks, SimUserTask+".md")); err != nil {
		t.Errorf("the case lost its simulated user's task: %v", err)
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedFiles lists a published seed's files by their slash path.
func seedFiles(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimPrefix(filepath.ToSlash(h.Name), "./")] = true
	}
}

// A snapshot taken from a case pins each member to the catalog version that
// publishes its very bytes (the case runs copies in its bin/), never the
// latest, and records the infra version published beside the catalog.
func TestASnapshotOfACasePinsItsMembersByContentAndRecordsTheInfra(t *testing.T) {
	c := &api.Case{Name: "archi", Title: "ArchiFactory", Dir: t.TempDir(),
		Universe: api.CaseUniverse{Sidecars: []string{"ktw-x"}}}
	e := NewEngine(c, t.TempDir(), "", io.Discard)
	bin := filepath.Join(e.RT.Bin(), "ktw-x.exe")
	write(t, bin, "the very bytes")
	if err := catalog.SaveManifest(e.RT.Universe(), &catalog.Manifest{Name: "scn-archi",
		Sidecars: []orchapi.SidecarSpec{{Name: "ktw-x", Transport: "nats", Binary: bin}}}); err != nil {
		t.Fatal(err)
	}
	market := t.TempDir()
	catalogPath := filepath.Join(market, "katyoti-marketplace.yaml")
	sum := sha256.Sum256([]byte("the very bytes"))
	asset := func(s string) []mktapi.Asset {
		return []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: s}}
	}
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: []mktapi.Item{{
		Name: "ktw-x", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{
			{Version: "2.0.0", Assets: asset("00")},
			{Version: "1.4.0", Assets: asset(hex.EncodeToString(sum[:]))},
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	write(t, catalogPath, string(data))
	write(t, filepath.Join(market, "infra", "channel.yaml"), "version: v7\n")

	r, err := e.Snapshot(SnapshotOptions{Version: "v1", Marketplace: catalogPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Members) != 1 || r.Members[0].Version != "1.4.0" {
		t.Fatalf("members = %+v, want ktw-x@1.4.0 (the version publishing the case's bytes)", r.Members)
	}
	if r.Infra != "v7" {
		t.Fatalf("infra = %q, want v7", r.Infra)
	}
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var cat mktapi.Catalog
	if err := config.DecodeBytes(raw, marshal.YAML, &cat); err != nil {
		t.Fatal(err)
	}
	for _, it := range cat.Items {
		if it.Name == "archi" {
			if len(it.Versions) != 1 || it.Versions[0].Infra != "v7" {
				t.Fatalf("the release does not record its infra: %+v", it.Versions)
			}
			return
		}
	}
	t.Fatalf("no release archi in the catalog: %+v", cat.Items)
}
