// SPDX-License-Identifier: Apache-2.0

package diagnose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// broken writes a temporary installation with something broken in each
// registry and in the control plane, and returns its files' content before
// any repair.
func broken(t *testing.T, w world) map[string]string {
	t.Helper()
	f := w.files()
	gone := filepath.Join(w.base, "gone")
	hubStore := filepath.Join(w.store, "ktw-hub", "2.0", exe("ktw-hub"))
	write(t, hubStore, "hub-bytes")
	write(t, filepath.Join(w.cp, exe("ktw-hub")), "a-dev-build")
	guiStore := filepath.Join(w.store, "ktw-gui", "2.0", exe("ktw-gui"))
	write(t, guiStore, "gui-bytes") // its control-plane copy is gone
	storeReplaced := filepath.Join(w.store, "ktw-b", "1.0", exe("ktw-b"))
	write(t, storeReplaced, "other-bytes")
	good := filepath.Join(w.store, "ktw-a", "1.0", exe("ktw-a"))
	write(t, good, "a-bytes")
	write(t, f.Sidecars, "sidecars:\n"+
		"  - {name: ktw-a, version: '1.0', binary_path: "+q(good)+", sha256: "+sum("a-bytes")+", capabilities: [x]}\n"+
		"  - {name: ktw-b, version: '1.0', binary_path: "+q(storeReplaced)+", sha256: "+sum("b-bytes")+"}\n"+
		"  - {name: ktw-c, version: '1.0', binary_path: "+q(filepath.Join(gone, exe("ktw-c")))+", sha256: "+sum("c")+"}\n"+
		"  - {name: ktw-hub, version: '2.0', source: infra-channel, binary_path: "+q(hubStore)+", sha256: "+sum("hub-bytes")+"}\n"+
		"  - {name: ktw-gui, version: '2.0', source: infra-channel, binary_path: "+q(guiStore)+", sha256: "+sum("gui-bytes")+"}\n")
	testUniverse := filepath.Join(w.tmp, "TestX", "u")
	if err := os.MkdirAll(testUniverse, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, f.Universes, "universes:\n"+
		"  - {name: dev, home_path: "+q(w.root)+"}\n"+
		"  - {name: old, home_path: "+q(filepath.Join(gone, "old"))+"}\n"+
		"  - {name: admin, home_path: "+q(filepath.Join(gone, "admin"))+", protected: true}\n"+
		"  - {name: scn-test, home_path: "+q(testUniverse)+"}\n")
	write(t, f.Instances, "instances:\n"+
		"  - {name: dev, folder: "+q(w.base)+"}\n"+
		"  - {name: removed, folder: "+q(filepath.Join(gone, "Katyoti"))+"}\n")
	write(t, f.Marketplaces, "marketplaces:\n"+
		"  - {name: acme, url: 'https://example.com/m', trust: pinned, pubKey: KEY}\n"+
		"  - {name: seeded, enabled: false}\n"+
		"  - {name: httptest, url: 'http://127.0.0.1:53111/katyoti-marketplace.yaml'}\n")
	before := map[string]string{}
	for _, p := range []string{f.Sidecars, f.Universes, f.Instances, f.Marketplaces, filepath.Join(w.cp, exe("ktw-hub"))} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		before[p] = string(b)
	}
	return before
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The doctor repairs what status names after backing up every file it
// changes, says what it did, keeps every sound row and leaves - saying what
// to do - what it cannot repair.
func TestRepair_RepairsAfterABackupAndSaysWhatItDid(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	before := broken(t, w)
	backups := filepath.Join(w.root, "backups")

	rep, err := Repair(f, backups, false)
	if err != nil {
		t.Fatal(err)
	}
	did := map[string]mktapi.RepairAction{}
	for _, a := range rep.Actions {
		did[a.Problem.Entry] = a
		if a.Did == "" {
			t.Errorf("%s: the doctor says nothing of what it did", a.Problem.Entry)
		}
	}
	for _, e := range []string{"ktw-c", "ktw-hub", "ktw-gui", "old", "scn-test", "removed", "httptest"} {
		if !did[e].Done {
			t.Errorf("%s is not repaired: %+v", e, did[e])
		}
	}
	for _, e := range []string{"ktw-b", "admin"} {
		if a, ok := did[e]; !ok || a.Done || !strings.HasPrefix(a.Did, "left: ") {
			t.Errorf("%s: want left, saying what to do: %+v", e, a)
		}
	}

	// What is left is all status still names.
	after := Diagnose(f)
	left := map[string]bool{}
	for _, p := range after.Problems {
		left[p.Entry] = true
	}
	if len(after.Problems) != 2 || !left["ktw-b"] || !left["admin"] {
		t.Errorf("after the repair status names %+v, want only ktw-b and admin", after.Problems)
	}
	// The sound rows are kept, whole.
	for file, keep := range map[string][]string{
		f.Sidecars:     {"ktw-a", "capabilities", "ktw-b", "ktw-hub", "ktw-gui"},
		f.Universes:    {"dev", "admin"},
		f.Instances:    {"dev"},
		f.Marketplaces: {"acme", "pinned", "KEY", "seeded"},
	} {
		got := read(t, file)
		for _, k := range keep {
			if !strings.Contains(got, k) {
				t.Errorf("%s lost %q:\n%s", filepath.Base(file), k, got)
			}
		}
	}
	if got := read(t, filepath.Join(w.cp, exe("ktw-hub"))); got != "hub-bytes" {
		t.Errorf("the replaced control-plane program is not put back: %q", got)
	}
	if got := read(t, filepath.Join(w.cp, exe("ktw-gui"))); got != "gui-bytes" {
		t.Errorf("the gone control-plane program is not put back: %q", got)
	}

	// The backup holds each changed file as it was, named by the manifest.
	if rep.Backup == "" || !strings.HasPrefix(rep.Backup, backups) {
		t.Fatalf("backup %q, want a folder under %s", rep.Backup, backups)
	}
	manifest := read(t, filepath.Join(rep.Backup, "manifest.yaml"))
	entries, err := os.ReadDir(rep.Backup)
	if err != nil {
		t.Fatal(err)
	}
	copies := map[string]string{}
	for _, e := range entries {
		if e.Name() != "manifest.yaml" {
			copies[strings.SplitN(e.Name(), "-", 2)[1]] = read(t, filepath.Join(rep.Backup, e.Name()))
		}
	}
	for p, was := range before {
		if copies[filepath.Base(p)] != was {
			t.Errorf("the backup of %s is not the file as it was", p)
		}
		if !strings.Contains(manifest, filepath.ToSlash(filepath.Base(p))) {
			t.Errorf("the manifest does not name %s:\n%s", p, manifest)
		}
	}
}

