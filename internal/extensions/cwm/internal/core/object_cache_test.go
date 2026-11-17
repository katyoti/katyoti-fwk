// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// : an object file not changed since its last reading is
// answered from memory, a changed one is read again, and what a caller does
// to its answer never reaches the next reader.
func TestAnObjectFileIsReadAgainOnlyWhenItChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.cwm.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("id: m.X-1\ntype: note\ntags: [a]\ncapabilities:\n  plan: {state: ready, steps: [{step: one}]}\n")
	first, rev1, err := readObjectFileRev(path)
	if err != nil {
		t.Fatal(err)
	}
	first.Tags[0] = "changed"
	first.Capabilities["plan"]["state"] = "changed"
	first.Capabilities["plan"]["steps"].([]any)[0].(map[string]any)["step"] = "changed"
	again, rev2, err := readObjectFileRev(path)
	if err != nil {
		t.Fatal(err)
	}
	if rev2 != rev1 || again.Tags[0] != "a" || again.Capabilities["plan"]["state"] != "ready" ||
		again.Capabilities["plan"]["steps"].([]any)[0].(map[string]any)["step"] != "one" {
		t.Fatalf("a caller's change reached the next reader: %+v", again)
	}
	write("id: m.X-1\ntype: note\ntags: [b]\ncapabilities:\n  plan: {state: done, steps: [{step: one}]}\n")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	changed, rev3, err := readObjectFileRev(path)
	if err != nil {
		t.Fatal(err)
	}
	if rev3 == rev1 || changed.Tags[0] != "b" || changed.Capabilities["plan"]["state"] != "done" {
		t.Fatalf("a changed file answered from memory: %+v", changed)
	}
}

// : an outside edit that keeps the file's size and time (a rewrite
// within the clock's tick of the previous write, or a tool restoring the
// time) is still read again - a file whose time is too recent to tell such an
// edit apart is never answered from memory; a file older than that window
// is.
func TestAnEditKeepingSizeAndTimeIsNeverServedFromMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.cwm.yaml")
	if err := os.WriteFile(path, []byte("id: m.X-1\ntype: note\nname: aaaa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := readObjectFileRev(path)
	if err != nil || first.Name != "aaaa" {
		t.Fatalf("first read: %+v, %v", first, err)
	}
	// The hand edit: same size, the time put back to the first write's.
	if err := os.WriteFile(path, []byte("id: m.X-1\ntype: note\nname: bbbb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	again, _, err := readObjectFileRev(path)
	if err != nil || again.Name != "bbbb" {
		t.Fatalf("an edit keeping size and time was answered from memory: name %q, %v", again.Name, err)
	}

	// A file whose time is older than the window is kept in memory.
	old := filepath.Join(dir, "old.cwm.yaml")
	if err := os.WriteFile(old, []byte("id: m.X-2\ntype: note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readObjectFileRev(old); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := cachedObjectFile(old); !ok {
		t.Fatal("a file older than the window was not kept in memory")
	}
	if _, _, _, ok := cachedObjectFile(path); ok {
		t.Fatal("a file written within the window was kept in memory")
	}
}
