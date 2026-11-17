// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsStale_OverwritesDigestDrift: with the exe unchanged, a changed override
// digest must flag the cache stale. This is the staleness-bug fix — the exe stat
// alone can't see override edits.
func TestIsStale_OverwritesDigestDrift(t *testing.T) {
	cache := NewCache(t.TempDir())
	man := makeManifest("ktw-compose", "1", "h1")
	exeDir := t.TempDir()
	exePath := filepath.Join(exeDir, "ktw-compose.exe")
	if err := os.WriteFile(exePath, []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-compose", man, exePath, SourceDigests{Overwrites: "digest-A"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, meta, err := cache.Load("ktw-compose", exePath)
	if err != nil || meta == nil {
		t.Fatalf("Load: %v meta=%v", err, meta)
	}

	// Same digest → fresh.
	if stale, reason := cache.IsStale(meta, SourceDigests{Binary: exePath, Overwrites: "digest-A"}); stale {
		t.Errorf("unchanged digest must be fresh; got stale: %s", reason)
	}
	// Drifted digest → stale, even though the exe is byte-identical.
	if stale, _ := cache.IsStale(meta, SourceDigests{Binary: exePath, Overwrites: "digest-B"}); !stale {
		t.Error("changed override digest must flag stale")
	}
	// Gate flip on→off (live "") vs stored non-empty → stale.
	if stale, _ := cache.IsStale(meta, SourceDigests{Binary: exePath}); !stale {
		t.Error("override removed (empty digest) must flag stale")
	}
}

// TestDigestDir: stable for identical trees, changes on content edit, "" for
// missing/empty dirs.
func TestDigestDir(t *testing.T) {
	if d := DigestDir(""); d != "" {
		t.Errorf("empty path must digest to \"\"; got %q", d)
	}
	if d := DigestDir(filepath.Join(t.TempDir(), "nope")); d != "" {
		t.Errorf("missing dir must digest to \"\"; got %q", d)
	}

	dir := t.TempDir()
	if d := DigestDir(dir); d != "" {
		t.Errorf("empty dir must digest to \"\"; got %q", d)
	}

	sub := filepath.Join(dir, "embed", "skills")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(sub, "greet.md")
	if err := os.WriteFile(f, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	d1 := DigestDir(dir)
	if d1 == "" {
		t.Fatal("populated dir must produce a non-empty digest")
	}
	if DigestDir(dir) != d1 {
		t.Error("digest must be stable for an unchanged tree")
	}

	// A size change must move the digest.
	if err := os.WriteFile(f, []byte("v2-longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if DigestDir(dir) == d1 {
		t.Error("content (size) change must change the digest")
	}
}