// A dry run says what the doctor would do and changes nothing: no file
// written, no backup made.
func TestRepair_DryRunChangesNothing(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	before := broken(t, w)
	backups := filepath.Join(w.root, "backups")
	rep, err := Repair(f, backups, true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.Backup != "" {
		t.Errorf("dry run %v, backup %q: want a dry run with no backup", rep.DryRun, rep.Backup)
	}
	would := 0
	for _, a := range rep.Actions {
		if a.Done {
			would++
			if !strings.Contains(a.Did, "dry run") {
				t.Errorf("%s: a dry run says it changed nothing: %q", a.Problem.Entry, a.Did)
			}
		}
	}
	if would != 7 {
		t.Errorf("%d would be repaired, want 7: %+v", would, rep.Actions)
	}
	for p, was := range before {
		if read(t, p) != was {
			t.Errorf("a dry run changed %s", p)
		}
	}
	if _, err := os.Stat(filepath.Join(w.cp, exe("ktw-gui"))); !os.IsNotExist(err) {
		t.Errorf("a dry run put a program back")
	}
	if _, err := os.Stat(backups); !os.IsNotExist(err) {
		t.Errorf("a dry run made a backup folder")
	}
}

// Nothing is changed without a backup: with no backup folder the doctor is
// refused and leaves everything as it is.
func TestRepair_NoBackupNoChange(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	before := broken(t, w)
	if _, err := Repair(f, "", false); err == nil {
		t.Fatal("a repair with no backup folder is not refused")
	}
	if _, err := os.Stat(filepath.Join(w.cp, exe("ktw-gui"))); !os.IsNotExist(err) {
		t.Errorf("a program was put back with no backup folder")
	}
	for p, was := range before {
		if read(t, p) != was {
			t.Errorf("%s changed with no backup", p)
		}
	}
}

// A control-plane program is put back only from a store copy that matches
// its checksum: else it is left, untouched.
func TestRepair_UnverifiedStoreCopyIsNotPutBack(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	hubStore := filepath.Join(w.store, "ktw-hub", "2.0", exe("ktw-hub"))
	write(t, hubStore, "tampered")
	target := filepath.Join(w.cp, exe("ktw-hub"))
	write(t, target, "a-dev-build")
	write(t, f.Sidecars, "sidecars:\n  - {name: ktw-hub, version: '2.0', source: infra-channel, binary_path: "+q(hubStore)+", sha256: "+sum("hub-bytes")+"}\n")
	rep, err := Repair(f, filepath.Join(w.root, "backups"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rep.Actions {
		if a.Done {
			t.Errorf("%s repaired from an unverified copy: %q", a.Problem.Entry, a.Did)
		}
	}
	if read(t, target) != "a-dev-build" {
		t.Errorf("the control-plane program was replaced from an unverified copy")
	}
}

// A subscription row is matched by its name and its URL, and said removed only
// once it was removed: a same-named row of another URL is kept, a problem whose
// row is no longer there is left, saying so.
func TestRepair_SubscriptionRowsMatchNameAndURL(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		w := newWorld(t)
		f := w.files()
		test := "http://127.0.0.1:53111/katyoti-marketplace.yaml"
		write(t, f.Marketplaces, "marketplaces:\n"+
			"  - {name: acme, url: '"+test+"'}\n"+
			"  - {name: acme, url: 'https://example.com/m', trust: pinned, pubKey: KEY}\n")
		b := &backup{root: filepath.Join(w.root, "backups"), dryRun: dryRun}
		got, err := f.dropRows(b, f.Marketplaces, []mktapi.InstallationProblem{
			{File: f.Marketplaces, Entry: "acme", Path: test},
			{File: f.Marketplaces, Entry: "gone", Path: "https://example.com/gone"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got[rowKey("acme", test)] != "removed" || len(got) != 1 {
			t.Errorf("dry run %v: %v, want only the test server's row removed", dryRun, got)
		}
		body := read(t, f.Marketplaces)
		if !strings.Contains(body, "https://example.com/m") || !strings.Contains(body, "KEY") {
			t.Errorf("dry run %v: the sound row of the same name is lost:\n%s", dryRun, body)
		}
		if strings.Contains(body, "53111") != dryRun {
			t.Errorf("dry run %v: the test server's row:\n%s", dryRun, body)
		}
	}
}
