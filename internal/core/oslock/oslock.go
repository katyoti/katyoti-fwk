// SPDX-License-Identifier: Apache-2.0

// Package oslock is an exclusive lock on a file, held by the operating system
// for as long as the process that took it lives: a process that sleeps keeps
// it, a process that dies releases it — however it dies. Holding one is the
// proof that its holder is alive, with no PID or heartbeat to judge.
//
// The lock does not stop the file from being read: its holder writes who it
// is (Lock.Note) for the others to read.
package oslock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrLocked: another holder has the lock.
var ErrLocked = errors.New("locked by another process")

// Lock is a lock this process holds.
type Lock struct {
	f    *os.File
	path string
}

// TryLock takes the lock on path (created if missing) without waiting:
// ErrLocked when another holder has it.
func TryLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{f: f, path: path}, nil
}

// Note writes s in the lock's file (who holds it, for the others to read).
func (l *Lock) Note(s string) error {
	if err := l.f.Truncate(0); err != nil {
		return err
	}
	_, err := l.f.WriteAt([]byte(s), 0)
	return err
}

// StillHeld reports whether path still names the file this lock holds — false
// when it was removed or replaced (on Unix a removed file's lock guards
// nothing: another process could lock a new file at the same path).
func (l *Lock) StillHeld() bool {
	mine, err := l.f.Stat()
	if err != nil {
		return false
	}
	now, err := os.Stat(l.path)
	return err == nil && os.SameFile(mine, now)
}

// Unlock releases the lock.
func (l *Lock) Unlock() error {
	err := unlockFile(l.f)
	return errors.Join(err, l.f.Close())
}

// Held reports whether path is locked by a holder (this process's other
// locks included). A missing file is not held.
func Held(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	switch err := lockFile(f); {
	case errors.Is(err, ErrLocked):
		return true, nil
	case err != nil:
		return false, err
	}
	_ = unlockFile(f)
	return false, nil
}

// Note reads what the holder of path wrote in it ("" when nothing).
func Note(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func wrap(path string, err error) error { return fmt.Errorf("oslock %s: %w", path, err) }
