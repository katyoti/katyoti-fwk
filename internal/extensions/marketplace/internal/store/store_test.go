// SPDX-License-Identifier: Apache-2.0

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// tgz builds an in-memory gzip-compressed tar from name→content entries.
func tgz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExtractTarGz_RoundTrip: a nested archive extracts with its tree intact.
func TestExtractTarGz_RoundTrip(t *testing.T) {
	data := tgz(t, map[string]string{
		"ktw-host.yaml":     "name: demo\n",
		"skills/flow.md":    "---\nname: flow\n---\nbody",
		"tasks/run/TASK.md": "task",
	})
	dest := t.TempDir()
	if err := extractTarGz(data, dest); err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, rel := range []string{"ktw-host.yaml", "skills/flow.md", "tasks/run/TASK.md"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing extracted file %q: %v", rel, err)
		}
	}
	got, _ := os.ReadFile(filepath.Join(dest, "skills", "flow.md"))
	if string(got) != "---\nname: flow\n---\nbody" {
		t.Errorf("content mismatch: %q", got)
	}
}

// TestExtractTarGz_RejectsTraversal: a zip-slip entry escaping dest is refused
// and writes nothing outside dest.
func TestExtractTarGz_RejectsTraversal(t *testing.T) {
	data := tgz(t, map[string]string{"../evil.txt": "pwned"})
	dest := filepath.Join(t.TempDir(), "sub")
	if err := extractTarGz(data, dest); err == nil {
		t.Fatal("expected traversal rejection, got nil error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Fatal("traversal wrote a file outside dest")
	}
}

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// A sidecar whose asset carries a bundle is installed with the bundle unpacked
// beside the binary; a second install is a no-op; a missing bundle is filled
// in even when the binary is already placed; a corrupt bundle is refused.
func TestInstallSidecarUnpacksItsBundle(t *testing.T) {
	src := t.TempDir()
	bin := []byte("binary")
	bundle := tgz(t, map[string]string{"drawio/editor/index.html": "<html/>", "drawio/editor/js/PreConfig.js": "x"})
	binPath, bundlePath := filepath.Join(src, "ktw-display"), filepath.Join(src, "ktw-display-assets.tar.gz")
	if err := os.WriteFile(binPath, bin, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundlePath, bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	asset := mktapi.Asset{URL: binPath, Sha256: sha(bin), Bundle: &mktapi.AssetBundle{URL: bundlePath, Sha256: sha(bundle)}}

	dest := filepath.Join(t.TempDir(), "ktw-display", "1.0.0", "ktw-display")
	if err := installSidecarAt(t.Context(), dest, "ktw-display", "1.0.0", asset); err != nil {
		t.Fatal(err)
	}
	dir := BundleDir(dest, "ktw-display")
	if got, err := os.ReadFile(filepath.Join(dir, "drawio", "editor", "js", "PreConfig.js")); err != nil || string(got) != "x" {
		t.Fatalf("bundle not unpacked beside the binary: %v %q", err, got)
	}
	if err := installSidecarAt(t.Context(), dest, "ktw-display", "1.0.0", asset); err != nil {
		t.Fatalf("second install: %v", err)
	}

	// Binary present, bundle missing (installed before bundles existed): filled in.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := installSidecarAt(t.Context(), dest, "ktw-display", "1.0.0", asset); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drawio", "editor", "index.html")); err != nil {
		t.Fatalf("missing bundle not filled in: %v", err)
	}

	bad := asset
	bad.Bundle = &mktapi.AssetBundle{URL: bundlePath, Sha256: sha([]byte("other"))}
	dest2 := filepath.Join(t.TempDir(), "ktw-display")
	if err := installSidecarAt(t.Context(), dest2, "ktw-display", "1.0.0", bad); err == nil {
		t.Fatal("a bundle whose sha256 does not match must be refused")
	}
}
