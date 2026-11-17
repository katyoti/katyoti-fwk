// SPDX-License-Identifier: Apache-2.0

package index

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// TestAssembleAndRoundTrip proves the producer output (assembleItem +
// MarshalCatalog) round-trips through the consumer parse path
// (config.DecodeBytes → api.Catalog) — i.e. a built catalog is readable by the
// same schema katyoti's search/info/install consume.
func TestAssembleAndRoundTrip(t *testing.T) {
	desc := appcore.SidecarDescription{
		Name:        "ktw-example",
		DisplayName: "LeanIX sidecar",
		Icon:        "🏛️",
		Description: "LeanIX factsheet queries",
		Version:     "1.4.2",
	}
	item := assembleItem(desc, []mktapi.Asset{{
		OS: "windows", Arch: "amd64",
		URL: "https://mkt.example/dl/ktw-example.exe", Sha256: "deadbeef",
	}})

	if item.Name != "ktw-example" || item.Kind != mktapi.KindSidecar {
		t.Fatalf("bad item identity: %+v", item)
	}
	if len(item.Versions) != 1 || item.Versions[0].Version != "1.4.2" {
		t.Fatalf("bad version: %+v", item.Versions)
	}
	a := item.Versions[0].Assets[0]
	if a.URL != "https://mkt.example/dl/ktw-example.exe" {
		t.Fatalf("bad asset URL: %q", a.URL)
	}
	if a.Sha256 != "deadbeef" {
		t.Fatalf("bad sha256: %q", a.Sha256)
	}

	cat := mktapi.Catalog{
		APIVersion:  1,
		Marketplace: mktapi.CatalogMeta{Name: "demo", DisplayName: "Demo"},
		Items:       []mktapi.Item{item},
	}
	out, err := MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}

	// Consume it back through the same pipeline katyoti uses.
	var parsed mktapi.Catalog
	if err := config.DecodeBytes(out, marshal.YAML, &parsed); err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if len(parsed.Items) != 1 || parsed.Items[0].Name != "ktw-example" {
		t.Fatalf("round-trip lost the item: %+v", parsed)
	}
	if parsed.Items[0].Versions[0].Assets[0].Sha256 != "deadbeef" {
		t.Fatal("round-trip lost the asset sha256")
	}
	if parsed.Items[0].Icon != "🏛️" {
		t.Fatalf("round-trip lost the icon: %q", parsed.Items[0].Icon)
	}
}

// TestDiscoverPlatforms proves a dist tree laid out as <root>/<goos>-<goarch>/
// is recognised as a platform matrix, and that unrelated folders and loose
// files are not mistaken for build targets.
func TestDiscoverPlatforms(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"darwin-arm64", "windows-amd64", "assets", "not-a-target"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "linux-amd64"), nil, 0o644); err != nil {
		t.Fatal(err) // a FILE named like a target must not count
	}

	got, err := DiscoverPlatforms(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 platforms, got %d: %+v", len(got), got)
	}
	byOS := map[string]Platform{}
	for _, p := range got {
		byOS[p.OS] = p
	}
	if p, ok := byOS["darwin"]; !ok || p.Arch != "arm64" || p.Sub != "darwin-arm64" {
		t.Fatalf("bad darwin platform: %+v", byOS)
	}
	if p, ok := byOS["windows"]; !ok || p.Arch != "amd64" {
		t.Fatalf("bad windows platform: %+v", byOS)
	}
}

