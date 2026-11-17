// SPDX-License-Identifier: Apache-2.0

package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func exe(dir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// leftovers reports staging files (.new-* / .old-*) beside target — a clean swap
// leaves none (nothing holds the aside in a test process).
func leftovers(t *testing.T, target string) []string {
	t.Helper()
	var out []string
	for _, marker := range []string{newMarker, oldMarker} {
		m, err := filepath.Glob(target + marker + "*")
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		out = append(out, m...)
	}
	return out
}

// TestReplace_FreshInstall — no existing target → created with the new content.
func TestReplace_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-hub")
	src := filepath.Join(dir, "src-bin")
	write(t, src, "NEW-v2")

	if err := Replace(target, src); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if got := read(t, target); got != "NEW-v2" {
		t.Errorf("target content = %q, want NEW-v2", got)
	}
	if lo := leftovers(t, target); len(lo) != 0 {
		t.Errorf("unexpected staging leftovers: %v", lo)
	}
}

// TestReplace_CreatesTargetDir — a target whose parent directory does not exist
// yet (fresh install-infra into an empty control-plane location) is created.
func TestReplace_CreatesTargetDir(t *testing.T) {
	dir := t.TempDir()
	target := exe(filepath.Join(dir, "control-plane"), "ktw-gui") // parent absent
	src := filepath.Join(dir, "src-bin")
	write(t, src, "NEW-v2")

	if err := Replace(target, src); err != nil {
		t.Fatalf("Replace into a missing dir: %v", err)
	}
	if got := read(t, target); got != "NEW-v2" {
		t.Errorf("target content = %q, want NEW-v2", got)
	}
}

// TestReplace_OverExisting — an existing (not-held) target is swapped and the
// aside is swept (nothing holds it in-process), leaving no staging files.
func TestReplace_OverExisting(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-gui")
	write(t, target, "OLD-v1")
	src := filepath.Join(dir, "src-bin")
	write(t, src, "NEW-v2")

	if err := Replace(target, src); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if got := read(t, target); got != "NEW-v2" {
		t.Errorf("target content = %q, want NEW-v2", got)
	}
	if lo := leftovers(t, target); len(lo) != 0 {
		t.Errorf("staging leftovers after clean swap: %v", lo)
	}
	// The source is consumed as a copy — the original stays put.
	if got := read(t, src); got != "NEW-v2" {
		t.Errorf("source should be untouched, got %q", got)
	}
}

// TestReplace_ExecBit — the published binary is executable (Unix; chmod is a
// no-op on Windows).
func TestReplace_ExecBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits not meaningful on Windows")
	}
	dir := t.TempDir()
	target := exe(dir, "katyoti")
	src := filepath.Join(dir, "src-bin")
	// Deliberately non-executable source to prove Replace sets the bit.
	if err := os.WriteFile(src, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Replace(target, src); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("target not executable: mode %v", fi.Mode())
	}
}

// TestReplace_MissingSource — a missing new binary errors and leaves the target
// untouched (no partial swap, no leftovers).
func TestReplace_MissingSource(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-hub")
	write(t, target, "OLD-v1")

	err := Replace(target, filepath.Join(dir, "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if got := read(t, target); got != "OLD-v1" {
		t.Errorf("target should be untouched, got %q", got)
	}
	if lo := leftovers(t, target); len(lo) != 0 {
		t.Errorf("no leftovers expected on failure, got %v", lo)
	}
}

// TestReplace_SourceIsDir — a directory source is rejected before any staging.
func TestReplace_SourceIsDir(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-hub")
	write(t, target, "OLD-v1")
	srcDir := filepath.Join(dir, "adir")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(target, srcDir); err == nil {
		t.Fatal("expected error for directory source")
	}
	if got := read(t, target); got != "OLD-v1" {
		t.Errorf("target should be untouched, got %q", got)
	}
}

// TestCleanupOld — sweeps only the <target>.old-* aside files, leaving the target
// and unrelated files intact.
func TestCleanupOld(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-gui")
	write(t, target, "current")
	write(t, target+oldMarker+"111", "stale-1")
	write(t, target+oldMarker+"222", "stale-2")
	unrelated := filepath.Join(dir, "keep.txt")
	write(t, unrelated, "keep")

	if err := CleanupOld(target); err != nil {
		t.Fatalf("CleanupOld: %v", err)
	}
	if lo, _ := filepath.Glob(target + oldMarker + "*"); len(lo) != 0 {
		t.Errorf(".old leftovers after cleanup: %v", lo)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target removed by cleanup: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated file removed by cleanup: %v", err)
	}
}

// TestReplace_Idempotentish — running Replace twice lands the latest content and
// never accumulates staging files.
func TestReplace_Idempotentish(t *testing.T) {
	dir := t.TempDir()
	target := exe(dir, "ktw-hub")
	src := filepath.Join(dir, "src-bin")

	write(t, src, "v2")
	if err := Replace(target, src); err != nil {
		t.Fatalf("first Replace: %v", err)
	}
	write(t, src, "v3")
	if err := Replace(target, src); err != nil {
		t.Fatalf("second Replace: %v", err)
	}
	if got := read(t, target); got != "v3" {
		t.Errorf("target = %q, want v3", got)
	}
	if lo := leftovers(t, target); len(lo) != 0 {
		t.Errorf("staging leftovers after two swaps: %v", lo)
	}
}

// IsLeftover names a swap's leftovers and never a program.
func TestIsLeftover(t *testing.T) {
	for name, want := range map[string]bool{
		"katyoti.exe.old-1792597412233358100": true,
		"ktw-hub.exe.new-17":                  true,
		"ktw-mcp.exe.old-4242":                true,
		"notes.old":                           true,
		"katyoti.exe":                         false,
		"ktw-hub":                             false,
		"SHA256SUMS":                          false,
		"ktw-gui-assets.tar.gz":               false,
	} {
		if got := IsLeftover(name); got != want {
			t.Errorf("IsLeftover(%q) = %v, want %v", name, got, want)
		}
	}
}
