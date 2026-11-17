// SPDX-License-Identifier: Apache-2.0

package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// A published catalog names its files relative to itself — the binary AND the
// bundle beside it: both are fetched from the catalog's folder, wherever the
// installing process runs.
func TestASidecarBundleIsFetchedBesideTheCatalog(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	mkt := t.TempDir()
	rel := "sidecars/v1/" + runtime.GOOS + "-" + runtime.GOARCH + "/"
	if err := os.MkdirAll(filepath.Join(mkt, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mkt, rel, "ktw-display"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "index.html", Mode: 0o644, Size: 2})
	_, _ = tw.Write([]byte("ok"))
	_ = tw.Close()
	_ = gz.Close()
	if err := os.WriteFile(filepath.Join(mkt, rel, "ktw-display-assets.tar.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	it := mktapi.Item{Name: "ktw-display", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{
		Version: "v1",
		Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: rel + "ktw-display",
			Bundle: &mktapi.AssetBundle{URL: rel + "ktw-display-assets.tar.gz"}}},
	}}}
	inst, err := (&service{}).installSidecar(context.Background(), it, "m", filepath.Join(mkt, "catalog.yaml"), "")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(inst.BinaryPath), "ktw-display-assets", "index.html")); err != nil {
		t.Fatalf("the bundle was not unpacked beside the binary: %v", err)
	}
	if it.Versions[0].Assets[0].Bundle.URL != rel+"ktw-display-assets.tar.gz" {
		t.Fatalf("the catalog's own bundle URL was rewritten: %s", it.Versions[0].Assets[0].Bundle.URL)
	}
}
