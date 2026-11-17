// SPDX-License-Identifier: Apache-2.0

package core

// e18 — the cache tiers' missing proofs: cold start, concurrent refresh, stale
// record. The ownership matrix that cites them is
// modules/contributions/cache-layout.md §1.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestCache_ColdStart_EmptyRootIsAWorkingEmptyCatalog — T1 + T2 at first boot:
// a cache root that does not exist yet loads nothing (a clean miss, not an
// error), and a catalog over it with DECLARED members is empty but fully
// usable: every read answers, nothing panics, and the first Save + Reload
// brings a member in (the rebuild path the hub takes after its first probe).
func TestCache_ColdStart_EmptyRootIsAWorkingEmptyCatalog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "never-created")
	cache := NewCache(root)

	m, meta, err := cache.Load("ktw-docs", testExePath(cache, "ktw-docs"))
	if err != nil || m != nil || meta != nil {
		t.Fatalf("cold Load = (%v, %v, %v), want a clean miss", m, meta, err)
	}
	names, err := cache.ListNames()
	if err != nil || len(names) != 0 {
		t.Fatalf("cold ListNames = (%v, %v), want empty", names, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("a read created the cache root (%v) — only Save may write", err)
	}

	ctx := context.Background()
	cat, err := NewCatalog(cache, MembersOf(cache, "ktw-docs", "ktw-weather"))
	if err != nil {
		t.Fatalf("NewCatalog over a cold cache: %v", err)
	}
	if ms, err := cat.List(ctx); err != nil || len(ms) != 0 {
		t.Errorf("cold List = (%d, %v), want 0 manifests", len(ms), err)
	}
	if cs, err := cat.GetContributionsByType(ctx, "skill"); err != nil || len(cs) != 0 {
		t.Errorf("cold GetContributionsByType = (%d, %v), want 0", len(cs), err)
	}
	if _, err := cat.Get(ctx, "ktw-docs"); err == nil {
		t.Error("cold Get of a declared-but-unprobed member returned a manifest")
	}
	if _, err := cat.GetBody(ctx, "ktw-docs:/skill/demo"); err == nil {
		t.Error("cold GetBody answered for content nothing has cached")
	}

	// Rebuild: the first probe lands, a reload serves it.
	if err := SeedCacheSave(t, cache, makeManifest("ktw-docs", "1.0.0", "sha256:first")); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := cat.Reload(); err != nil {
		t.Fatalf("Reload after the first Save: %v", err)
	}
	got, err := cat.Get(ctx, "ktw-docs")
	if err != nil || got.Hash != "sha256:first" {
		t.Fatalf("after first Save + Reload, Get = (%v, %v), want the saved manifest", got, err)
	}
}

// TestCache_ConcurrentRefresh_NoTornRead — T1 under the hub's real concurrency:
// boot probes save many sidecars in parallel while the catalog reloads. Every
// SUCCESSFUL Load must return a manifest and metadata from the same Save (the
// pair is written in one directory and renamed as one); a Load of a sidecar
// whose folder is being swapped may miss or fail — and must then say so rather
// than serve half of one version and half of another. A sidecar nobody is
// saving must never be disturbed. Run with -race.
func TestCache_ConcurrentRefresh_NoTornRead(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := filepath.Join(t.TempDir(), "fake.exe")
	if err := os.WriteFile(exe, []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	stable := makeManifest("stable", "1.0.0", "sha256:stable")
	if err := cache.Save("stable", stable, exe, SourceDigests{}); err != nil {
		t.Fatalf("seed stable: %v", err)
	}

	const writers, rounds = 6, 25
	var wg sync.WaitGroup
	var stop atomic.Bool
	var saveErrs, readErrs, torn, stableFaults, reads atomic.Int64
	var firstTorn, firstSaveErr atomic.Value

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			name := fmt.Sprintf("sidecar-%d", w)
			for i := 0; i < rounds; i++ {
				m := makeManifest(name, "1.0.0", fmt.Sprintf("sha256:%s-%d", name, i))
				if err := cache.Save(name, m, exe, SourceDigests{}); err != nil {
					saveErrs.Add(1)
					firstSaveErr.CompareAndSwap(nil, err.Error())
				}
			}
		}(w)
	}

	var rwg sync.WaitGroup
	rwg.Add(1)
	go func() {
		defer rwg.Done()
		for !stop.Load() {
			names, err := cache.ListNames()
			if err != nil {
				readErrs.Add(1)
				continue
			}
			for _, n := range append(names, "stable") {
				if strings.HasSuffix(n, ".tmp") {
					torn.Add(1) // a half-written folder listed as a member
					continue
				}
				m, meta, err := cache.Load(n, exe)
				reads.Add(1)
				if n == "stable" {
					if err != nil || m == nil || m.Hash != "sha256:stable" {
						stableFaults.Add(1)
					}
					continue
				}
				if err != nil || m == nil {
					readErrs.Add(1) // a miss or a refused pair during a swap: allowed
					continue
				}
				if meta.ManifestHash != m.Hash || m.Provider != n || !strings.HasPrefix(m.Hash, "sha256:"+n+"-") {
					torn.Add(1)
					firstTorn.CompareAndSwap(nil, fmt.Sprintf("%s: meta=%q manifest=%q provider=%q", n, meta.ManifestHash, m.Hash, m.Provider))
				}
			}
		}
	}()

	wg.Wait()
	stop.Store(true)
	rwg.Wait()

	if torn.Load() > 0 {
		t.Errorf("%d torn read(s) served as valid — first: %v", torn.Load(), firstTorn.Load())
	}
	if stableFaults.Load() > 0 {
		t.Errorf("%d read(s) of an untouched sidecar failed or changed while others were saved", stableFaults.Load())
	}
	if saveErrs.Load() > 0 {
		t.Errorf("%d Save(s) of distinct sidecars failed under concurrency — first: %v", saveErrs.Load(), firstSaveErr.Load())
	}
	// After the storm every sidecar holds its LAST save, consistently.
	for w := 0; w < writers; w++ {
		name := fmt.Sprintf("sidecar-%d", w)
		m, meta, err := cache.Load(name, exe)
		want := fmt.Sprintf("sha256:%s-%d", name, rounds-1)
		if err != nil || m == nil || m.Hash != want || meta.ManifestHash != want {
			t.Errorf("%s after refresh: (%v, %v, %v), want hash %s", name, m, meta, err, want)
		}
	}
	t.Logf("%d reads during %d concurrent saves; %d transient misses/refusals, 0 torn", reads.Load(), writers*rounds, readErrs.Load())
}

