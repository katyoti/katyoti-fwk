// SPDX-License-Identifier: Apache-2.0

// Package atomicio is the platform's crash-safe file I/O primitive: CWM's
// storage, lockfile and object files, and every typed config file written
// through config.SaveAtomic. One implementation of the atomic protocol, so
// every writer and reader of a file follows the same rules.
//
// Two functions, both with Windows-aware bounded retry:
//
//   - WriteAtomic: tempfile (unique per call via os.CreateTemp) →
//     write → fsync → close → rename → sync the rename (the parent
//     directory on POSIX, the replaced file on Windows). A reader of
//     path always sees either the prior bytes or the full new bytes —
//     never partial — and a power loss after it returns keeps the new.
//   - ReadAtomic:  read with retries that absorb Windows
//     ERROR_ACCESS_DENIED collisions with in-flight renames, and the
//     file-not-found an open meets inside a replace (replaceState).
//
// On POSIX both retry loops are effectively no-ops (rename is
// atomic, readers don't see sharing violations). On Windows the
// retries are load-bearing.
//
// Rule: modules/framework/rules/R-003-files-shared-across-goroutines-or-processes-go.cwm.md.
package atomicio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// The retry policy both sides of the atomic protocol share.
//
// # Why a time budget rather than a count
//
// It was 50 attempts × 2 ms = 100 ms, which reads like a lot of retries and is
// a tenth of a second. What it has to absorb is another process holding the
// file open — on Windows that is typically an on-access antivirus scan, and a
// scan of a file this code just wrote routinely exceeds 100 ms on a loaded
// machine. The count said "we tried hard"; the budget said otherwise, and reads
// failed under load with a sharing violation.
//
// So the bound is stated as the thing that actually matters — how long we are
// willing to wait — and the backoff grows so a long hold is not also a busy
// loop. The common case is unchanged: the first attempt succeeds and none of
// this runs.
const (
	retryBudget       = 3 * time.Second
	retryBackoffStart = 2 * time.Millisecond
	retryBackoffMax   = 50 * time.Millisecond
)

// retryTransient runs op until it succeeds, until stop reports the error is
// final, or until the budget elapses. It returns the last error.
//
// One implementation for the write side and the read side: they are two halves
// of one protocol and a policy that differed between them would be a race
// nobody could reason about.
func retryTransient(op func() error, stop func(error) bool) error {
	deadline := time.Now().Add(retryBudget)
	backoff := retryBackoffStart
	var lastErr error
	for {
		err := op()
		if err == nil {
			return nil
		}
		if stop != nil && stop(err) {
			return err
		}
		lastErr = err
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(backoff)
		if backoff < retryBackoffMax {
			backoff *= 2
			if backoff > retryBackoffMax {
				backoff = retryBackoffMax
			}
		}
	}
}

// NoSyncEnv, set to "1" in a test process, makes WriteAtomic skip its fsync
// and its durability step: a test's universe lives in a temporary folder no
// power loss can matter to, and the syncs are most of a planning test's time
// on Windows. The replace stays atomic. Never set by a
// product binary.
const NoSyncEnv = "KTW_ATOMICIO_NO_SYNC"

// noSync reads NoSyncEnv once, at the first write.
var noSync = sync.OnceValue(func() bool { return os.Getenv(NoSyncEnv) == "1" })

// WriteAtomic publishes data to path with crash-safe semantics.
//
// Sequence: create parent dir → open unique tempfile in the same
// dir → write → fsync → close → rename onto path. A reader of path
// always sees either the prior bytes or the full new bytes — never
// partial. The tempfile is unique per call (via os.CreateTemp) so
// concurrent writers don't collide. On any error before rename, the
// tempfile is removed.
//
// On Windows the final rename may transiently fail when a reader is
// holding the destination open; we retry a bounded number of times.
// On POSIX rename is atomic and the retry loop is a no-op.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomicio: mkdir %s: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomicio: create temp in %s: %w", dir, err)
	}
	tmp := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("atomicio: write %s: %w", tmp, err)
	}
	if err := syncTemp(f); err != nil {
		cleanup()
		return fmt.Errorf("atomicio: fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomicio: close %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomicio: chmod %s: %w", tmp, err)
	}

	// Every rename failure is treated as transient: on Windows a reader holding
	// the destination open is the expected case, and a genuinely broken rename
	// (bad path, gone volume) fails the same way every attempt and is reported
	// when the budget runs out.
	r := replacing(path, true)
	r.inflight.Add(1)
	err = retryTransient(func() error { return replaceFile(tmp, path) }, nil)
	r.inflight.Add(-1)
	r.done.Add(1)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomicio: rename %s → %s within %s: %w",
			tmp, path, retryBudget, err)
	}
	// The new bytes are on disk (fsync above), but the rename that names them
	// is metadata the OS may still hold in memory: after a power loss the name
	// could point at the old file again. syncReplaced makes it durable; until
	// it has, the write is not reported done.
	// A path gone again by then was removed (or replaced) by another writer
	// after this rename: there is nothing of this write left to make durable.
	if noSync() {
		return nil
	}
	if err := retryTransient(func() error {
		if err := syncReplaced(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}, nil); err != nil {
		return fmt.Errorf("atomicio: make the replace of %s durable within %s: %w",
			path, retryBudget, err)
	}
	return nil
}

