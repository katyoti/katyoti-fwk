// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// readingMember stands in for ktw-agents: one binary, whose manifest is
// whatever its universe's skills folder holds — one `universe:` skill per
// file — and which declares that folder as a read root. The probe reads the
// folder of the universe named by *probing, the way the real probe inherits
// the hub's universe; duringProbe, when set, runs after the folder is read
// and before the probe returns (a save landing while the probe runs).
func readingMember(t *testing.T, probing *string, duringProbe ...func()) []appcore.SidecarSpec {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "ktw-agents.exe")
	if err := os.WriteFile(exe, []byte("one-build"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := probeManifest
	probeManifest = func(binary, _ string, _ bool) (*mbapi.Manifest, error) {
		m := &mbapi.Manifest{
			Provider:  "ktw-agents",
			Version:   "1",
			ReadRoots: []mbapi.ReadRoot{{Anchor: mbapi.ReadRootUniverse, Path: "skills"}},
		}
		var ids []string
		entries, _ := os.ReadDir(filepath.Join(*probing, "skills"))
		for _, e := range entries {
			id := strings.TrimSuffix(e.Name(), ".md")
			ids = append(ids, id)
			m.Contributions = append(m.Contributions, contribapi.Contribution{Provider: "universe", Type: "skill", ID: id})
		}
		m.Hash = "ktw-agents-" + strings.Join(ids, "+")
		for _, f := range duringProbe {
			f()
		}
		return m, nil
	}
	t.Cleanup(func() { probeManifest = prev })
	return []appcore.SidecarSpec{{Name: "ktw-agents", Binary: exe}}
}

// universeWithSkills is a universe folder whose skills folder holds one file
// per name (none: no skills folder at all).
func universeWithSkills(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	if len(names) == 0 {
		return dir
	}
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, "skills", n+".md"), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// skillsServed lists the `universe:` skills a catalog serves for ktw-agents.
func skillsServed(t *testing.T, cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) []string {
	t.Helper()
	cat, err := manifestlocal.NewCatalog(cache, catalogMembers(sidecars))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	m, err := cat.Get(context.Background(), "ktw-agents")
	if err != nil {
		t.Fatalf("catalog.Get(ktw-agents): %v", err)
	}
	var out []string
	for _, c := range m.Contributions {
		if c.Provider == "universe" {
			out = append(out, c.ID)
		}
	}
	sort.Strings(out)
	return out
}

// TestReadRoots_EachUniverseServesItsOwnRecord —: two
// universes on one host share the tier-1 cache and the same ktw-agents binary,
// and each has skills of its own in its universe folder. The record was keyed
// by (name, binary) only, so the last universe to probe wrote the record both
// served: universe A listed B's `universe:` skills. A manifest that declares a
// universe read root is now a record per universe: each serves its own, and
// neither reads the other's as stale, so two hubs running at once do not
// re-probe each other's record on every round. The very first probe of the
// binary on the host learns its read roots and is confirmed by one more probe;
// B's first probe already knows them from A's record.
func TestReadRoots_EachUniverseServesItsOwnRecord(t *testing.T) {
	root, global := t.TempDir(), t.TempDir()
	uA, uB := universeWithSkills(t, "alpha"), universeWithSkills(t, "beta")
	var probing string
	sidecars := readingMember(t, &probing)
	exe := sidecars[0].Binary
	cacheA := manifestlocal.NewUniverseCache(root, uA, global)
	cacheB := manifestlocal.NewUniverseCache(root, uB, global)
	saveHubRecord(t, cacheA, t.TempDir())

	probing = uA
	hydrate(t, cacheA, sidecars)
	if moved, why := cacheA.ReadsMoved("ktw-agents", exe); !moved || !strings.Contains(why, "learnt") {
		t.Fatalf("the first probe on the host: ReadsMoved = %t (%q); want a confirming probe", moved, why)
	}
	hydrate(t, cacheA, sidecars) // the confirming probe
	probing = uB
	hydrate(t, cacheB, sidecars)

	if got := skillsServed(t, cacheA, sidecars); strings.Join(got, ",") != "alpha" {
		t.Errorf("universe A serves universe: skills %v, want [alpha] (B's record must not reach A)", got)
	}
	if got := skillsServed(t, cacheB, sidecars); strings.Join(got, ",") != "beta" {
		t.Errorf("universe B serves universe: skills %v, want [beta]", got)
	}
	// B's probe left A's record intact and fresh: no re-probe ping-pong.
	want := sourceDigestsIn("ktw-agents", sidecars)
	if fresh, err := cacheIsFresh(cacheA, "ktw-agents", exe, want); err != nil || !fresh {
		t.Errorf("after B probed, A's record fresh=%t err=%v; want fresh", fresh, err)
	}
	if fresh, err := cacheIsFresh(cacheB, "ktw-agents", exe, want); err != nil || !fresh {
		t.Errorf("B's record fresh=%t err=%v; want fresh", fresh, err)
	}
}

// TestReadRoots_AnEditInAReadFolderIsSeen —: a skill
// added to the universe folder leaves the ktw-agents executable unchanged, so
// the exe stat never noticed it and the catalog kept the old list until the
// binary changed. The record keeps the declared folders' digest; the member
// watch (ReadsMoved) and the staleness check see the edit, including a folder
// created after the record was taken, and the next probe serves it.
func TestReadRoots_AnEditInAReadFolderIsSeen(t *testing.T) {
	root, global := t.TempDir(), t.TempDir()
	uni := universeWithSkills(t) // no skills folder yet
	probing := uni
	sidecars := readingMember(t, &probing)
	exe := sidecars[0].Binary
	cache := manifestlocal.NewUniverseCache(root, uni, global)
	saveHubRecord(t, cache, t.TempDir())
	hydrate(t, cache, sidecars)
	hydrate(t, cache, sidecars) // confirms the roots the first probe learnt
	want := sourceDigestsIn("ktw-agents", sidecars)

	if moved, why := cache.ReadsMoved("ktw-agents", exe); moved {
		t.Fatalf("ReadsMoved right after the probe = true (%s)", why)
	}
	// The folder appears, with a skill in it.
	if err := os.MkdirAll(filepath.Join(uni, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uni, "skills", "fresh.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, why := cache.ReadsMoved("ktw-agents", exe)
	if !moved || !strings.Contains(why, "read folders changed") {
		t.Fatalf("ReadsMoved after adding a skill = %t (%q); want true, read folders changed", moved, why)
	}
	if fresh, _ := cacheIsFresh(cache, "ktw-agents", exe, want); fresh {
		t.Fatal("the record reads fresh after a skill was added to a declared read folder")
	}
	hydrate(t, cache, sidecars)
	if got := skillsServed(t, cache, sidecars); strings.Join(got, ",") != "fresh" {
		t.Errorf("after the re-probe the catalog serves %v, want [fresh]", got)
	}
	if moved, why := cache.ReadsMoved("ktw-agents", exe); moved {
		t.Errorf("ReadsMoved after the re-probe = true (%s)", why)
	}
}

// TestReadRoots_AnEditDuringTheProbeIsNotLost —, the
// reviewer's race: a skill saved while ktw-agents is being probed is not in the
// manifest the probe returns. When the record's fingerprint was taken after
// the probe, it covered that skill, the record read fresh, and the skill was
// never served until something else changed. The fingerprint is now taken
// before the probe, so the next check sees the skill as a change and re-probes.
func TestReadRoots_AnEditDuringTheProbeIsNotLost(t *testing.T) {
	root, global := t.TempDir(), t.TempDir()
	uni := universeWithSkills(t, "first")
	probing := uni
	saveDuring := false
	sidecars := readingMember(t, &probing, func() {
		if !saveDuring {
			return
		}
		saveDuring = false
		if err := os.WriteFile(filepath.Join(uni, "skills", "late.md"), []byte("x"), 0o644); err != nil {
			t.Error(err)
		}
	})
	exe := sidecars[0].Binary
	cache := manifestlocal.NewUniverseCache(root, uni, global)
	saveHubRecord(t, cache, t.TempDir())
	hydrate(t, cache, sidecars)
	hydrate(t, cache, sidecars) // the roots are known from here on

	// An edit makes the record stale; the probe it triggers races a second save.
	if err := os.WriteFile(filepath.Join(uni, "skills", "second.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveDuring = true
	hydrate(t, cache, sidecars)
	if got := skillsServed(t, cache, sidecars); strings.Join(got, ",") != "first,second" {
		t.Fatalf("the probe that raced the save serves %v, want [first second]", got)
	}
	moved, why := cache.ReadsMoved("ktw-agents", exe)
	if !moved {
		t.Fatal("a skill saved during the probe reads as already seen: it would never be served")
	}
	if !strings.Contains(why, "read folders changed") {
		t.Errorf("ReadsMoved reason %q", why)
	}
	hydrate(t, cache, sidecars)
	if got := skillsServed(t, cache, sidecars); strings.Join(got, ",") != "first,late,second" {
		t.Errorf("after the next probe the catalog serves %v, want [first late second]", got)
	}
}
