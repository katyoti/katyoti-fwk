// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
)

// writeCatalog writes a catalog under a temporary marketplace folder.
func writeCatalog(t *testing.T, cat mktapi.Catalog) string {
	t.Helper()
	dir := t.TempDir()
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "katyoti-marketplace.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// PROMOTION PUBLISHES THE CANDIDATE'S ARTIFACTS ON STABLE - a
// candidate is marked canary; promoting it adds the official version on
// stable, first, with the candidate's own assets (same URLs, same sha256) and
// writes the solution's pin under the official version; the candidate stays on
// canary; a version already published is never replaced.
func TestPromotionPublishesTheCandidatesArtifactsOnStable(t *testing.T) {
	asset := mktapi.Asset{OS: "windows", Arch: "amd64", URL: "2026.10-rc2/windows-amd64/ktw-agents.exe", Sha256: "abc123"}
	dir := writeCatalog(t, mktapi.Catalog{Marketplace: mktapi.CatalogMeta{Name: "archifactory"}, Items: []mktapi.Item{
		{Name: "ktw-agents", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "2026.10-rc2", Assets: []mktapi.Asset{asset}}, {Version: "2026.09"}}},
		{Name: "archifactory", Kind: mktapi.KindSolution, Versions: []mktapi.ItemVersion{{Version: "2026.10-rc2"}, {Version: "2026.09"}}},
	}})
	s := New()
	if _, err := s.MarkChannel(dir, "2026.10-rc2", mktapi.ChannelCanary, ""); err != nil {
		t.Fatal(err)
	}
	res, err := s.Promote(dir, "2026.10-rc2", "2026.10", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Errorf("promoted %v; want both items", res.Items)
	}
	cat, err := loadCatalogFile(filepath.Join(dir, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	agents := cat.Items[0].Versions
	if agents[0].Version != "2026.10" || agents[0].Channel != "" || len(agents[0].Assets) != 1 || agents[0].Assets[0] != asset {
		t.Errorf("the official version: %+v; want 2026.10 on stable, first, with the candidate's own asset %+v", agents[0], asset)
	}
	if agents[1].Version != "2026.10-rc2" || agents[1].Channel != mktapi.ChannelCanary {
		t.Errorf("the candidate after promotion: %+v; want it still on canary", agents[1])
	}
	if _, err := os.Stat(filepath.Join(dir, "solutions", "archifactory", "2026.10", "solution.yaml")); err != nil {
		t.Errorf("the solution's pin under its official version: %v", err)
	}
	if _, err := s.Promote(dir, "2026.10-rc2", "2026.10", ""); err == nil {
		t.Error("a promotion replaced a published version")
	}
	if _, err := s.MarkChannel(dir, "2026.10", "beta", ""); err == nil {
		t.Error("an unknown channel was accepted")
	}
}

// A CANARY CUT NEVER PUSHES A STABLE RELEASE OUT - its
// version is merged with no retention, marked canary, then the catalog is
// pruned: every item keeps its `keep` newest stable versions beside the
// candidate; what retention drops loses only the files no kept version uses.
func TestACanaryCutNeverPushesAStableReleaseOut(t *testing.T) {
	file := func(v string) mktapi.Asset {
		return mktapi.Asset{OS: "windows", Arch: "amd64", URL: v + "/windows-amd64/ktw-agents.exe", Sha256: v}
	}
	shared := mktapi.Asset{OS: "windows", Arch: "amd64", URL: "2026.07-rc1/windows-amd64/ktw-agents.exe", Sha256: "rc"}
	versions := []mktapi.ItemVersion{ // newest first, the cut's version merged with no retention
		{Version: "2026.11-rc1", Assets: []mktapi.Asset{file("2026.11-rc1")}},
		{Version: "2026.10", Assets: []mktapi.Asset{file("2026.10")}},
		{Version: "2026.09", Assets: []mktapi.Asset{file("2026.09")}},
		{Version: "2026.08", Assets: []mktapi.Asset{file("2026.08")}},
		{Version: "2026.07", Assets: []mktapi.Asset{shared}}, // promoted from its candidate
		{Version: "2026.07-rc1", Channel: mktapi.ChannelCanary, Assets: []mktapi.Asset{shared}},
	}
	dir := writeCatalog(t, mktapi.Catalog{Marketplace: mktapi.CatalogMeta{Name: "archifactory"}, Items: []mktapi.Item{
		{Name: "ktw-agents", Kind: mktapi.KindSidecar, Versions: versions},
	}})
	for _, v := range versions {
		p := filepath.Join(dir, filepath.FromSlash(v.Assets[0].URL))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v.Version), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := New()
	if _, err := s.MarkChannel(dir, "2026.11-rc1", mktapi.ChannelCanary, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(dir, 3, ""); err != nil {
		t.Fatal(err)
	}
	cat, err := loadCatalogFile(filepath.Join(dir, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, v := range cat.Items[0].Versions {
		kept = append(kept, v.Version)
	}
	if want := "2026.11-rc1 2026.10 2026.09 2026.08 2026.07-rc1"; strings.Join(kept, " ") != want {
		t.Errorf("kept %v; want %s - three stable releases beside the candidates", kept, want)
	}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}
	if !exists(shared.URL) {
		t.Error("2026.07's file was deleted though its candidate 2026.07-rc1 is kept")
	}
	for _, v := range []string{"2026.11-rc1", "2026.10", "2026.08"} {
		if !exists(file(v).URL) {
			t.Errorf("%s's file was deleted", v)
		}
	}
	// One more stable release: the oldest goes, its own file with it.
	cat.Items[0].Versions = append([]mktapi.ItemVersion{{Version: "2026.12", Assets: []mktapi.Asset{file("2026.12")}}}, cat.Items[0].Versions...)
	data, err := index.MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "katyoti-marketplace.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(dir, 3, ""); err != nil {
		t.Fatal(err)
	}
	if exists(file("2026.08").URL) {
		t.Error("the stable release past keep kept its file")
	}
}
