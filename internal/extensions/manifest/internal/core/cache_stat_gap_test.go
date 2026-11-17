// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// cache_stat_gap_test.go — the stat key and its one gap (blind audit M2).
//
// Tier-1 identity is the stat on one host: size and mtime of the declared
// binary. The record used to compare mtime truncated to the second, so a
// same-size rebuild inside one second read FRESH (`stale=false why=""`). The
// filesystem offers more than seconds nearly everywhere (100 ns on NTFS, ns on
// ext4/APFS), and the record keeps what it offers, so the compare is exact.
//
// The gap that remains is a stamp the filesystem, or the tool that set it,
// keeps to the whole second: two builds inside that tick share size and mtime.
// There the recorded provider digests are the tie-breaker, on a re-probe.

// TestCache_SameSizeRebuildWithinOneSecondIsStale is the audit's
// reproduction: two builds of one size, stamped 20 ms apart in the same
// second.
func TestCache_SameSizeRebuildWithinOneSecondIsStale(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := filepath.Join(filepath.Dir(cache.Root), "bin", "ktw-x.exe")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Both stamps in one second, 20 ms apart: what a rebuild right after a
	// probe looks like on any filesystem finer than a second.
	first := time.Date(2026, 9, 26, 12, 0, 0, 300_000_000, time.Local)
	if err := os.Chtimes(exe, first, first); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-x", makeManifest("ktw-x", "1", "h1"), exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("build-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(exe, first.Add(20*time.Millisecond), first.Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, meta, err := cache.Load("ktw-x", exe)
	if err != nil || meta == nil {
		t.Fatalf("load: meta=%v err=%v", meta, err)
	}
	stale, why := cache.IsStale(meta, SourceDigests{Binary: exe})
	if !stale {
		t.Fatalf("same size, same second, different content read FRESH: stale=%v why=%q", stale, why)
	}
	if !strings.Contains(why, "exe_mtime") {
		t.Fatalf("reason %q does not name the mtime", why)
	}
}

// TestCache_WholeSecondStampIsUndecided_ProvidersBreakTheTie: a stamp with no
// sub-second part cannot tell two builds apart, and the stat says so instead
// of "fresh"; the recorded provider digests then decide against a re-probe.
func TestCache_WholeSecondStampIsUndecided_ProvidersBreakTheTie(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := filepath.Join(filepath.Dir(cache.Root), "bin", "ktw-y.exe")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	whole := time.Date(2026, 9, 26, 12, 0, 0, 0, time.Local) // a FAT stamp, or a tarball's
	if err := os.Chtimes(exe, whole, whole); err != nil {
		t.Fatal(err)
	}
	recorded := makeManifest("ktw-y", "1", "h1")
	recorded.Providers = []mbapi.ProviderInfo{
		{Name: "ktw-y", Type: "skill", Digest: "sha256:aaa"},
		{Name: "ktw-y", Type: "task", Digest: "sha256:bbb"},
		{Name: "ktw-y", Type: "document", Mutable: true},
	}
	if err := cache.Save("ktw-y", recorded, exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("build-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(exe, whole, whole); err != nil {
		t.Fatal(err)
	}
	_, meta, err := cache.Load("ktw-y", exe)
	if err != nil || meta == nil {
		t.Fatalf("load: meta=%v err=%v", meta, err)
	}
	live := SourceDigests{Binary: exe}
	if stale, why := cache.IsStale(meta, live); stale {
		t.Fatalf("the stat cannot see this rebuild; IsStale must not claim it does: %s", why)
	}
	if !cache.StatUndecided(meta, live) {
		t.Fatal("a whole-second stamp on a same-size binary must read UNDECIDED, not fresh")
	}

	// The tie-breaker: an immutable provider's digest moved.
	probed := makeManifest("ktw-y", "1", "h2")
	probed.Providers = []mbapi.ProviderInfo{
		{Name: "ktw-y", Type: "skill", Digest: "sha256:aaa"},
		{Name: "ktw-y", Type: "task", Digest: "sha256:ccc"},
		{Name: "ktw-y", Type: "document", Mutable: true},
	}
	moved, why := cache.ProvidersMoved(recorded.Providers, probed.Providers)
	if !moved {
		t.Fatal("a changed provider digest must break the tie as STALE")
	}
	if !strings.Contains(why, "ktw-y/task") || !strings.Contains(why, "sha256:bbb") || !strings.Contains(why, "sha256:ccc") {
		t.Fatalf("reason %q must name the provider and both digests", why)
	}
	// Same digests: the record stands; a mutable provider carries none and
	// never decides.
	if moved, why := cache.ProvidersMoved(recorded.Providers, recorded.Providers); moved {
		t.Fatalf("identical digests read as moved: %s", why)
	}
	// A provider that appeared or disappeared is a change of the source too;
	// a mutable one carries no digest and never decides.
	if moved, _ := cache.ProvidersMoved(recorded.Providers, recorded.Providers[1:]); !moved {
		t.Fatal("an immutable provider gone from the probe must read as moved")
	}
	if moved, why := cache.ProvidersMoved(recorded.Providers, recorded.Providers[:2]); moved {
		t.Fatalf("a mutable provider gone from the probe decided: %s", why)
	}

	// A stamp with a sub-second part decides alone: not undecided.
	fine := whole.Add(300 * time.Millisecond)
	if err := os.Chtimes(exe, fine, fine); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-y", recorded, exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	if _, meta, err = cache.Load("ktw-y", exe); err != nil || meta == nil {
		t.Fatalf("load: meta=%v err=%v", meta, err)
	}
	if cache.StatUndecided(meta, live) {
		t.Fatal("a sub-second stamp is decided by the stat alone")
	}
	// And a stat that already says stale is never undecided.
	if err := os.WriteFile(exe, []byte("build-three"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cache.StatUndecided(meta, live) {
		t.Fatal("a size change is decided: stale, not undecided")
	}
}

// TestCache_RecordKeepsTheSubSecondStamp: what Save records is what IsStale
// compares, through the YAML round trip, at the filesystem's resolution.
func TestCache_RecordKeepsTheSubSecondStamp(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := filepath.Join(filepath.Dir(cache.Root), "bin", "ktw-z.exe")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	if err := os.WriteFile(exe, []byte("build"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 26, 12, 0, 0, 123_456_700, time.Local)
	if err := os.Chtimes(exe, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-z", makeManifest("ktw-z", "1", "h"), exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	_, meta, err := cache.Load("ktw-z", exe)
	if err != nil || meta == nil {
		t.Fatalf("load: meta=%v err=%v", meta, err)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.ExeModTime.Equal(info.ModTime()) {
		t.Fatalf("recorded %s, the file says %s: the sub-second part was lost", meta.ExeModTime.Format(time.RFC3339Nano), info.ModTime().Format(time.RFC3339Nano))
	}
	if stale, why := cache.IsStale(meta, SourceDigests{Binary: exe}); stale {
		t.Fatalf("an untouched binary read stale: %s", why)
	}
}
