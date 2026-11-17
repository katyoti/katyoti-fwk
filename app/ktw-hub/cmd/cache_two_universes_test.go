// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// universeNaming builds one universe's membership: the member `name`, whose
// binary lives in a bin folder of its own and carries `label` as its content
// (the probe stub answers that content as the manifest version, as a build
// of its own would), serving a content root of its own.
func universeNaming(t *testing.T, name, label string) []appcore.SidecarSpec {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name+".exe")
	if err := os.WriteFile(exe, []byte("built-for-"+label), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, label+".md"), []byte(label), 0o644); err != nil {
		t.Fatal(err)
	}
	return []appcore.SidecarSpec{{Name: name, Binary: exe, ContentRoot: root}}
}

// versionOf is the version a catalog serves for a member — which binary's
// manifest it holds.
func versionOf(t *testing.T, cat mbapi.Catalog, name string) string {
	t.Helper()
	m, err := cat.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("catalog.Get(%s): %v", name, err)
	}
	return m.Version
}

// TestUniverseServesOnlyTheBinaryItDeclares_TwoUniversesOneCache — blind
// audit H1 (e29, item 1): two universes on one host share the tier-1 cache
// and both name a member `ktw-x`, each at a binary of its own with a content
// root of its own. A record is keyed by (name, declared binary), so each
// universe sees only the record of the binary it declares: the other
// universe's record never reads fresh for it, its hydration writes its own
// record BESIDE the other rather than over it, and a Reload — what
// WatchCache runs in one hub when the other hub writes — keeps each catalog
// on its own manifest. Over a warm cache neither universe re-probes.
func TestUniverseServesOnlyTheBinaryItDeclares_TwoUniversesOneCache(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	uA := universeNaming(t, "ktw-x", "a")
	uB := universeNaming(t, "ktw-x", "b")
	exeA, exeB := uA[0].Binary, uB[0].Binary

	// Universe A hydrates; its catalog serves A's binary.
	hydrate(t, cache, uA)
	catA, err := manifestlocal.NewCatalog(cache, catalogMembers(uA))
	if err != nil {
		t.Fatalf("NewCatalog(A): %v", err)
	}
	if v := versionOf(t, catA, "ktw-x"); v != "built-for-a" {
		t.Fatalf("universe A serves ktw-x version %q, want built-for-a", v)
	}

	// The audit's symptom: universe B, declaring ktw-x at its own binary,
	// read A's record as fresh and served A's manifest.
	wantB := sourceDigestsIn("ktw-x", uB)
	fresh, err := cacheIsFresh(cache, "ktw-x", exeB, wantB)
	if err != nil || fresh {
		t.Fatalf("universe B (binary %s) sees A's record (binary %s) as fresh=%t err=%v; B must probe its own binary", exeB, exeA, fresh, err)
	}
	// Asked for B, A's record says why it is not B's.
	_, metaA, err := cache.Load("ktw-x", exeA)
	if err != nil || metaA == nil {
		t.Fatalf("A's record by its own key: meta=%v err=%v", metaA, err)
	}
	if stale, why := cache.IsStale(metaA, wantB); !stale || !strings.Contains(why, exeA) || !strings.Contains(why, exeB) {
		t.Errorf("IsStale(A's record, B's key) = (%t, %q); want stale, naming both binaries", stale, why)
	}

	// Universe B hydrates: one probe, of its own binary; A's record survives.
	st.probes.Store(0)
	hydrate(t, cache, uB)
	if n := st.probes.Load(); n != 1 {
		t.Fatalf("universe B probed %d time(s), want 1 (its own binary)", n)
	}
	catB, err := manifestlocal.NewCatalog(cache, catalogMembers(uB))
	if err != nil {
		t.Fatalf("NewCatalog(B): %v", err)
	}
	if v := versionOf(t, catB, "ktw-x"); v != "built-for-b" {
		t.Fatalf("universe B serves ktw-x version %q, want built-for-b", v)
	}
	// What WatchCache does in hub A when hub B writes the cache: Reload.
	if err := catA.(catalogReloader).Reload(); err != nil {
		t.Fatalf("Reload(A) after B's hydrate: %v", err)
	}
	if v := versionOf(t, catA, "ktw-x"); v != "built-for-a" {
		t.Fatalf("after B's hydrate, universe A serves ktw-x version %q — B's record was swapped into A's view", v)
	}

	// Warm: each universe has its own record; alternating them re-probes
	// nothing.
	st.probes.Store(0)
	for _, u := range [][]appcore.SidecarSpec{uA, uB, uA, uB} {
		hydrate(t, cache, u)
	}
	if n := st.probes.Load(); n != 0 {
		t.Errorf("alternating universes re-probed %d time(s) over a warm cache; each has its own record", n)
	}

	// Two records of one name, side by side; ListNames counts the name once.
	records, err := cache.Records()
	if err != nil {
		t.Fatal(err)
	}
	ofX := 0
	for _, r := range records {
		if r.Name == "ktw-x" {
			ofX++
		}
	}
	if ofX != 2 {
		t.Errorf("%d record(s) of ktw-x, want 2 (one per declared binary)", ofX)
	}
	names, _ := cache.ListNames()
	if len(names) != 2 { // ktw-x and the hub
		t.Errorf("ListNames = %v, want [ktw-hub ktw-x]", names)
	}
}

// TestUniverseRedeclaringABinaryReprobes — the same defect inside one
// universe: Univers.yaml moves ktw-x to another binary at a path that also
// exists. The record of the old binary is not the new binary's, and its
// stat is never consulted for it.
func TestUniverseRedeclaringABinaryReprobes(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	before := universeNaming(t, "ktw-x", "old")
	hydrate(t, cache, before)

	after := universeNaming(t, "ktw-x", "new")
	after[0].ContentRoot = before[0].ContentRoot // same content; only the binary moved
	// The audit's symptom, stated directly: the old binary's record reads
	// fresh for the new binary, because the stat was of the RECORDED path.
	if fresh, err := cacheIsFresh(cache, "ktw-x", after[0].Binary, sourceDigestsIn("ktw-x", after)); err != nil || fresh {
		t.Fatalf("the record of binary %s reads fresh=%t err=%v for the declared binary %s", before[0].Binary, fresh, err, after[0].Binary)
	}
	st.probes.Store(0)
	hydrate(t, cache, after)
	if n := st.probes.Load(); n != 1 {
		t.Fatalf("redeclared binary re-probed %d time(s), want 1", n)
	}
	cat, err := manifestlocal.NewCatalog(cache, catalogMembers(after))
	if err != nil {
		t.Fatal(err)
	}
	if v := versionOf(t, cat, "ktw-x"); v != "built-for-new" {
		t.Fatalf("after redeclaring the binary the view serves %q, want built-for-new", v)
	}
}