// TestCache_StaleRecord_HashMismatchIsRefused — T1 integrity: a record whose
// metadata names a different manifest hash than the manifest it sits beside
// (an interrupted write, a hand edit) is refused, never served; and the rebuild
// path — Invalidate then Save — restores a readable record.
func TestCache_StaleRecord_HashMismatchIsRefused(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := seedCache(t, cache, makeManifest("ktw-docs", "1.0.0", "sha256:good"))

	metaPath := filepath.Join(cache.RecordDir("ktw-docs", exe), "metadata.yaml")
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta Metadata
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	meta.ManifestHash = "sha256:some-other-version"
	out, _ := yaml.Marshal(&meta)
	if err := os.WriteFile(metaPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	m, md, err := cache.Load("ktw-docs", exe)
	if err == nil || m != nil || md != nil {
		t.Fatalf("Load of a mismatched pair = (%v, %v, %v), want a refusal", m, md, err)
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("refusal does not name the reason: %v", err)
	}

	if err := cache.Invalidate("ktw-docs"); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	if err := cache.Save("ktw-docs", makeManifest("ktw-docs", "1.0.0", "sha256:rebuilt"), exe, SourceDigests{}); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	if m, _, err := cache.Load("ktw-docs", exe); err != nil || m.Hash != "sha256:rebuilt" {
		t.Errorf("after rebuild Load = (%v, %v), want the rebuilt record", m, err)
	}
}

// TestCache_StaleRecord_MissingExe — T1 invalidation: a record whose binary is
// gone reports stale with reason "exe missing" (the hub then re-probes; with no
// binary it keeps serving the record — G12's no-spawn discovery).
func TestCache_StaleRecord_MissingExe(t *testing.T) {
	cache := NewCache(t.TempDir())
	exe := seedCache(t, cache, makeManifest("ktw-docs", "1.0.0", "sha256:good"))
	_, meta, err := cache.Load("ktw-docs", exe)
	if err != nil {
		t.Fatal(err)
	}
	if stale, why := cache.IsStale(meta, SourceDigests{Binary: exe}); stale {
		t.Fatalf("fresh record reads stale: %s", why)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	stale, why := cache.IsStale(meta, SourceDigests{Binary: exe})
	if !stale || !strings.HasPrefix(why, "exe missing") {
		t.Errorf("IsStale with the exe gone = (%v, %q), want (true, \"exe missing: …\")", stale, why)
	}
}

// TestMemoryCatalog_ColdStartAndConcurrentReplace — T7 (the consumer view):
// an empty view answers every read, and a refresh swapping it wholesale while
// readers list never exposes a half-built view: each List sees one complete
// generation (every provider of it, none of another). Run with -race.
func TestMemoryCatalog_ColdStartAndConcurrentReplace(t *testing.T) {
	ctx := context.Background()
	view := NewMemoryCatalog(nil, nil)
	if ms, err := view.List(ctx); err != nil || len(ms) != 0 {
		t.Fatalf("cold view List = (%d, %v), want empty", len(ms), err)
	}
	if _, err := view.Get(ctx, "ktw-docs"); !errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
		t.Errorf("cold view Get = %v, want ErrSidecarNotInCatalog", err)
	}

	generation := func(g int) []*mbapi.Manifest {
		return []*mbapi.Manifest{
			makeManifest("a", "1", fmt.Sprintf("sha256:gen-%d", g)),
			makeManifest("b", "1", fmt.Sprintf("sha256:gen-%d", g)),
			makeManifest("c", "1", fmt.Sprintf("sha256:gen-%d", g)),
		}
	}
	view.Replace(generation(0), nil)

	var stop atomic.Bool
	var mixed atomic.Int64
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				ms, err := view.List(ctx)
				if err != nil || len(ms) != 3 {
					mixed.Add(1)
					continue
				}
				for _, m := range ms[1:] {
					if m.Hash != ms[0].Hash {
						mixed.Add(1)
					}
				}
			}
		}()
	}
	for g := 1; g <= 200; g++ {
		view.Replace(generation(g), nil)
	}
	stop.Store(true)
	wg.Wait()
	if mixed.Load() > 0 {
		t.Errorf("%d List(s) saw a partial or mixed view during Replace", mixed.Load())
	}
}
