// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// watch_exe_test.go — a compiled member rebuilt while the hub runs is
// re-probed within the poll interval (blind audit M1). The poll used to cover
// served folders only, and the exe stat ran only under allow_overwrites, so a
// rebuild waited for a hub restart or `catalog refresh`.

// TestWatch_ARebuiltMemberIsReprobedWithinThePollInterval drives the real
// watch loop and the real exe-stat decision over a real record, with the
// interval injected: an untouched member is never probed; a rebuilt one is
// probed within a few rounds; once its record is rewritten it is quiet again;
// the hub itself is never a member of the watch.
func TestWatch_ARebuiltMemberIsReprobedWithinThePollInterval(t *testing.T) {
	root := t.TempDir()
	cache := manifestlocal.NewCache(filepath.Join(root, "sidecars"))
	exe := filepath.Join(root, "bin", "ktw-a.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := func(version string) {
		m := &mbapi.Manifest{Provider: "ktw-a", Version: version, Hash: "a-" + version}
		if err := cache.Save("ktw-a", m, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
			t.Fatal(err)
		}
	}
	record("1")

	hubExe := filepath.Join(root, "bin", hubProvider+".exe")
	if err := os.WriteFile(hubExe, []byte("hub"), 0o644); err != nil {
		t.Fatal(err)
	}
	members := func() []appcore.SidecarSpec {
		return []appcore.SidecarSpec{
			{Name: "ktw-a", Binary: exe},
			{Name: hubProvider, Binary: hubExe}, // never watched: a rebuilt hub is a restart
			{Name: "no-binary"},                 // not a member of the cache
		}
	}
	watch := newExeStatWatch(cache)
	moved := func(s appcore.SidecarSpec) (bool, string) {
		return watch.moved(s.Name, s.Binary), "executable changed"
	}
	var probes atomic.Int32
	refreshed := make(chan string, 8)
	refresh := func(name string) error {
		probes.Add(1)
		record("2") // what refreshMember does: probe, save the new record
		refreshed <- name
		return nil
	}

	const interval = 25 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchMembers(ctx, interval, members, moved, refresh, io.Discard, io.Discard)

	// Untouched: rounds pass, nothing is probed.
	time.Sleep(8 * interval)
	if n := probes.Load(); n != 0 {
		t.Fatalf("an untouched member was re-probed %d time(s)", n)
	}

	// Rebuilt: same size, other bytes, a later stamp — set explicitly so the
	// test does not depend on the clock having moved between two writes.
	// The new binary lands whole, as a build's output does: written and
	// stamped beside it, then renamed in. Written in place, a round between
	// the write and the stamp re-probed it twice.
	next := exe + ".new"
	if err := os.WriteFile(next, []byte("build-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(next, later, later); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, exe); err != nil {
		t.Fatal(err)
	}
	select {
	case name := <-refreshed:
		if name != "ktw-a" {
			t.Fatalf("re-probed %q, want ktw-a", name)
		}
	case <-time.After(40 * interval):
		t.Fatalf("a rebuilt member was not re-probed within %v (%d rounds)", 40*interval, 40)
	}

	// The new record matches the new binary: quiet again, one probe in all.
	time.Sleep(8 * interval)
	if n := probes.Load(); n != 1 {
		t.Fatalf("after the re-probe the member was probed again: %d probes in all", n)
	}
	// And the hub's own binary never reached the refresh, whatever its stat.
	if err := os.WriteFile(hubExe, []byte("hub-rebuilt"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(8 * interval)
	if n := probes.Load(); n != 1 {
		t.Fatalf("the hub's own rebuild was probed as a member: %d probes in all", n)
	}
}

// TestExeStatWatch_ComparesAgainstTheRecordNotABaseline: a member whose
// binary was rebuilt BETWEEN the record being written and the first round is
// moved on that first round. A watch seeded from the binary as first seen
// would have called the rebuilt binary "what it has always been".
func TestExeStatWatch_ComparesAgainstTheRecordNotABaseline(t *testing.T) {
	root := t.TempDir()
	cache := manifestlocal.NewCache(filepath.Join(root, "sidecars"))
	exe := filepath.Join(root, "bin", "ktw-b.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &mbapi.Manifest{Provider: "ktw-b", Version: "1", Hash: "b-1"}
	if err := cache.Save("ktw-b", m, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	// Rebuilt before anyone looked.
	if err := os.WriteFile(exe, []byte("build-2!"), 0o644); err != nil {
		t.Fatal(err)
	}
	watch := newExeStatWatch(cache)
	if !watch.moved("ktw-b", exe) {
		t.Fatal("a rebuild made before the first round must read as moved on the first round")
	}
	// A member with no record is hydration's: never moved here.
	if watch.moved("ktw-none", exe) {
		t.Fatal("a member with no record is not the watch's to probe")
	}
	// A missing binary is not a change to probe either.
	if watch.moved("ktw-b", filepath.Join(root, "bin", "gone.exe")) {
		t.Fatal("an absent binary has nothing to probe")
	}
}