// TestCollectAssetsMatrix proves one binary yields one asset per platform that
// ships it — with the .exe suffix applied per target OS, the sub-path in the
// URL, and a platform missing the binary skipped rather than emitted broken.
func TestCollectAssetsMatrix(t *testing.T) {
	root := t.TempDir()
	platforms := []Platform{
		{OS: "windows", Arch: "amd64", Dir: filepath.Join(root, "windows-amd64"), Sub: "windows-amd64"},
		{OS: "darwin", Arch: "arm64", Dir: filepath.Join(root, "darwin-arm64"), Sub: "darwin-arm64"},
		{OS: "linux", Arch: "amd64", Dir: filepath.Join(root, "linux-amd64"), Sub: "linux-amd64"},
	}
	for _, p := range platforms {
		if err := os.MkdirAll(p.Dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// linux deliberately has no ktw-example — that build failed.
	if err := os.WriteFile(filepath.Join(platforms[0].Dir, "ktw-example.exe"), []byte("win"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(platforms[1].Dir, "ktw-example"), []byte("mac"), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, staged := collectAssets(platforms, "ktw-example", "/srv/mkt", "ktw-example", "1.0.0", LayoutFlat)
	if len(assets) != 2 {
		t.Fatalf("want 2 assets, got %d: %+v", len(assets), assets)
	}
	if assets[0].OS != "windows" || assets[0].URL != "/srv/mkt/windows-amd64/ktw-example.exe" {
		t.Fatalf("bad windows asset: %+v", assets[0])
	}
	if assets[1].OS != "darwin" || assets[1].URL != "/srv/mkt/darwin-arm64/ktw-example" {
		t.Fatalf("bad darwin asset: %+v", assets[1])
	}
	if assets[0].Sha256 == assets[1].Sha256 {
		t.Fatal("distinct binaries must hash differently")
	}
	if len(staged) != 2 || staged[0].Rel != "windows-amd64/ktw-example.exe" {
		t.Fatalf("staging plan must mirror the asset paths: %+v", staged)
	}
}

// TestCollectAssetsVersionedLayout proves the published layout LEADS with the
// version and carries NO per-item folder: `<version>/<goos>-<goarch>/<file>`.
//
// The version must be in the path or publishing v2 would overwrite v1's bytes
// while v1's entry still records v1's sha256 — the download would succeed and
// the hash check would fail. The item folder is deliberately absent: the
// filename already identifies the item, and version-first makes retiring a whole
// release one `rm -rf <version>/`.
func TestCollectAssetsVersionedLayout(t *testing.T) {
	root := t.TempDir()
	p := Platform{OS: "darwin", Arch: "arm64", Dir: filepath.Join(root, "darwin-arm64"), Sub: "darwin-arm64"}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, "ktw-docs"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	assets, staged := collectAssets([]Platform{p}, "ktw-docs", "", "ktw-docs", "1.2.3", LayoutVersioned)
	want := "sidecars/1.2.3/darwin-arm64/ktw-docs"
	if len(assets) != 1 || assets[0].URL != want {
		t.Fatalf("want relative versioned URL under sidecars/ %q, got %+v", want, assets)
	}
	if len(staged) != 1 || staged[0].Rel != want {
		t.Fatalf("staging plan must match the URL exactly: %+v", staged)
	}
	if staged[0].Src != filepath.Join(p.Dir, "ktw-docs") {
		t.Fatalf("bad source path: %+v", staged[0])
	}
}

func catWith(item string, versions ...string) mktapi.Catalog {
	var vs []mktapi.ItemVersion
	for _, v := range versions {
		vs = append(vs, mktapi.ItemVersion{Version: v})
	}
	return mktapi.Catalog{
		APIVersion:  1,
		Marketplace: mktapi.CatalogMeta{Name: "demo"},
		Items:       []mktapi.Item{{Name: item, Kind: mktapi.KindSidecar, Versions: vs}},
	}
}

// TestMergeAccumulates proves a second publish ADDS a version instead of
// replacing the catalog — the whole point of the change.
func TestMergeAccumulates(t *testing.T) {
	merged, dropped, err := Merge(catWith("ktw-docs", "1.0.0"), catWith("ktw-docs", "2.0.0"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) != 0 {
		t.Fatalf("keep=0 means keep all, got drops: %+v", dropped)
	}
	got := merged.Items[0].Versions
	if len(got) != 2 || got[0].Version != "2.0.0" || got[1].Version != "1.0.0" {
		t.Fatalf("want newest-first [2.0.0 1.0.0], got %+v", got)
	}
}

// TestMergeRefusesRepublish proves a published version is immutable. Silently
// accepting it would swap the bytes under consumers that already pinned it.
func TestMergeRefusesRepublish(t *testing.T) {
	_, _, err := Merge(catWith("ktw-docs", "1.0.0"), catWith("ktw-docs", "1.0.0"), 0)
	if err == nil {
		t.Fatal("want an error republishing an existing version, got nil")
	}
}

// TestMergeRetention proves retention reports what it drops, so the caller can
// delete those asset folders in the same pass. A catalog entry kept without its
// assets fails at download; assets kept without an entry leak disk.
func TestMergeRetention(t *testing.T) {
	merged, dropped, err := Merge(catWith("ktw-docs", "2.0.0", "1.0.0"), catWith("ktw-docs", "3.0.0"), 2)
	if err != nil {
		t.Fatal(err)
	}
	got := merged.Items[0].Versions
	if len(got) != 2 || got[0].Version != "3.0.0" || got[1].Version != "2.0.0" {
		t.Fatalf("want the two newest, got %+v", got)
	}
	if len(dropped) != 1 || dropped[0].Version != "1.0.0" || dropped[0].Item != "ktw-docs" {
		t.Fatalf("want 1.0.0 reported as dropped, got %+v", dropped)
	}
}

// TestMergeNewItem proves an item that did not exist before is added whole.
func TestMergeNewItem(t *testing.T) {
	merged, _, err := Merge(catWith("ktw-docs", "1.0.0"), catWith("ktw-example", "1.0.0"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Items) != 2 {
		t.Fatalf("want both items, got %+v", merged.Items)
	}
}

// TestBuildRequiresHostPlatform proves indexing a foreign-only tree fails
// LOUDLY. Describe metadata can only be read from a binary this machine can
// run, and silently emitting an empty catalog would ship a signed, empty
// marketplace.
func TestBuildRequiresHostPlatform(t *testing.T) {
	_, err := Build([]Platform{{OS: "sunos", Arch: "sparc", Dir: t.TempDir()}}, "demo", "/srv/mkt", LayoutVersioned)
	if err == nil {
		t.Fatal("want an error when the host platform is absent, got nil")
	}
}

// TestBuildReportsSkips proves a binary that cannot be described is REPORTED
// rather than silently dropped. A skip that only shows up as a smaller item
// count ships a signed marketplace quietly missing a sidecar — this regressed
// once when an antivirus scan pushed a cold binary past the describe timeout.
func TestBuildReportsSkips(t *testing.T) {
	dir := t.TempDir()
	// Not a real executable, so describe fails however it is invoked.
	if err := os.WriteFile(filepath.Join(dir, "ktw-bogus"+exeSuffix(runtime.GOOS)), []byte("nope"), 0o755); err != nil {
		t.Fatal(err)
	}

	built, err := Build(
		[]Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: dir}}, "demo", "/srv/mkt", LayoutVersioned)
	if err != nil {
		t.Fatal(err)
	}
	cat, skips := built.Catalog, built.Skips
	if len(cat.Items) != 0 {
		t.Fatalf("want no items, got %+v", cat.Items)
	}
	if len(skips) != 1 || skips[0].Binary != "ktw-bogus"+exeSuffix(runtime.GOOS) {
		t.Fatalf("want the undescribable binary reported as a skip, got %+v", skips)
	}
	if skips[0].Reason == "" {
		t.Fatal("a skip must carry a reason")
	}
}

func platformWithMarker(t *testing.T, root, sub, marker string) Platform {
	t.Helper()
	dir := filepath.Join(root, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if marker != "" {
		if err := os.WriteFile(filepath.Join(dir, BuildMarkerFile), []byte(marker+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Platform{OS: "linux", Arch: sub, Dir: dir, Sub: sub}
}

// TestVerifyOneBuild proves a release cannot mix platforms from different
// builds. Publishing a rebuilt Windows binary alongside a stale macOS one
// freezes mismatched bytes under one immutable version — invisible afterwards,
// because the catalog records whatever it was handed.
func TestVerifyOneBuild(t *testing.T) {
	t.Run("all agree", func(t *testing.T) {
		root := t.TempDir()
		ps := []Platform{
			platformWithMarker(t, root, "a", "v1 abc123"),
			platformWithMarker(t, root, "b", "v1 abc123"),
		}
		if err := verifyOneBuild(ps); err != nil {
			t.Fatalf("matching markers must pass: %v", err)
		}
	})

	t.Run("disagree is refused", func(t *testing.T) {
		root := t.TempDir()
		ps := []Platform{
			platformWithMarker(t, root, "a", "v2 def456"),
			platformWithMarker(t, root, "b", "v1 abc123"),
		}
		if err := verifyOneBuild(ps); err == nil {
			t.Fatal("want an error when platforms came from different builds")
		}
	})

	t.Run("a stale platform missing its marker is refused", func(t *testing.T) {
		root := t.TempDir()
		ps := []Platform{
			platformWithMarker(t, root, "a", "v1 abc123"),
			platformWithMarker(t, root, "b", ""),
		}
		if err := verifyOneBuild(ps); err == nil {
			t.Fatal("want an error when only some platforms carry a marker")
		}
	})

	t.Run("a hand-assembled tree with no markers is allowed", func(t *testing.T) {
		root := t.TempDir()
		ps := []Platform{
			platformWithMarker(t, root, "a", ""),
			platformWithMarker(t, root, "b", ""),
		}
		if err := verifyOneBuild(ps); err != nil {
			t.Fatalf("no markers anywhere means nothing to compare: %v", err)
		}
	})
}

// TestMergeRetractsPublishedInfra proves an EXISTING catalog heals itself.
//
// Excluding infra from fresh builds does not retract what is already published:
// an accumulating catalog keeps every version it ever carried, so a tree
// published before the infra channel existed would keep serving katyoti /
// ktw-hub / ktw-gui forever -- and keep reproducing the ambiguity that made
// install-infra unresolvable when several marketplaces served the same name.
func TestMergeRetractsPublishedInfra(t *testing.T) {
	published := mktapi.Catalog{
		APIVersion:  1,
		Marketplace: mktapi.CatalogMeta{Name: "old"},
		Items: []mktapi.Item{
			{Name: "katyoti", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0"}}},
			{Name: "ktw-hub", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0"}}},
			{Name: "ktw-gui", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0"}}},
			{Name: "ktw-docs", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0"}}},
		},
	}
	merged, _, err := Merge(published, catWith("ktw-notify", "1.0.0"), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range merged.Items {
		if it.Name == "katyoti" || it.Name == "ktw-hub" || it.Name == "ktw-gui" {
			t.Errorf("%s survived the republish; infra must be retracted from the catalog", it.Name)
		}
	}
	// The ordinary sidecars must be untouched -- this retracts infra, it does
	// not rebuild the catalog from scratch.
	names := map[string]bool{}
	for _, it := range merged.Items {
		names[it.Name] = true
	}
	for _, want := range []string{"ktw-docs", "ktw-notify"} {
		if !names[want] {
			t.Errorf("%s was lost; only infra should be dropped", want)
		}
	}
}

// A companion bundle beside a binary is published with it: hashed, staged next
// to the binary, recorded on the platform's asset — and never taken for a
// sidecar of its own.
func TestCollectAssetsPublishesTheBundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ktw-display"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ktw-display"+mktapi.BundleSuffix), []byte("tgz"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets, staged := collectAssets([]Platform{{OS: "linux", Arch: "amd64", Dir: dir}}, "ktw-display", "", "ktw-display", "1.2.0", LayoutVersioned)
	if len(assets) != 1 || assets[0].Bundle == nil || assets[0].Bundle.Sha256 == "" {
		t.Fatalf("asset = %+v", assets)
	}
	if want := AssetPath("1.2.0", "linux", "amd64", "ktw-display-assets.tar.gz"); assets[0].Bundle.URL != want {
		t.Errorf("bundle URL = %q, want %q", assets[0].Bundle.URL, want)
	}
	if len(staged) != 2 {
		t.Errorf("staged = %+v, want the binary and its bundle", staged)
	}
	if got := assets[0].URLs(); len(got) != 2 {
		t.Errorf("URLs = %v, want the binary and its bundle (pruning removes both)", got)
	}
	if isSidecarBinary("ktw-display" + mktapi.BundleSuffix) {
		t.Error("a bundle is not a sidecar")
	}
}

// RETENTION KEEPS EACH CHANNEL AND THE FILES A KEPT VERSION USES
// - release candidates never push a stable release out (keep
// counts per channel), and a dropped candidate's files that its promoted
// release shares are not reported for deletion.
func TestRetentionKeepsEachChannelAndTheFilesAKeptVersionUses(t *testing.T) {
	shared := mktapi.Asset{OS: "windows", Arch: "amd64", URL: "2026.10-rc2/windows-amd64/ktw-docs.exe"}
	own := func(v string) []mktapi.Asset {
		return []mktapi.Asset{{OS: "windows", Arch: "amd64", URL: v + "/windows-amd64/ktw-docs.exe"}}
	}
	cat := mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "demo"}, Items: []mktapi.Item{{Name: "ktw-docs", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{
		{Version: "2026.10", Assets: []mktapi.Asset{shared}},
		{Version: "2026.10-rc2", Channel: mktapi.ChannelCanary, Assets: []mktapi.Asset{shared}},
	}}}}
	var all []Dropped
	for _, v := range []string{"2026.11-rc1", "2026.11-rc2", "2026.11-rc3"} {
		fresh := mktapi.Catalog{Items: []mktapi.Item{{Name: "ktw-docs", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: v, Channel: mktapi.ChannelCanary, Assets: own(v)}}}}}
		var dropped []Dropped
		var err error
		if cat, dropped, err = Merge(cat, fresh, 2); err != nil {
			t.Fatal(err)
		}
		all = append(all, dropped...)
	}
	var kept []string
	for _, v := range cat.Items[0].Versions {
		kept = append(kept, v.Version)
	}
	if len(kept) != 3 || kept[2] != "2026.10" {
		t.Errorf("kept %v; want the two newest candidates and the stable 2026.10", kept)
	}
	if len(all) != 2 || all[0].Version != "2026.10-rc2" || len(all[0].Assets) != 0 {
		t.Errorf("dropped %+v; want 2026.10-rc2 first with no file to delete (2026.10 uses it)", all)
	}
	if len(all) == 2 && (len(all[1].Assets) != 1 || all[1].Assets[0] != "2026.11-rc1/windows-amd64/ktw-docs.exe") {
		t.Errorf("dropped %+v; want 2026.11-rc1's own file to delete", all[1])
	}
}

// A member a later cut ships unchanged - the same files under the same
// version - changes nothing (: a proof channel holds two
// versions built from one tree); other bytes under that version are refused.
func TestMergeLeavesAnUnchangedRepublishAlone(t *testing.T) {
	withAsset := func(sum string) mktapi.Catalog {
		c := catWith("ktw-display", "1.0.0")
		c.Items[0].Versions[0].Assets = []mktapi.Asset{{OS: "windows", Arch: "amd64", URL: "1.0.0/windows-amd64/ktw-display.exe", Sha256: sum}}
		return c
	}
	merged, _, err := Merge(withAsset("abc"), withAsset("abc"), 0)
	if err != nil {
		t.Fatalf("the same files again: %v", err)
	}
	if v := merged.Items[0].Versions; len(v) != 1 || v[0].Assets[0].Sha256 != "abc" {
		t.Fatalf("the item after an unchanged republish: %+v", v)
	}
	if _, _, err := Merge(withAsset("abc"), withAsset("def"), 0); err == nil {
		t.Fatal("other bytes under a published version were accepted")
	}
}

// A pass adds a version when one of its items carries a version the catalog
// lacks; a pass made only of published versions adds none (:
// release-marketplace refuses it - published versions are immutable).
func TestAddsAVersionOnlyForAVersionNotPublished(t *testing.T) {
	published := catWith("ktw-display", "1.0.0")
	published.Items = append(published.Items, catWith("ktw-host", "1.0.0").Items...)

	if AddsAVersion(published, catWith("ktw-display", "1.0.0")) {
		t.Error("a published version again adds a version")
	}
	cut := catWith("ktw-display", "1.0.0")
	cut.Items = append(cut.Items, catWith("ktw-host", "1.1.0").Items...)
	if !AddsAVersion(published, cut) {
		t.Error("a cut with a new version beside an unchanged member adds none")
	}
	if !AddsAVersion(published, catWith("ktw-notify", "1.0.0")) {
		t.Error("a new item adds no version")
	}
}
