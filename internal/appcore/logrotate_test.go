// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// : a binary's log is bounded - past its limit it is rotated
// to <name>.log.1, one previous file kept; a file already past the limit is
// rotated when it is opened; a file another process rotated away is reopened.
func TestASidecarLogRotatesPastItsLimit(t *testing.T) {
	prev := maxLogBytes.Load()
	maxLogBytes.Store(1 << 10)
	t.Cleanup(func() { maxLogBytes.Store(prev) })

	dir := t.TempDir()
	path := filepath.Join(dir, "ktw-x.log")
	line := append(bytes.Repeat([]byte("x"), 99), '\n')

	r, err := openRotatingLog(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 50 { // 5 000 bytes through a 1 KiB limit: several rotations
		if _, err := r.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if st, err := os.Stat(path); err != nil || st.Size() > 1<<10 {
		t.Fatalf("%s is %v bytes past its 1 KiB limit (err %v)", path, sizeOf(st), err)
	}
	if st, err := os.Stat(path + ".1"); err != nil || st.Size() > 1<<10 || st.Size() == 0 {
		t.Fatalf("the previous file %s.1 = %v bytes (err %v), want one bounded previous file", path, sizeOf(st), err)
	}
	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Fatalf("more than one previous file kept: %s.2 exists", path)
	}
	_ = r.file.Close()

	// A file already past the limit is rotated when it is opened.
	if err := os.WriteFile(path, bytes.Repeat(line, 20), 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := openRotatingLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st == nil || st.Size() != 0 {
		t.Fatalf("an oversized log opened as is: %v bytes", sizeOf(st))
	}

	// Another process rotated the file away while this one holds it: within
	// logRecheckEvery writes the writer reopens the path.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatalf("a log held open cannot be moved aside: %v", err)
	}
	small := []byte("y\n")
	for range logRecheckEvery {
		if _, err := r2.Write(small); err != nil {
			t.Fatal(err)
		}
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Fatalf("after another process rotated it, nothing was written to %s again (%v bytes, err %v)", path, sizeOf(st), err)
	}
	_ = r2.file.Close()
}

func sizeOf(st os.FileInfo) any {
	if st == nil {
		return "no file"
	}
	return st.Size()
}

// Two processes write one log: one rotates it twice while the other still
// holds the file it opened; both keep writing, and the other comes back to the
// path within logRecheckEvery writes.
func TestTwoWritersShareARotatingLog(t *testing.T) {
	prev := maxLogBytes.Load()
	maxLogBytes.Store(1 << 10)
	t.Cleanup(func() { maxLogBytes.Store(prev) })

	path := filepath.Join(t.TempDir(), "ktw-x.log")
	a, err := openRotatingLog(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := openRotatingLog(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.file.Close(); _ = b.file.Close() })

	line := append(bytes.Repeat([]byte("a"), 99), '\n')
	if _, err := b.Write([]byte("b\n")); err != nil {
		t.Fatal(err)
	}
	for range 30 { // two rotations and more, b holding its first file
		if _, err := a.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if st, err := os.Stat(path); err != nil || st.Size() > 1<<10 {
		t.Fatalf("%s = %v bytes (err %v), want it bounded while another writer holds an old file", path, sizeOf(st), err)
	}
	for range logRecheckEvery {
		if _, err := b.Write([]byte("b\n")); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("b\n")) {
		t.Fatalf("the other writer never came back to %s after it was rotated", path)
	}
}
