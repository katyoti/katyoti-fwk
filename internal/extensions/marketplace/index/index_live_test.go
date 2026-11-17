// SPDX-License-Identifier: Apache-2.0

package index

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBuildLive builds a real catalog from the repo's ./bin by running
// `<binary> describe` on each built sidecar. Env-gated (execs binaries); writes
// the catalog to KATYOTI_CATALOG_OUT (or a temp file) so a katyoti smoke can
// consume it. Only binaries rebuilt with the `describe` verb are included.
//
// ./bin holds host binaries directly (no platform subfolders), so this is the
// single-platform case: one Platform rooted at binDir.
func TestBuildLive(t *testing.T) {
	if os.Getenv("KATYOTI_LIVE") == "" {
		t.Skip("set KATYOTI_LIVE=1 to build a catalog from ./bin")
	}
	binDir := os.Getenv("KATYOTI_BIN_DIR")
	if binDir == "" {
		binDir = filepath.Join("..", "..", "..", "..", "bin") // → repo/bin
	}
	if _, err := os.Stat(binDir); err != nil {
		t.Skipf("no bin dir %q: %v", binDir, err)
	}
	baseURL := os.Getenv("KATYOTI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://mkt.example/dl"
	}
	built, err := Build(
		[]Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH, Dir: binDir}}, "local", baseURL, LayoutFlat)
	if err != nil {
		t.Fatal(err)
	}
	cat := built.Catalog
	for _, s := range built.Skips {
		t.Logf("skipped %s: %s", s.Binary, s.Reason)
	}
	if len(cat.Items) == 0 {
		t.Fatal("no items built from ./bin (are any binaries rebuilt with `describe`?)")
	}
	out, err := MarshalCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	dest := os.Getenv("KATYOTI_CATALOG_OUT")
	if dest == "" {
		dest = filepath.Join(t.TempDir(), "katyoti-marketplace.yaml")
	}
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("built %d-item catalog → %s", len(cat.Items), dest)
}
