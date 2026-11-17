// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// probeStub stands in for the exec probe of a compiled member: the member
// named after the binary publishes one entry of its own, plus the system set
// (one entry here) when asked to include it — what `sc resources manifest
// [--include-system=false]` returns. The manifest is what the BINARY carries:
// its version is the placeholder file's content, so two binaries of one name
// answer two manifests, as two builds would. It counts the probes it answered
// and the ones that asked for the system set. gate, when set, is called first
// with the probe's ordinal (1-based), so a test can hold probes.
type probeStub struct {
	probes     atomic.Int32
	withSystem atomic.Int32
	gate       func(n int32)
}

func fakeProbe(t *testing.T) *probeStub {
	t.Helper()
	st := &probeStub{}
	prev := probeManifest
	probeManifest = func(binary, _ string, includeSystem bool) (*mbapi.Manifest, error) {
		n := st.probes.Add(1)
		if st.gate != nil {
			st.gate(n)
		}
		name := strings.TrimSuffix(filepath.Base(binary), ".exe")
		version := "1"
		if b, err := os.ReadFile(binary); err == nil && len(b) > 0 {
			version = string(b)
		}
		m := &mbapi.Manifest{Provider: name, Version: version, Hash: name + "-own-" + version}
		m.Contributions = append(m.Contributions, contribapi.Contribution{Provider: name, Type: "skill", ID: "own"})
		if includeSystem {
			st.withSystem.Add(1)
			m.Contributions = append(m.Contributions, systemEntry())
			m.Hash = name + "-with-system"
		}
		return m, nil
	}
	t.Cleanup(func() { probeManifest = prev })
	return st
}

func systemEntry() contribapi.Contribution {
	return contribapi.Contribution{Provider: "system", Type: "contribution-type", ID: "skill"}
}

// saveHubRecord writes the hub's own record the way saveHubOwnManifest does —
// its own entry plus the system set, include_system: true — without building
// the hub application (TestUniverseListsTheHubsOwnOfferings covers the real
// builder).
func saveHubRecord(t *testing.T, cache *manifestlocal.Cache, binDir string) {
	t.Helper()
	exe := filepath.Join(binDir, hubProvider+".exe")
	if _, err := os.Stat(exe); err != nil {
		if err := os.WriteFile(exe, []byte(hubProvider), 0o644); err != nil {
			t.Error(err)
			return
		}
	}
	m := &mbapi.Manifest{Provider: hubProvider, Version: "1", Hash: "hub-with-system"}
	m.Contributions = append(m.Contributions,
		contribapi.Contribution{Provider: hubProvider, Type: "service", ID: "catalog-service"}, systemEntry())
	// The fixture manifest goes through the REAL writer (saveHubOwnManifest)
	// via its two seams, so what the tests exercise is what the hub runs.
	prevManifest, prevExe := hubOwnManifest, hubOwnExecutable
	hubOwnManifest = func(context.Context, bool) (*mbapi.Manifest, error) { return m, nil }
	hubOwnExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { hubOwnManifest, hubOwnExecutable = prevManifest, prevExe })
	if err := saveHubOwnManifest(context.Background(), cache); err != nil {
		t.Errorf("save the hub's record: %v", err)
	}
}

