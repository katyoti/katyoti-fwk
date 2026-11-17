// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// cache_content_test.go — a served folder is a source the executable cannot
// show.
//
// A generic host running someone's folder has the SAME binary whatever that
// folder holds. The exe stat is therefore blind to every edit in it, so an
// author changes a skill and the aggregator never asks again. The content
// digest is what makes that visible, and it is computed by the aggregator from
// the root the membership declares — never by asking the member, which would be
// contacting a contributor just to find out whether anything changed.

// TestCache_ServedFolderEditInvalidates is the case that failed: same binary,
// changed folder.
func TestCache_ServedFolderEditInvalidates(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	// A stand-in for the host executable. It is never touched below — that is
	// the point.
	exePath := filepath.Join(dir, "ktw-host.exe")
	if err := os.WriteFile(exePath, []byte("the host binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The folder it serves.
	content := filepath.Join(dir, "served")
	if err := os.MkdirAll(filepath.Join(content, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(content, "skills", "SKILL.md")
	if err := os.WriteFile(skill, []byte("version one"), 0o644); err != nil {
		t.Fatal(err)
	}

	man := &mbapi.Manifest{Provider: "my-pack", Version: "1.0.0"}
	ReHash(man)
	digests := SourceDigests{Content: DigestDir(content)}
	if digests.Content == "" {
		t.Fatal("digest of a non-empty folder is empty")
	}
	if err := cache.Save("my-pack", man, exePath, digests); err != nil {
		t.Fatal(err)
	}
	_, meta, err := cache.Load("my-pack", exePath)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing changed: fresh.
	if stale, reason := cache.IsStale(meta, SourceDigests{Binary: exePath, Content: DigestDir(content)}); stale {
		t.Fatalf("unchanged folder reported stale: %s", reason)
	}

	// An author EDITS a served file. The binary is untouched.
	if err := os.WriteFile(skill, []byte("version two, rather longer than before"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, reason := cache.IsStale(meta, SourceDigests{Binary: exePath, Content: DigestDir(content)})
	if !stale {
		t.Fatal("an edit inside the served folder did not invalidate the cached manifest — " +
			"the exe stat cannot see it, so nothing would ever re-probe")
	}
	if reason == "" {
		t.Error("staleness reported with no reason")
	}
}

// TestCache_ServedFolderAdditionAndRemovalInvalidate covers the other two
// shapes of change, so fixing edits cannot regress them.
func TestCache_ServedFolderAdditionAndRemovalInvalidate(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	exePath := filepath.Join(dir, "ktw-host.exe")
	if err := os.WriteFile(exePath, []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(dir, "served")
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(content, "a.md")
	if err := os.WriteFile(first, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}

	man := &mbapi.Manifest{Provider: "my-pack"}
	ReHash(man)
	if err := cache.Save("my-pack", man, exePath, SourceDigests{Content: DigestDir(content)}); err != nil {
		t.Fatal(err)
	}
	_, meta, _ := cache.Load("my-pack", exePath)

	if err := os.WriteFile(filepath.Join(content, "b.md"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if stale, _ := cache.IsStale(meta, SourceDigests{Binary: exePath, Content: DigestDir(content)}); !stale {
		t.Error("an added file did not invalidate")
	}

	if err := os.Remove(filepath.Join(content, "b.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	if stale, _ := cache.IsStale(meta, SourceDigests{Binary: exePath, Content: DigestDir(content)}); !stale {
		t.Error("a removed file did not invalidate")
	}
}

// TestCache_NoServedFolderIsUnaffected asserts a compiled sidecar — which
// carries its content rather than reading it — is untouched by any of this.
func TestCache_NoServedFolderIsUnaffected(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	exePath := filepath.Join(dir, "ktw-docs.exe")
	if err := os.WriteFile(exePath, []byte("compiled sidecar"), 0o644); err != nil {
		t.Fatal(err)
	}
	man := &mbapi.Manifest{Provider: "ktw-docs"}
	ReHash(man)
	if err := cache.Save("ktw-docs", man, exePath, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	_, meta, _ := cache.Load("ktw-docs", exePath)
	if stale, reason := cache.IsStale(meta, SourceDigests{Binary: exePath}); stale {
		t.Errorf("a sidecar serving no folder reported stale: %s", reason)
	}
}