// syncTemp flushes the temporary file before its rename, unless NoSyncEnv.
func syncTemp(f *os.File) error {
	if noSync() {
		return nil
	}
	return f.Sync()
}

// ReadAtomic reads path with bounded retries on Windows
// sharing-violation errors. The companion to WriteAtomic — both
// sides of the atomic publish/consume protocol need the same retry
// budget on Windows where reads can transiently collide with a
// concurrent rename. On POSIX the retry loop is a no-op.
//
// Returns os.ErrNotExist (unwrapped via errors.Is) when path does
// not exist, so callers can distinguish "missing" from "transiently
// unavailable".
func ReadAtomic(path string) ([]byte, error) {
	var data []byte
	err := retryTransient(
		func() error {
			before := replacing(path, false).snapshot()
			var rerr error
			data, rerr = readShared(path)
			if os.IsNotExist(rerr) && replacing(path, false).overlapped(before) {
				// Missing only because a replace of this path ran during the
				// open (see replaceState): transient, read again.
				return errReplacing
			}
			return rerr
		},
		// "Missing" is an answer, not a transient state — a caller
		// distinguishes it from "briefly unavailable" and must not wait out
		// the budget for it.
		os.IsNotExist,
	)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("atomicio: read %s within %s: %w", path, retryBudget, err)
	}
	return data, nil
}

// A READ NEVER SEES A FILE MISSING BECAUSE IT IS BEING REPLACED.
//
// On Windows the replace is a rename with POSIX semantics (rename_windows.go:
// FILE_RENAME_POSIX_SEMANTICS, so readers holding the old file do not starve
// the writer). That replace is not atomic to a concurrent open: under load a
// CreateFile on the destination returns ERROR_FILE_NOT_FOUND while the rename
// is in progress (measured: raw error 2, the POSIX rename succeeding, never the
// os.Rename fallback) — the destination's old name is gone before the new one
// is linked. The reader read "missing" for an object that was only being
// rewritten (TestAReadDuringAWriteDoesNotFail, a few readers at once, about one
// full cwm/core run in four on a loaded machine).
//
// "Missing" must stay an immediate answer (a caller asks for absent files all
// the time), so the read cannot simply retry it. Instead the writer marks the
// replace: each path WriteAtomic replaces has a replaceState — replaces in
// flight, replaces finished. A reader snapshots it before the open; when the
// open says missing and a replace of that path was in flight or finished
// meanwhile, the miss is the replace's window and the reader reads again.
// Otherwise the file is missing and the reader says so at once.
//
// It covers readers in the writer's process — every CWM object reader, since
// CWM holds one writer per workspace. On POSIX the rename is atomic and the
// overlap never yields a miss; the bookkeeping is the same and costs two
// atomic loads per read.

// errReplacing is the transient error a reader retries through.
var errReplacing = errors.New("atomicio: the file is being replaced")

type replaceState struct {
	inflight atomic.Int64
	done     atomic.Uint64
}

type replaceSnapshot struct {
	inflight int64
	done     uint64
}

// replaces holds a state per absolute path ever replaced in this process.
var replaces sync.Map // string → *replaceState

// noReplace is the state of a path never replaced: nothing overlaps it.
var noReplace = &replaceState{}

// replacing is path's state; create adds one for a writer.
func replacing(path string, create bool) *replaceState {
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	if v, ok := replaces.Load(key); ok {
		return v.(*replaceState)
	}
	if !create {
		return noReplace
	}
	v, _ := replaces.LoadOrStore(key, &replaceState{})
	return v.(*replaceState)
}

func (r *replaceState) snapshot() replaceSnapshot {
	return replaceSnapshot{inflight: r.inflight.Load(), done: r.done.Load()}
}

// overlapped reports a replace in flight before or now, or one finished since.
func (r *replaceState) overlapped(before replaceSnapshot) bool {
	return before.inflight > 0 || r.inflight.Load() > 0 || r.done.Load() != before.done
}
