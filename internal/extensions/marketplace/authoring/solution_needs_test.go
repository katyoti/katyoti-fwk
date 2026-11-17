// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A solution's version requires what its two members' catalog versions
// require, summed: a program both need once, at the higher version, required
// because one requires it; each key and login once.
func TestAddSolution_SumsItsMembersNeeds(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "universe")
	market := filepath.Join(dir, "market")
	catalogPath := filepath.Join(market, "katyoti-marketplace.yaml")
	var specs []orchapi.SidecarSpec
	var items []mktapi.Item
	member := func(name string, req mktapi.Requires) {
		bin := filepath.Join(dir, "bin", name+".exe")
		if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte("bytes of "+name), 0o755); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte("bytes of " + name))
		specs = append(specs, orchapi.SidecarSpec{Name: name, Transport: "nats", Binary: bin})
		items = append(items, mktapi.Item{Name: name, Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{
			Version: "1.0.0", Requires: req,
			Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: name, Sha256: hex.EncodeToString(sum[:])}},
		}}})
	}
	member("ktw-docs", mktapi.NeedsOf("ktw-docs",
		[]publish.ToolRequirement{{Name: "Pandoc", Version: "3.1"}},
		[]publish.APIKeyRequirement{{EnvName: "KROKI_TOKEN"}},
		[]publish.AuthRequirement{{Name: "google"}}))
	member("ktw-kb", mktapi.NeedsOf("ktw-kb",
		[]publish.ToolRequirement{{Name: "Pandoc", Required: true, Version: "3.6"}, {Name: "ffmpeg"}},
		[]publish.APIKeyRequirement{{EnvName: "KROKI_TOKEN", Required: true}},
		[]publish.AuthRequirement{{Name: "google"}}))
	if err := catalog.SaveManifest(home, &catalog.Manifest{Name: "u", Sidecars: specs}); err != nil {
		t.Fatal(err)
	}
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(market, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := New().AddSolution(home, "v1", "demo", catalogPath, "", SolutionSeed{}, SolutionSpec{}); err != nil {
		t.Fatal(err)
	}
	cat, err := loadCatalogFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var req *mktapi.Requires
	for i := range cat.Items {
		if cat.Items[i].Name == "demo" {
			req = &cat.Items[i].Versions[0].Requires
		}
	}
	if req == nil {
		t.Fatal("the solution is not in the catalog")
	}
	if len(req.Tools) != 2 || req.Tools[0].Name != "ffmpeg" || req.Tools[1].Name != "Pandoc" ||
		!req.Tools[1].Required || req.Tools[1].Version != "3.6" || len(req.Tools[1].NeededBy) != 2 {
		t.Errorf("tools = %+v, want ffmpeg and Pandoc once (required, 3.6, both members)", req.Tools)
	}
	if len(req.APIKeys) != 1 || !req.APIKeys[0].Required || len(req.APIKeys[0].NeededBy) != 2 {
		t.Errorf("keys = %+v, want KROKI_TOKEN once, required", req.APIKeys)
	}
	if len(req.Auth) != 1 || req.Auth[0].Name != "google" {
		t.Errorf("logins = %+v, want google once", req.Auth)
	}
}

// A content folder's programs, keys and logins (its ktw-host.yaml `config:`)
// are its catalog item's requires, as a compiled sidecar's describe gives them.
func TestBuildContent_CarriesTheFolderNeeds(t *testing.T) {
	folder := seedContentFolder(t, `name: acme-playbooks
config:
  tools:
    - name: Pandoc
      commands: [pandoc]
      version: "3.1"
      recipes:
        - {platform: windows, kind: winget, package: JohnMacFarlane.Pandoc}
  apiKeys:
    - {envName: ACME_KEY, required: true}
  auth:
    - {name: acme, loginHint: acme login}
`)
	out := t.TempDir()
	if _, err := New().BuildContent(folder, "v1.0.0", out, 0, "", "", ""); err != nil {
		t.Fatalf("BuildContent: %v", err)
	}
	cat, err := loadCatalogFile(filepath.Join(out, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	req := cat.Items[0].Versions[0].Requires
	if len(req.Tools) != 1 || req.Tools[0].Name != "Pandoc" || !req.Tools[0].Required || req.Tools[0].Version != "3.1" ||
		len(req.Tools[0].Platforms) != 1 || req.Tools[0].NeededBy[0] != "acme-playbooks" {
		t.Errorf("tools = %+v", req.Tools)
	}
	if len(req.APIKeys) != 1 || req.APIKeys[0].Name != "ACME_KEY" || !req.APIKeys[0].Required {
		t.Errorf("keys = %+v", req.APIKeys)
	}
	if len(req.Auth) != 1 || req.Auth[0].LoginHint != "acme login" {
		t.Errorf("logins = %+v", req.Auth)
	}
}
