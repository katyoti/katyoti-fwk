// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFSLoader_ForwardSlashRoot pins the fix for the mixed-separator bug:
// a rootDir given with forward slashes (e.g. a `--folder C:/foo` CLI arg on
// Windows) must still yield RELATIVE List entries that ReadFile can resolve.
// Before the NewFSLoader filepath.Clean normalization, List returned absolute
// paths (the strip prefix used OS separators while the root used slashes), and
// the follow-on ReadFile joined two absolutes → file not found.
func TestFSLoader_ForwardSlashRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := "---\nname: flow-spec\n---\nbody"
	if err := os.WriteFile(filepath.Join(dir, "skills", "flow-spec.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	// Force forward slashes regardless of what t.TempDir returned.
	forwardRoot := filepath.ToSlash(dir)
	l := NewFSLoader(forwardRoot)

	got := l.List("skills/*.md")
	if len(got) != 1 {
		t.Fatalf("List(skills/*.md) = %v, want exactly one entry", got)
	}
	// The entry MUST be relative to the root, not absolute.
	if filepath.IsAbs(got[0]) || got[0] != "skills/flow-spec.md" {
		t.Fatalf("List entry = %q, want relative %q", got[0], "skills/flow-spec.md")
	}
	// And the relative entry must round-trip through ReadFile.
	data, err := l.ReadFile(got[0])
	if err != nil || string(data) != want {
		t.Fatalf("ReadFile(%q) = (%q, %v), want (%q, nil)", got[0], data, err, want)
	}

	// Walk must likewise return relative, OS-agnostic paths.
	walked := l.Walk()
	if len(walked) != 1 || walked[0] != "skills/flow-spec.md" {
		t.Fatalf("Walk() = %v, want [skills/flow-spec.md]", walked)
	}
}
