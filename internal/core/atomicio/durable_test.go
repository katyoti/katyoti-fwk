// SPDX-License-Identifier: Apache-2.0

package atomicio

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A REPLACE IS MADE DURABLE: after the rename, WriteAtomic runs the step that
// writes the new name to disk (the parent directory's fsync on POSIX, the
// replaced file's flush on Windows) — with the new bytes already in place — and
// a write whose step fails is not reported done.
func TestAReplaceIsMadeDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "obj.yaml")
	if err := WriteAtomic(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	real := syncReplaced
	t.Cleanup(func() { syncReplaced = real })
	var synced []string
	syncReplaced = func(p string) error {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, []byte("new")) {
			t.Errorf("the durability step ran before the rename: %s holds %q (%v)", p, got, err)
		}
		synced = append(synced, p)
		return real(p) // the platform's own step works on a real file
	}
	if err := WriteAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 || synced[0] != path {
		t.Fatalf("the replace of %s was not made durable: durability step ran on %v", path, synced)
	}

	// Removed by another writer between the rename and the durability step:
	// nothing of this write is left to make durable - the write is done.
	syncReplaced = func(p string) error {
		_ = os.Remove(p)
		return real(p)
	}
	if err := WriteAtomic(path, []byte("gone"), 0o600); err != nil {
		t.Fatalf("a replace removed by another writer right after it failed the write: %v", err)
	}

	failed := errors.New("disk gone")
	syncReplaced = func(string) error { return failed }
	if err := WriteAtomic(path, []byte("newer"), 0o600); !errors.Is(err, failed) {
		t.Fatalf("a replace that could not be made durable was reported done: %v", err)
	}
}