// members builds a Univers.yaml `sidecars:` list whose binaries exist (the
// freshness check stats them), in the given order.
func members(t *testing.T, binDir string, names ...string) []appcore.SidecarSpec {
	t.Helper()
	out := make([]appcore.SidecarSpec, 0, len(names))
	for _, n := range names {
		exe := filepath.Join(binDir, n+".exe")
		if _, err := os.Stat(exe); err != nil {
			if err := os.WriteFile(exe, []byte(n), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		out = append(out, appcore.SidecarSpec{Name: n, Binary: exe})
	}
	return out
}

// systemCopies counts the system set in what the catalog of a universe with
// this membership loads from the cache (catalogMembers: the declared sidecars
// plus the hub): one per record carrying it. A member with no binary has no
// record and counts nothing.
func systemCopies(t *testing.T, cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) int {
	t.Helper()
	n := 0
	for _, member := range catalogMembers(sidecars) {
		m, _, err := cache.Load(member.Name, member.Binary)
		if err != nil {
			t.Fatalf("cache.Load(%s, %s): %v", member.Name, member.Binary, err)
		}
		if m == nil {
			continue
		}
		for _, c := range m.Contributions {
			if c.Provider == "system" {
				n++
				break
			}
		}
	}
	return n
}

func hydrate(t *testing.T, cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) {
	t.Helper()
	if _, _, errs := hydrateCache(cache, sidecars, io.Discard); errs != 0 {
		t.Errorf("hydrateCache: %d errors", errs)
	}
}

// assertMembersWithoutSystem checks every member record says how it was
// probed: without the set, whatever universe probed it.
func assertMembersWithoutSystem(t *testing.T, cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) {
	t.Helper()
	for _, s := range sidecars {
		if _, meta, _ := cache.Load(s.Name, s.Binary); meta != nil && meta.IncludeSystem {
			t.Errorf("%s record include_system = true; only the hub carries the set", s.Name)
		}
	}
}

// TestUniverseHoldsTheSystemSetOnce_ReorderedMembers — e24 / N2, e26: the
// universe holds the system set exactly once whatever its member order, over a
// WARM cache: the hub's record carries it, no member's does. Reordering or
// dropping members changes nothing in any record, so nothing is re-probed.
func TestUniverseHoldsTheSystemSetOnce_ReorderedMembers(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	saveHubRecord(t, cache, bin)

	first := members(t, bin, "ktw-a", "ktw-b", "ktw-c")
	hydrate(t, cache, first)
	if got := systemCopies(t, cache, first); got != 1 {
		t.Fatalf("cold hydrate: %d system copies, want 1", got)
	}
	st.probes.Store(0)
	for _, list := range [][]appcore.SidecarSpec{
		first,
		members(t, bin, "ktw-b", "ktw-a", "ktw-c"),
		members(t, bin, "ktw-b", "ktw-c"),
	} {
		hydrate(t, cache, list)
		if got := systemCopies(t, cache, list); got != 1 {
			t.Fatalf("members %v: %d system copies, want 1", memberNames(catalogMembers(list)), got)
		}
		assertMembersWithoutSystem(t, cache, list)
	}
	if n := st.probes.Load(); n != 0 {
		t.Errorf("warm hydrates re-probed %d member(s); a member's record must not depend on the universe's order", n)
	}
	if n := st.withSystem.Load(); n != 0 {
		t.Errorf("%d probe(s) asked a member for the system set; members are always probed without it", n)
	}
}

// TestUniverseHoldsTheSystemSetOnce_TwoUniversesShareOneCache — e24 / N2, e26:
// two universes on one machine share the tier-1 cache and declare different
// first members. Each view holds exactly one copy (the hub's), and the other
// universe's hydration re-probes nothing: the shared records suit both.
func TestUniverseHoldsTheSystemSetOnce_TwoUniversesShareOneCache(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()
	saveHubRecord(t, cache, bin)

	u1 := members(t, bin, "ktw-a", "ktw-b")
	u2 := members(t, bin, "ktw-b", "ktw-c")

	hydrate(t, cache, u1)
	hydrate(t, cache, u2)
	st.probes.Store(0)
	hydrate(t, cache, u1)
	hydrate(t, cache, u2)
	for i, u := range [][]appcore.SidecarSpec{u1, u2} {
		if got := systemCopies(t, cache, u); got != 1 {
			t.Errorf("universe %d: %d system copies, want 1", i+1, got)
		}
	}
	if n := st.probes.Load(); n != 0 {
		t.Errorf("alternating universes re-probed %d member(s) over a warm shared cache", n)
	}
}

// TestUniverseHoldsTheSystemSetOnce_TwoHubsAtOnce — e26: two hubs of two
// differently ordered universes hydrating one cache AT ONCE. Hub 1's probes
// are held after its freshness checks; hub 2 records its own manifest and
// hydrates fully in that window; then hub 1 resumes and saves over hub 2's
// records. Before e26 the two hubs wanted opposite records for the same member
// and each rewrote it for its own role; now both views count exactly one copy.
func TestUniverseHoldsTheSystemSetOnce_TwoHubsAtOnce(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	bin := t.TempDir()

	u1 := members(t, bin, "ktw-a", "ktw-b", "ktw-c")
	u2 := members(t, bin, "ktw-c", "ktw-b", "ktw-a")

	held := make(chan struct{}, len(u1))
	release := make(chan struct{})
	st.gate = func(n int32) {
		if int(n) <= len(u1) { // hub 1's probes
			held <- struct{}{}
			<-release
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		saveHubRecord(t, cache, bin)
		hydrate(t, cache, u1)
	}()
	for range u1 {
		<-held // hub 1 found every member cold and is mid-probe
	}
	saveHubRecord(t, cache, bin)
	hydrate(t, cache, u2)
	close(release)
	<-done

	for i, u := range [][]appcore.SidecarSpec{u1, u2} {
		if got := systemCopies(t, cache, u); got != 1 {
			t.Errorf("universe %d after both hubs hydrated: %d system copies, want 1", i+1, got)
		}
		assertMembersWithoutSystem(t, cache, u)
	}
	if n := st.withSystem.Load(); n != 0 {
		t.Errorf("%d probe(s) asked a member for the system set", n)
	}
}

// TestUniverseWithNoBinaryMemberHasTheSystemSet — e26: a universe whose members
// declare no binary (nothing is probed) still holds the system set once,
// through the hub's own record.
func TestUniverseWithNoBinaryMemberHasTheSystemSet(t *testing.T) {
	st := fakeProbe(t)
	cache := manifestlocal.NewCache(t.TempDir())
	saveHubRecord(t, cache, t.TempDir())
	u := []appcore.SidecarSpec{{Name: "host-only"}}
	hydrate(t, cache, u)
	if n := st.probes.Load(); n != 0 {
		t.Fatalf("%d probe(s) for a universe with no binary member", n)
	}
	if got := systemCopies(t, cache, u); got != 1 {
		t.Fatalf("%d system copies, want 1 (the hub's)", got)
	}
}

// TestCarriesSystemSet pins the rule: the hub's own record carries the set,
// no member's does, whatever the membership.
func TestCarriesSystemSet(t *testing.T) {
	sc := []appcore.SidecarSpec{{Name: "ktw-a", Binary: "a"}, {Name: "ktw-b", Binary: "b"}}
	for _, name := range []string{"ktw-a", "ktw-b", "host-only"} {
		if sourceDigestsIn(name, sc).IncludeSystem {
			t.Errorf("member %q would be recorded with the system set", name)
		}
	}
	if !sourceDigestsIn(hubProvider, sc).IncludeSystem || !sourceDigestsIn(hubProvider, nil).IncludeSystem {
		t.Error("the hub's own record would be recorded without the system set")
	}
}
