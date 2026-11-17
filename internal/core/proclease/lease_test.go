// SPDX-License-Identifier: Apache-2.0

package proclease

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
)

// TestLeaseAcquireRelease — happy path. Acquire creates
// the file with our PID; Release removes it. Re-Acquire on the same
// instance is a no-op success; re-Acquire after Release succeeds.
func TestLeaseAcquireRelease(t *testing.T) {
	root := t.TempDir()
	l := New(filepath.Join(root, ".lock"))
	l.SetLeaseTTL(2 * time.Second)
	ctx := context.Background()

	if err := l.Acquire(ctx); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if !l.IsHeldBy() {
		t.Errorf("IsHeldBy = false after Acquire")
	}
	if _, err := os.Stat(filepath.Join(root, ".lock")); err != nil {
		t.Errorf("lease file not on disk: %v", err)
	}

	// Idempotent re-Acquire while held.
	if err := l.Acquire(ctx); err != nil {
		t.Errorf("re-Acquire while held: %v", err)
	}

	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if l.IsHeldBy() {
		t.Errorf("IsHeldBy = true after Release")
	}
	if _, err := os.Stat(filepath.Join(root, ".lock")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lease file not removed: stat=%v", err)
	}

	// Re-Acquire after Release.
	if err := l.Acquire(ctx); err != nil {
		t.Errorf("re-Acquire after Release: %v", err)
	}
	_ = l.Release()
}

// TestLeaseRefusesSecondHolder — second NewProcessLockfile
// against the same root must refuse while the first holds it.
// (Same-process, different instances — simulates two distinct PIDs
// via the writeLockRecord helper.)
func TestLeaseRefusesSecondHolder(t *testing.T) {
	root := t.TempDir()

	// Fake an existing holder with a different live PID + fresh
	// heartbeat. We use the parent process's parent PID — guaranteed
	// alive on POSIX; on Windows the test process's parent is also
	// alive. If somehow the PID is unavailable, fall back to PID 4
	// (System on Windows) / 1 (init on Unix), both reliably alive.
	otherPID := pickLiveOtherPID(t)
	writeLockRecord(t, root, lockRecord{
		PID:         otherPID,
		StartedAt:   time.Now().Add(-1 * time.Hour).UTC(),
		LeaseTTL:    "30s",
		HeartbeatAt: time.Now().UTC(),
	})

	l := New(filepath.Join(root, ".lock"))
	err := l.Acquire(context.Background())
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("Acquire over live fresh holder: err=%v, want ErrLockHeldByOtherProcess", err)
	}
	var held *HeldError
	if !errors.As(err, &held) || held.PID != otherPID {
		t.Fatalf("the refusal %v does not name the holder pid %d", err, otherPID)
	}
}

// TestLeaseTakesOverStale — existing record claims an
// unlikely-alive PID. Acquire must succeed.
func TestLeaseTakesOverStale(t *testing.T) {
	root := t.TempDir()

	// 999999 is exceedingly unlikely to be a running process. On
	// Windows the OS uses 4 / 8 / 12 ... low PIDs; user processes
	// commonly hit 10K-100K. POSIX max_pid defaults are 32768. So
	// 999999 is reliably "not alive".
	writeLockRecord(t, root, lockRecord{
		PID:         999999,
		StartedAt:   time.Now().Add(-1 * time.Minute).UTC(),
		LeaseTTL:    "30s",
		HeartbeatAt: time.Now().UTC(),
	})

	l := New(filepath.Join(root, ".lock"))
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire over stale holder: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	// After takeover the file's PID should be us.
	rec := readLockRecord(t, root)
	if rec.PID != os.Getpid() {
		t.Errorf("post-takeover PID = %d, want %d", rec.PID, os.Getpid())
	}
}

// TestLeaseTakesOverWedged — existing record claims a
// live PID (in this test, our own) but the heartbeat is older than
// 2× TTL. Acquire treats it as wedged and renews.
func TestLeaseTakesOverWedged(t *testing.T) {
	root := t.TempDir()
	writeLockRecord(t, root, lockRecord{
		PID:         os.Getpid(),
		StartedAt:   time.Now().Add(-1 * time.Hour).UTC(),
		LeaseTTL:    "1s",
		HeartbeatAt: time.Now().Add(-10 * time.Minute).UTC(),
	})

	l := New(filepath.Join(root, ".lock"))
	l.SetLeaseTTL(1 * time.Second)
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire over wedged holder: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	rec := readLockRecord(t, root)
	if time.Since(rec.HeartbeatAt) > 5*time.Second {
		t.Errorf("heartbeat not refreshed on takeover: %v ago", time.Since(rec.HeartbeatAt))
	}
}

// TestLeaseHeartbeatUpdates — Acquire with a short TTL,
// wait > TTL/2, read the file; HeartbeatAt should have advanced.
func TestLeaseHeartbeatUpdates(t *testing.T) {
	root := t.TempDir()
	l := New(filepath.Join(root, ".lock"))
	l.SetLeaseTTL(600 * time.Millisecond) // → heartbeat interval 200ms

	if err := l.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { _ = l.Release() })

	initial := readLockRecord(t, root).HeartbeatAt
	time.Sleep(500 * time.Millisecond)
	updated := readLockRecord(t, root).HeartbeatAt

	// Rewritten, not necessarily later: the wall clock can be stepped back (a
	// VM resyncing with its host — seen under WSL 2); a lease tolerates it
	// (2× its TTL), and so does this test.
	if updated.Equal(initial) {
		t.Errorf("heartbeat not rewritten: initial=%v updated=%v", initial, updated)
	}
}

// TestLeaseOwnPIDRenews — file already exists with our
// own PID (e.g. we restarted after a crash without removing the
// lockfile). Acquire treats it as ours and renews.
func TestLeaseOwnPIDRenews(t *testing.T) {
	root := t.TempDir()
	writeLockRecord(t, root, lockRecord{
		PID:         os.Getpid(),
		StartedAt:   time.Now().Add(-1 * time.Hour).UTC(),
		LeaseTTL:    "30s",
		HeartbeatAt: time.Now().Add(-5 * time.Second).UTC(),
	})

	l := New(filepath.Join(root, ".lock"))
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire of own-PID lockfile: %v", err)
	}
	_ = l.Release()
}

// --- test helpers ----------------------------------------------------------

func writeLockRecord(t *testing.T, root string, rec lockRecord) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := atomicio.WriteAtomic(filepath.Join(root, ".lock"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readLockRecord(t *testing.T, root string) lockRecord {
	t.Helper()
	data, err := atomicio.ReadAtomic(filepath.Join(root, ".lock"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var rec lockRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return rec
}

// pickLiveOtherPID returns a PID that is alive and not os.Getpid().
// Uses os.Getppid() — the parent process of the test binary is
// reliably alive (it spawned us) on every platform Go supports.
func pickLiveOtherPID(t *testing.T) int {
	t.Helper()
	ppid := os.Getppid()
	if ppid > 0 && ppid != os.Getpid() {
		return ppid
	}
	t.Fatalf("could not determine parent PID for live-other-PID test")
	return 0
}
