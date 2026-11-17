// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
)

// : the holds file's lock is taken over only from a dead
// holder and removed only by its holder (the shared core/oslock package, the
// mechanism also uses for its own store's lock).

// TestALiveHoldersHoldsLockIsNeverTakenOverHoweverOld: a live holder working
// for an hour keeps the lock - a writer waits and gives up rather than take
// it, and the lock file stays, still held.
func TestALiveHoldersHoldsLockIsNeverTakenOverHoweverOld(t *testing.T) {
	defer func(w time.Duration) { holdsLockWait = w }(holdsLockWait)
	holdsLockWait = 300 * time.Millisecond

	dir := t.TempDir()
	path := filepath.Join(dir, holdsLockFile)
	holder, err := oslock.TryLock(path) // another handle, as another process holds it
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Unlock()
	_ = holder.Note("pid 4242")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	ran := false
	err = holdsLocked(dir, func() error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("a writer took a live holder's holds lock (err %v, ran %v)", err, ran)
	}
	if !holder.StillHeld() {
		t.Fatal("the live holder's lock file was removed or replaced by a writer")
	}
	if held, _ := oslock.Held(path); !held {
		t.Fatal("the live holder no longer holds its lock")
	}
}

// TestADeadHoldersHoldsLockIsTakenAtOnce: a lock its holder left on dying is
// taken at once, and the writer releases it - leaving the file - when it ends.
func TestADeadHoldersHoldsLockIsTakenAtOnce(t *testing.T) {
	defer func(w time.Duration) { holdsLockWait = w }(holdsLockWait)
	holdsLockWait = 300 * time.Millisecond

	dir := t.TempDir()
	path := filepath.Join(dir, holdsLockFile)
	dead, err := oslock.TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = dead.Note("pid 4242")
	_ = dead.Unlock() // its process died: the lock is released, the file stays

	ran := false
	if err := holdsLocked(dir, func() error {
		ran = true
		if held, _ := oslock.Held(path); !held {
			return errors.New("the writer runs without holding the lock")
		}
		return nil
	}); err != nil || !ran {
		t.Fatalf("a dead holder's holds lock was not taken (err %v, ran %v)", err, ran)
	}
	if held, _ := oslock.Held(path); held {
		t.Fatal("the writer kept the lock after it ended")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file was removed: %v; no process removes it", err)
	}
}

// TestTheHoldsLockIsPlanificationsOwnNotTheSchedulers: the increment holds'
// lock is planification's `.holds.lock`, never the scheduler store's `.lock` -
// a process holding a `.lock` in the same folder does not keep a holds writer
// out.
func TestTheHoldsLockIsPlanificationsOwnNotTheSchedulers(t *testing.T) {
	defer func(w time.Duration) { holdsLockWait = w }(holdsLockWait)
	holdsLockWait = 300 * time.Millisecond

	if holdsLockFile != ".holds.lock" {
		t.Fatalf("the holds lock file is %q, want planification's own .holds.lock", holdsLockFile)
	}
	dir := t.TempDir()
	other, err := oslock.TryLock(filepath.Join(dir, ".lock")) // the scheduler store's lock name
	if err != nil {
		t.Fatal(err)
	}
	defer other.Unlock()

	ran := false
	if err := holdsLocked(dir, func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("a held .lock kept the holds writer out (err %v, ran %v)", err, ran)
	}
	if _, err := os.Stat(filepath.Join(dir, holdsLockFile)); err != nil {
		t.Fatalf("the holds writer did not take its own %s: %v", holdsLockFile, err)
	}
}
