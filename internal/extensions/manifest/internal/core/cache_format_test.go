// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCache_RecordWithoutFormatIsStale: a tier-1 record written before the
// layout carried include_system (no `format` key) must re-probe, never read
// fresh, or a member saved with the system set would serve a second copy
// after an upgrade (re-audit N20, 2026-09-25).
func TestCache_RecordWithoutFormatIsStale(t *testing.T) {
	root := t.TempDir()
	cache := NewCache(root)
	exe := filepath.Join(root, "ktw-x.exe")
	if err := os.WriteFile(exe, []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := cache.RecordDir("ktw-x", exe)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := strings.Join([]string{
		"exe_path: " + filepath.ToSlash(exe),
		"exe_size: 3",
		"exe_mtime: 2026-01-01T00:00:00Z",
		`manifest_hash: ""`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"provider":"ktw-x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, meta, err := cache.Load("ktw-x", exe)
	if err != nil || meta == nil {
		t.Fatalf("load: meta=%v err=%v", meta, err)
	}
	stale, why := cache.IsStale(meta, SourceDigests{Binary: exe, IncludeSystem: false})
	if !stale {
		t.Fatalf("a record without a format key read FRESH; want stale (re-probe)")
	}
	if !strings.Contains(why, "predates") {
		t.Fatalf("reason %q does not name the format gap", why)
	}
}
