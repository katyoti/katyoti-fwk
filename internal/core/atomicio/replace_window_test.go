// SPDX-License-Identifier: Apache-2.0

package atomicio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAMissDuringAReplaceIsNotAnAnswer: a read whose open overlapped a replace
// of its path counts the miss as the replace's window; one that overlapped
// nothing answers missing.
func TestAMissDuringAReplaceIsNotAnAnswer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o.yaml")
	if replacing(path, false).overlapped(replacing(path, false).snapshot()) {
		t.Fatal("a path never replaced overlaps a replace")
	}
	r := replacing(path, true)
	before := r.snapshot()
	r.inflight.Add(1) // a replace starts during the open
	if !r.overlapped(before) {
		t.Fatal("a replace in flight is not seen")
	}
	r.inflight.Add(-1)
	r.done.Add(1)
	if !r.overlapped(before) {
		t.Fatal("a replace finished during the open is not seen")
	}
	if r.overlapped(r.snapshot()) {
		t.Fatal("a replace finished before the open is seen")
	}
	// Through WriteAtomic: its replace is counted.
	before = replacing(path, false).snapshot()
	if err := WriteAtomic(path, []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !replacing(path, false).overlapped(before) {
		t.Fatal("WriteAtomic's replace is not counted")
	}
}

// TestMissingIsAnsweredAtOnce: a file that is simply absent is reported
// missing without waiting out the retry budget.
func TestMissingIsAnsweredAtOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.yaml")
	start := time.Now()
	if _, err := ReadAtomic(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want not-exist", err)
	}
	// Also for a path this process wrote then removed.
	if err := WriteAtomic(path, []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAtomic(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want not-exist", err)
	}
	if d := time.Since(start); d > retryBudget/2 {
		t.Fatalf("missing took %s", d)
	}
}

// TestAReadInsideAReplaceWindowWaitsForTheFile reproduces the window
// deterministically: the path is absent while a replace of it is in flight
// (as NTFS shows it between unlinking the old file and linking the new). The
// read waits it out instead of answering missing.
func TestAReadInsideAReplaceWindowWaitsForTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "o.yaml")
	if err := WriteAtomic(path, []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := replacing(path, true)
	r.inflight.Add(1)
	if err := os.Rename(path, path+".hold"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.Rename(path+".hold", path)
		r.inflight.Add(-1)
		r.done.Add(1)
	}()
	data, err := ReadAtomic(path)
	if err != nil || string(data) != "a: 1\n" {
		t.Fatalf("a read inside a replace window: %q, %v", data, err)
	}
}
