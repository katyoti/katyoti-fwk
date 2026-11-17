// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// seedContentFolder writes a minimal files-only sidecar: a manifest that names
// it, plus one artifact.
func seedContentFolder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "ktw-host.yaml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Content items were INSTALLABLE long before anything could publish one, so the
// kind existed for consumers and was unreachable for authors — which is exactly
// the contributor this feature is for: someone with files and no Go toolchain.
func TestBuildContent_PublishesAFolderAsACatalogItem(t *testing.T) {
	folder := seedContentFolder(t, "name: acme-playbooks\ndisplayName: ACME Playbooks\ndescription: team stuff\nicon: book\n")
	out := t.TempDir()

	res, err := New().BuildContent(folder, "v1.0.0", out, 0, "", "", "")
	if err != nil {
		t.Fatalf("BuildContent: %v", err)
	}
	if res.Name != "acme-playbooks" {
		t.Errorf("name = %q, want the one declared in ktw-host.yaml", res.Name)
	}
	// A files-only sidecar IS a sidecar, so it is served from sidecars/ beside
	// the compiled binaries of the same version — with no <goos>-<goarch>
	// level, because a folder of markdown is the same bytes everywhere.
	if !strings.HasPrefix(res.AssetPath, "sidecars/") {
		t.Errorf("asset path %q is not under sidecars/", res.AssetPath)
	}
	if strings.Contains(res.AssetPath, runtime.GOOS+"-") {
		t.Errorf("asset path %q is platform-pinned — a content folder is not", res.AssetPath)
	}
	if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(res.AssetPath))); err != nil {
		t.Errorf("catalog advertises an asset that is not on disk: %v", err)
	}

	cat, err := loadCatalogFile(filepath.Join(out, "katyoti-marketplace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(cat.Items))
	}
	it := cat.Items[0]
	if it.Kind != mktapi.KindContent {
		t.Errorf("kind = %q, want content", it.Kind)
	}
	if it.DisplayName != "ACME Playbooks" || it.Description != "team stuff" || it.Icon != "book" {
		t.Errorf("identity not carried from the manifest: %+v", it)
	}
	// ONE asset with no OS/Arch: a folder of markdown is the same bytes
	// everywhere, and the installer's picker treats empty as "matches anything".
	assets := it.Versions[0].Assets
	if len(assets) != 1 {
		t.Fatalf("want exactly 1 platform-agnostic asset, got %d", len(assets))
	}
	if assets[0].OS != "" || assets[0].Arch != "" {
		t.Errorf("asset is platform-pinned (%s/%s) — a content folder is not", assets[0].OS, assets[0].Arch)
	}
	if assets[0].Sha256 == "" {
		t.Error("asset has no sha256 — the install-time integrity check would be a no-op")
	}
}

// A folder without a manifest is not a content item. Saying so HERE beats
// publishing a catalog entry that nothing can run.
func TestBuildContent_RefusesAFolderWithNoManifest(t *testing.T) {
	_, err := New().BuildContent(seedContentFolder(t, ""), "v1.0.0", t.TempDir(), 0, "", "", "")
	if err == nil {
		t.Fatal("published a folder with no ktw-host.yaml")
	}
	if !strings.Contains(err.Error(), "ktw-host.yaml") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

// The name in ktw-host.yaml is what the HOST adopts as its runtime provider, so
// a manifest without one cannot produce a coherent item.
func TestBuildContent_RefusesAManifestWithNoName(t *testing.T) {
	_, err := New().BuildContent(seedContentFolder(t, "displayName: Nameless\n"), "v1.0.0", t.TempDir(), 0, "", "", "")
	if err == nil {
		t.Fatal("published a manifest with no name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error %q does not explain what is missing", err)
	}
}

// Publishing accumulates, and a published version is immutable — the same rule
// the binary catalog enforces: the same files again under it change nothing
// , other bytes are refused.
func TestBuildContent_RefusesRepublishingAVersion(t *testing.T) {
	folder := seedContentFolder(t, "name: acme\n")
	out := t.TempDir()
	if _, err := New().BuildContent(folder, "v1.0.0", out, 0, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := New().BuildContent(folder, "v2.0.0", out, 0, "", "", ""); err != nil {
		t.Fatalf("second version refused: %v", err)
	}
	if _, err := New().BuildContent(folder, "v1.0.0", out, 0, "", "", ""); err != nil {
		t.Fatalf("the same files again under v1.0.0 refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(folder, "skills", "a.md"), []byte("# a, changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New().BuildContent(folder, "v1.0.0", out, 0, "", "", ""); err == nil {
		t.Error("republished other bytes under an existing version — published versions are immutable")
	}
}
