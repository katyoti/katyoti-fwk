// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestCatalog_KindRegistryIsBuiltOncePerReload: the registry over every
// member's declarations is built once per reload and answered from memory
// after that; an OpenLayer with an empty layer used to rebuild it on every
// call (re-audit N14, 2026-09-26). KindsAt prefers the holder.
func TestCatalog_KindRegistryIsBuiltOncePerReload(t *testing.T) {
	root := t.TempDir()
	cache := NewCache(root)
	exe := filepath.Join(root, "ktw-x.exe")
	if err := os.WriteFile(exe, []byte("exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	var decl contribapi.Contribution
	decl.Provider, decl.Type, decl.ID = "ktw-x", contribapi.ResourceTypeContributionType, "widget"
	decl.SetData(json.RawMessage(`{"name":"widget","schema":"ktw-x:/data-type/widget","layers":{"data":{"mediaType":"application/json","required":true,"default":true}},"meta":{"budget":128},"validate":"warn"}`))
	var entry contribapi.Contribution
	entry.Provider, entry.Type, entry.ID = "ktw-x", "widget", "w1"
	entry.SetData(json.RawMessage(`{"name":"w1"}`))
	m := &mbapi.Manifest{Provider: "ktw-x", Version: "1", Contributions: []contribapi.Contribution{decl, entry}}
	ReHash(m)
	if err := cache.Save("ktw-x", m, exe, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	cat, err := NewCatalog(cache, []Member{{Name: "ktw-x", Binary: exe}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, _, err := cat.OpenLayer(ctx, "ktw-x:/widget/w1", ""); err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}
	if _, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}); err != nil {
		t.Fatal(err)
	}
	cat.mu.Lock()
	builds := cat.kindsBuilds
	cat.mu.Unlock()
	if builds != 1 {
		t.Fatalf("kind registry built %d time(s) for three opens and one KindsAt; want exactly 1", builds)
	}
	if err := cat.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cat.OpenLayer(ctx, "ktw-x:/widget/w1", ""); err != nil {
		t.Fatal(err)
	}
	cat.mu.Lock()
	builds = cat.kindsBuilds
	cat.mu.Unlock()
	if builds != 2 {
		t.Fatalf("after a reload the registry must be rebuilt exactly once more; builds=%d", builds)
	}
}
