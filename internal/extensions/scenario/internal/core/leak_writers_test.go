// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// THE LEAK CHECK TELLS THE PERSON'S HUBS' WRITES FROM THE CASE'S
// : on a temporary installation, with the « is this the
// case's? » judge injected, a new sidecar cache record whose metadata exe_path
// lies outside the case folders is a note; the same record written for a case
// binary, or with no metadata.yaml, is a leak; config/Profile.yaml changed
// while a person's hub holds a lock is a note, with no such hub a leak; and
// universes.yaml changed while the person's hub runs is still a leak; a
// person's record changed or removed is a leak with no
// person's hub before or after the verb, a note with one (a hub stopping
// during the verb counts). Falsified: without the exe_path rule the person's
// record is a leak; without the hub condition Profile.yaml with no hub
// passes; without it for changed or removed records, they pass with no hub.
func TestTheLeakCheckTellsThePersonsHubsWritesFromTheCases(t *testing.T) {
	base := t.TempDir()
	host := application.MachineDirs{Root: filepath.Join(base, "root"), Data: filepath.Join(base, "data"), Cache: filepath.Join(base, "cache")}
	write := func(rel, s string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	caseBin := filepath.Join(base, "case", "bin")
	personBin := filepath.Join(base, "person", "bin")
	hubDir := filepath.Join(host.Data, "universes", "dev")
	owner := leakOwner{
		lock: func(dir string) bool { return false }, // every hub here is the person's
		exe:  func(p string) bool { return within(caseBin, p) },
	}
	hubUp := func(up bool) {
		if up {
			if err := hubclient.WriteLockFile(hubDir, hubclient.LockFile{PID: 1, NATSURL: "nats://127.0.0.1:1"}); err != nil {
				t.Fatal(err)
			}
			return
		}
		_ = os.Remove(hubclient.LockFilePath(hubDir))
	}
	check := func(name string, act func()) (leaks, notes []string) {
		before := takeLeakSnapshot(host, nil)
		act()
		return before.leaks(takeLeakSnapshot(host, before), owner)
	}
	has := func(list []string, want string) bool {
		for _, l := range list {
			if strings.Contains(l, want) {
				return true
			}
		}
		return false
	}
	record := func(name, exe string) {
		rel := "cache/katyoti/sidecars/" + name + "@abc"
		write(rel+"/manifest.json", `{"name":"`+name+`"}`)
		if exe != "" {
			write(rel+"/metadata.yaml", "exe_path: "+filepath.ToSlash(exe)+"\n")
		}
	}
	write("root/universes.yaml", "universes: []\n")
	write("root/config/Profile.yaml", "name: a\n")

	// A new record written for the person's binary: a note.
	leaks, notes := check("person record", func() { record("ktw-p", filepath.Join(personBin, "ktw-p.exe")) })
	if len(leaks) != 0 || !has(notes, "ktw-p@abc") {
		t.Errorf("a record for the person's binary: leaks %v notes %v; want a note only", leaks, notes)
	}

	// A record written for a case binary, or with no metadata: leaks.
	leaks, _ = check("case record", func() { record("ktw-c", filepath.Join(caseBin, "ktw-c.exe")) })
	if !has(leaks, "ktw-c@abc") {
		t.Errorf("a record for a case binary: leaks %v; want a leak naming it", leaks)
	}
	leaks, _ = check("no metadata", func() { record("ktw-n", "") })
	if !has(leaks, "ktw-n@abc") {
		t.Errorf("a record with no metadata: leaks %v; want a leak naming it", leaks)
	}

	// Profile.yaml changed while the person's hub is up: a note; with no hub: a leak.
	hubUp(true)
	leaks, notes = check("profile, hub up", func() { write("root/config/Profile.yaml", "name: changed by the person's hub\n") })
	if has(leaks, "Profile.yaml") || !has(notes, "Profile.yaml") {
		t.Errorf("Profile.yaml with the person's hub up: leaks %v notes %v; want a note", leaks, notes)
	}
	// universes.yaml changed while the person's hub runs: still a leak.
	leaks, _ = check("registry, hub up", func() { write("root/universes.yaml", "universes: [x]\n") })
	if !has(leaks, "universes.yaml") {
		t.Errorf("universes.yaml with the person's hub up: leaks %v; want a leak", leaks)
	}
	hubUp(false)
	leaks, _ = check("profile, no hub", func() { write("root/config/Profile.yaml", "name: changed with no hub running\n") })
	if !has(leaks, "Profile.yaml") {
		t.Errorf("Profile.yaml with no person hub: leaks %v; want a leak", leaks)
	}

	// A person's record changed or removed: with no person's
	// hub before or after the verb, a leak; with one, a note.
	recDir := filepath.Join(host.Cache, "katyoti", "sidecars", "ktw-p@abc")
	leaks, _ = check("changed record, no hub", func() { write("cache/katyoti/sidecars/ktw-p@abc/manifest.json", `{"name":"ktw-p","v":2}`) })
	if !has(leaks, "ktw-p@abc") {
		t.Errorf("a person's record changed with no person hub: leaks %v; want a leak", leaks)
	}
	record("ktw-q", filepath.Join(personBin, "ktw-q.exe"))
	leaks, _ = check("removed record, no hub", func() { _ = os.RemoveAll(filepath.Join(host.Cache, "katyoti", "sidecars", "ktw-q@abc")) })
	if !has(leaks, "ktw-q@abc") {
		t.Errorf("a person's record removed with no person hub: leaks %v; want a leak", leaks)
	}
	hubUp(true)
	leaks, notes = check("changed record, hub up", func() { write("cache/katyoti/sidecars/ktw-p@abc/manifest.json", `{"name":"ktw-p","v":3}`) })
	if has(leaks, "ktw-p@abc") || !has(notes, "ktw-p@abc") {
		t.Errorf("a person's record changed with the person's hub up: leaks %v notes %v; want a note", leaks, notes)
	}
	// The hub stops during the verb: it held a lock before, still a note.
	leaks, notes = check("removed record, hub stops", func() {
		_ = os.RemoveAll(recDir)
		hubUp(false)
	})
	if has(leaks, "ktw-p@abc") || !has(notes, "ktw-p@abc") {
		t.Errorf("a person's record removed as the person's hub stops: leaks %v notes %v; want a note", leaks, notes)
	}
}
