// SPDX-License-Identifier: Apache-2.0

package frozenhost

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, c string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPackFrozenFolderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "pack")
	write(t, filepath.Join(folder, "ktw-host.yaml"), "name: demo\n")
	write(t, filepath.Join(folder, "skills", "x.md"), "---\nname: x\n---\nbody")

	host := []byte("FAKE-KTW-HOST-BINARY-BYTES-0123456789")
	frozen, err := Pack(host, folder)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(frozen, host) {
		t.Fatal("frozen binary must start with the host bytes (still runnable)")
	}
	if !IsFrozen(frozen) {
		t.Fatal("frozen binary must carry the trailer")
	}

	exe := filepath.Join(dir, "frozen.exe")
	if err := os.WriteFile(exe, frozen, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")

	got, err := FrozenFolder(exe, cache)
	if err != nil {
		t.Fatalf("FrozenFolder: %v", err)
	}
	if got == "" {
		t.Fatal("a frozen exe should yield an extracted folder")
	}
	for _, rel := range []string{"ktw-host.yaml", filepath.Join("skills", "x.md")} {
		if _, err := os.Stat(filepath.Join(got, rel)); err != nil {
			t.Errorf("extracted folder missing %q: %v", rel, err)
		}
	}
	// idempotent: a second call reuses the same dir without error
	got2, err := FrozenFolder(exe, cache)
	if err != nil || got2 != got {
		t.Fatalf("second FrozenFolder should reuse the cache: %q vs %q (%v)", got2, got, err)
	}

	// a plain (non-frozen) binary yields "" (falls back to --folder)
	plain := filepath.Join(dir, "plain.exe")
	if err := os.WriteFile(plain, host, 0o755); err != nil {
		t.Fatal(err)
	}
	if f, err := FrozenFolder(plain, cache); err != nil || f != "" {
		t.Fatalf("plain binary must not be treated as frozen: %q (%v)", f, err)
	}
}
