// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readFileImpl(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func TestLockFilePath_Canonical(t *testing.T) {
	got := LockFilePath("/tmp/data")
	want := filepath.Join("/tmp/data", "hub", "hub.lock")
	if got != want {
		t.Errorf("LockFilePath = %q, want %q", got, want)
	}
}

func TestWriteReadLockFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := LockFile{
		NATSURL:    "nats://127.0.0.1:54321",
		HubVersion: "1.0.0",
	}
	if err := WriteLockFile(dir, in); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	got, err := ReadLockFile(dir)
	if err != nil {
		t.Fatalf("ReadLockFile: %v", err)
	}
	if got == nil {
		t.Fatal("ReadLockFile returned nil")
	}
	if got.NATSURL != in.NATSURL {
		t.Errorf("NATSURL = %q, want %q", got.NATSURL, in.NATSURL)
	}
	if got.HubVersion != in.HubVersion {
		t.Errorf("HubVersion = %q, want %q", got.HubVersion, in.HubVersion)
	}
	if got.PID == 0 {
		t.Error("PID = 0, expected WriteLockFile to default to os.Getpid()")
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt is zero, expected WriteLockFile to default to now")
	}
	if time.Since(got.StartedAt) > 5*time.Second {
		t.Errorf("StartedAt = %v, expected ≈ now", got.StartedAt)
	}
}

func TestWriteLockFile_RejectsEmptyURL(t *testing.T) {
	dir := t.TempDir()
	err := WriteLockFile(dir, LockFile{NATSURL: ""})
	if err == nil {
		t.Error("expected error for empty NATSURL, got nil")
	}
}

func TestReadLockFile_AbsentReturnsNilNil(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadLockFile(dir)
	if err != nil {
		t.Errorf("ReadLockFile on missing dir: err = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("ReadLockFile on missing dir: got = %+v, want nil", got)
	}
}

func TestRemoveLockFile_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Remove when absent — must not error.
	if err := RemoveLockFile(dir); err != nil {
		t.Errorf("first RemoveLockFile: %v", err)
	}

	// Write, then remove — must succeed.
	if err := WriteLockFile(dir, LockFile{NATSURL: "nats://x.test"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLockFile(dir); err != nil {
		t.Errorf("second RemoveLockFile: %v", err)
	}

	// Verify gone.
	got, err := ReadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("after RemoveLockFile: ReadLockFile returned %+v", got)
	}
}

func TestWriteLockFile_AtomicSwap(t *testing.T) {
	// Verify the .tmp file is gone after a successful write — i.e. the
	// rename happened, not a partial-write.
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{NATSURL: "nats://x.test"}); err != nil {
		t.Fatal(err)
	}

	tmp := LockFilePath(dir) + ".tmp"
	got, err := ReadLockFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("lockfile missing after write")
	}

	// .tmp must not exist.
	if _, err := readFile(t, tmp); err == nil {
		t.Errorf("temp file %s still present after rename", tmp)
	}
}

func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return readFileImpl(path)
}
