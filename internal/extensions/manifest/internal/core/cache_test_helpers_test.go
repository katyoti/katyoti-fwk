// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// makeManifest builds a minimal valid Manifest for tests. Provider +
// Version + Hash are populated (Cache.Save requires the same trio).
// Contains exactly one skill Contribution by default; tests that need
// a richer body overwrite `.Contributions` after the call.
//
// Migrated from the deleted `inventory_test.go` (2026-06-05 Cache
// redesign) so catalog tests keep their fixture surface unchanged.
func makeManifest(provider, version, hash string) *mbapi.Manifest {
	return &mbapi.Manifest{
		Provider: provider,
		Version:  version,
		Hash:     hash,
		Contributions: []contribapi.Contribution{
			{Provider: provider, Type: "skill", ID: "demo"},
		},
	}
}

// seedCache writes a manifest into the cache + creates a placeholder
// "binary" file so Cache.Save's stat call succeeds. Returns the exe
// path so tests can mutate it (touch, delete) to exercise staleness
// detection.
//
// The placeholder is a 1-byte file under the test's tempdir — never
// executed, just stat-ed by Save / IsStale.
func seedCache(t *testing.T, cache *Cache, manifest *mbapi.Manifest) string {
	t.Helper()
	exePath := testExePath(cache, manifest.Provider)
	_ = os.MkdirAll(filepath.Dir(exePath), 0o755)
	if err := os.WriteFile(exePath, []byte{0}, 0o644); err != nil {
		t.Fatalf("seedCache: write fake binary: %v", err)
	}
	if err := cache.Save(manifest.Provider, manifest, exePath, SourceDigests{}); err != nil {
		t.Fatalf("seedCache: Save %s: %v", manifest.Provider, err)
	}
	return exePath
}

// SeedCacheSave is the drop-in replacement for the deleted
// `Inventory.Save(m)` call shape used by the existing catalog tests
// (`if err := inv.Save(m); err != nil { … }`). It mirrors `seedCache`
// — creates a placeholder binary + calls `Cache.Save` — but returns
// only the error so the test bodies don't need to be restructured to
// consume the exe path. Use `seedCache` when the test needs to mutate
// the exe path afterwards (staleness tests).
func SeedCacheSave(t *testing.T, cache *Cache, manifest *mbapi.Manifest) error {
	t.Helper()
	exePath := testExePath(cache, manifest.Provider)
	_ = os.MkdirAll(filepath.Dir(exePath), 0o755)
	if err := os.WriteFile(exePath, []byte{0}, 0o644); err != nil {
		return err
	}
	return cache.Save(manifest.Provider, manifest, exePath, SourceDigests{})
}

// testExePath is where the seed helpers put the placeholder binary of a
// member: beside the cache root, one per name, deterministic so a test can
// address the record it seeded — a record is keyed by (name, binary) — with
// MembersOf and never has to carry the path around.
func testExePath(cache *Cache, name string) string {
	return filepath.Join(filepath.Dir(cache.Root), "bin", name+".exe")
}

// MembersOf is the catalog membership over the seed helpers: each name with
// the binary seedCache / SeedCacheSave stat for it. Exported so the external
// test packages of this directory can use it too.
func MembersOf(cache *Cache, names ...string) []Member {
	out := make([]Member, 0, len(names))
	for _, n := range names {
		out = append(out, Member{Name: n, Binary: testExePath(cache, n)})
	}
	return out
}
