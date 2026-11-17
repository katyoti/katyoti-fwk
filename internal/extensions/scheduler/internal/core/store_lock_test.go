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

// : the to-dos' lock is taken over only from a dead holder and
// removed only by its holder.

// TestALiveHoldersLockIsNeverTakenOverHoweverOld: a live holder that works
// past the old 30 s keeps the lock - a writer waits for it and gives up rather
// than take it, and the lock file stays, still held by its holder.
func TestALiveHoldersLockIsNeverTakenOverHoweverOld(t *testing.T) {
	defer func(w time.Duration) { lockWait = w }(lockWait)
	lockWait = 300 * time.Millisecond

	space := t.TempDir()
	path := filepath.Join(space, todoDir, lockFile)
	holder, err := oslock.TryLock(path) // the live holder: another handle, as another process holds it
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Unlock()
	_ = holder.Note("pid 4242")
	old := time.Now().Add(-time.Hour) // working for an hour: well past any age rule
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	ran := false
	err = locked(space, func() error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("a writer took a live holder's lock (err %v, ran %v)", err, ran)
	}
	if !holder.StillHeld() {
		t.Fatal("the live holder's lock file was removed or replaced by a writer")
	}
	if held, _ := oslock.Held(path); !held {
		t.Fatal("the live holder no longer holds its lock")
	}
}

// TestADeadHoldersLockIsTakenAtOnce: a lock its holder left on dying (the file
// with its note, no holder any more) is taken at once, however young.
func TestADeadHoldersLockIsTakenAtOnce(t *testing.T) {
	defer func(w time.Duration) { lockWait = w }(lockWait)
	lockWait = 300 * time.Millisecond

	space := t.TempDir()
	path := filepath.Join(space, todoDir, lockFile)
	dead, err := oslock.TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = dead.Note("pid 4242")
	_ = dead.Unlock() // its process died: the system released its lock, the file stays

	ran := false
	if err := locked(space, func() error {
		ran = true
		if held, _ := oslock.Held(path); !held {
			return errors.New("the writer runs without holding the lock")
		}
		return nil
	}); err != nil || !ran {
		t.Fatalf("a dead holder's lock was not taken (err %v, ran %v)", err, ran)
	}
	if held, _ := oslock.Held(path); held {
		t.Fatal("the writer kept the lock after it ended")
	}
}

// TestWritersOfOneModuleRunOneAtATime: two writers on one module never run
// together.
func TestWritersOfOneModuleRunOneAtATime(t *testing.T) {
	space := t.TempDir()
	inside := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- locked(space, func() error { close(inside); <-release; return nil })
	}()
	<-inside
	second := make(chan error, 1)
	secondRan := make(chan struct{})
	go func() {
		second <- locked(space, func() error { close(secondRan); return nil })
	}()
	select {
	case <-secondRan:
		t.Fatal("a second writer ran while the first held the lock")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the second writer, after the first: %v", err)
	}
}
