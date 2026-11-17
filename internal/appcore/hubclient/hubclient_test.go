// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestConnectToHub_RequiresDataDir(t *testing.T) {
	_, err := ConnectToHub(context.Background(), ConnectOptions{})
	if err == nil {
		t.Fatal("expected error for empty DataDir, got nil")
	}
}

func TestConnectToHub_ModeReuse_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	_, err := ConnectToHub(context.Background(), ConnectOptions{
		DataDir: dir,
		Mode:    ModeReuse,
	})
	if !errors.Is(err, ErrNoHub) {
		t.Errorf("expected ErrNoHub, got %v", err)
	}
}

func TestConnectToHub_ModeReuse_StaleLockfile(t *testing.T) {
	dir := t.TempDir()

	// Write a lockfile pointing at a definitely-dead PID. We use a PID
	// far above any reasonable running process — and processAlive
	// checks via os.FindProcess + Signal(0) (Unix) / FindProcess (Win).
	// On Windows, FindProcess always returns a non-nil handle even
	// for dead PIDs — so we skip there since the test would falsely
	// succeed.
	if !canDetectStalePIDs() {
		t.Skip("platform doesn't reliably detect stale PIDs")
	}

	if err := WriteLockFile(dir, LockFile{
		PID:     999999, // very unlikely to be live
		NATSURL: "nats://127.0.0.1:1",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := ConnectToHub(context.Background(), ConnectOptions{
		DataDir: dir,
		Mode:    ModeReuse,
	})
	if !errors.Is(err, ErrStaleLockfile) {
		t.Errorf("expected ErrStaleLockfile, got %v", err)
	}
	// The reader removes it: the next one finds no hub, not a dead one.
	if lf, _ := ReadLockFile(dir); lf != nil {
		t.Errorf("a lockfile naming a dead pid was left in place: %+v", lf)
	}
}

func TestConnectToHub_ModeSpawn_FailsCleanlyWhenBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ConnectToHub(context.Background(), ConnectOptions{
		DataDir:       dir,
		Mode:          ModeSpawn,
		HubBinaryPath: "/path/that/does/not/exist/ktw-hub",
	})
	if err == nil {
		t.Fatal("expected error for missing hub binary, got nil")
	}
	if !errors.Is(err, ErrHubBinaryNotFound) {
		t.Errorf("expected ErrHubBinaryNotFound, got %v", err)
	}
}

func TestConnectToHub_ModeFindOrSpawn_FailsCleanlyWhenBinaryMissing(t *testing.T) {
	// FindOrSpawn first tries ModeReuse (which fails with ErrNoHub
	// because the temp dir is empty), then falls back to ModeSpawn,
	// which fails because the binary doesn't exist.
	dir := t.TempDir()
	_, err := ConnectToHub(context.Background(), ConnectOptions{
		DataDir:       dir,
		Mode:          ModeFindOrSpawn,
		HubBinaryPath: "/path/that/does/not/exist/ktw-hub",
	})
	if err == nil {
		t.Fatal("expected error for missing hub binary on FindOrSpawn fallback, got nil")
	}
	if !errors.Is(err, ErrHubBinaryNotFound) {
		t.Errorf("expected ErrHubBinaryNotFound, got %v", err)
	}
}

func TestProcessAlive_OwnPID(t *testing.T) {
	if !ProcessAlive(os.Getpid()) {
		t.Error("ProcessAlive(os.Getpid()) = false, want true")
	}
}

func TestProcessAlive_ZeroPID(t *testing.T) {
	if ProcessAlive(0) {
		t.Error("ProcessAlive(0) = true, want false")
	}
}

// canDetectStalePIDs reports whether this platform can reliably detect
// stale PIDs via os.FindProcess + signal probe. On Windows, FindProcess
// returns a non-nil handle for any PID without verifying liveness, so
// the test skip is correct.
func canDetectStalePIDs() bool {
	return canDetectStalePIDsImpl()
}
