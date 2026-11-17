// SPDX-License-Identifier: Apache-2.0

// Package hubclient provides the find-or-spawn-then-connect-to-ktw-hub
// library used by ktw-gui and ktw-agents (CLI). Replaces the
// hand-rolled `exec.Cmd("ktw-agents", "sc", "ktsp", "stdio")` pattern
// (post-sidecar-norm canonical form; the pre-2026-05-23 shape was
// the top-level `ktsp stdio` verb) with a uniform "connect to the
// universe's hub" surface (per `bus-split-hubclient-library`).
//
// First impl pass scope: the lockfile protocol and `ConnectToHub` in
// `ModeReuse`. Spawn / FindOrSpawn modes land in a follow-up alongside
// the ktw-gui and ktw-agents call-site cutovers.
package hubclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile is the on-disk descriptor written by ktw-hub on startup and
// removed on shutdown. Used by hubclient.ConnectToHub to discover a
// running hub for the active universe — without spawning a new one.
//
// One file per (DataDir/universe). Per (modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md) and the
// XDG layout, DataDir is universe-scoped, so the file lives at
// `<DataDir>/hub/hub.lock`.
type LockFile struct {
	// PID is the OS process ID of the running ktw-hub.
	PID int `json:"pid"`

	// NATSURL is the NATS client URL the hub is listening on (e.g.
	// "nats://127.0.0.1:54321"). hubclient connects with this.
	NATSURL string `json:"nats_url"`

	// HubVersion is the version of the running ktw-hub binary —
	// useful for upgrade-aware reuse decisions.
	HubVersion string `json:"hub_version,omitempty"`

	// StartedAt records when the hub started. UTC.
	StartedAt time.Time `json:"started_at"`
}

// LockFilePath returns the canonical lockfile path for a given DataDir.
// Callers typically pass `application.Env().DataDir()`.
func LockFilePath(dataDir string) string {
	return filepath.Join(dataDir, "hub", "hub.lock")
}

// WriteLockFile atomically writes the lockfile to <dataDir>/hub/hub.lock.
// Creates the parent directory if missing. Atomic via write-temp-then-rename
// so concurrent readers never see a half-written file.
func WriteLockFile(dataDir string, lf LockFile) error {
	if lf.StartedAt.IsZero() {
		lf.StartedAt = time.Now().UTC()
	}
	if lf.PID == 0 {
		lf.PID = os.Getpid()
	}
	if lf.NATSURL == "" {
		return fmt.Errorf("hubclient: WriteLockFile: empty NATSURL")
	}

	path := LockFilePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("hubclient: mkdir %s: %w", filepath.Dir(path), err)
	}

	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("hubclient: marshal lockfile: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("hubclient: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("hubclient: rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// A lockfile the hub has just renamed into place can be held EXCLUSIVELY for a
// moment by another process: on Windows an on-access scanner opens every new
// file before anyone else may, and a reader that arrives in that window gets
// ERROR_SHARING_VIOLATION although the file exists and is complete. Read once
// with no tolerance, that window was reported as a broken lockfile — a test
// that had just watched the file appear failed with "connect to hub: read
// hub.lock: … utilisé par un autre processus" (once in ten runs under load,
// 2026-09-26, e29 H6), and ModeFindOrSpawn returns the same error to ktw-gui
// as fatal. So a transient hold is retried for a bounded time; only a hold
// that persists is reported.
const (
	lockfileReadRetry  = 25 * time.Millisecond
	lockfileReadBudget = 3 * time.Second
)

// ReadLockFile reads the lockfile from <dataDir>/hub/hub.lock.
// Returns (nil, nil) — no error — if the file doesn't exist (the hub
// isn't running). Returns (nil, err) on parse / IO failure.
func ReadLockFile(dataDir string) (*LockFile, error) {
	path := LockFilePath(dataDir)
	data, err := os.ReadFile(path)
	for held := time.Now().Add(lockfileReadBudget); err != nil && isTransientOpenError(err) && time.Now().Before(held); {
		time.Sleep(lockfileReadRetry)
		data, err = os.ReadFile(path)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("hubclient: read %s: %w", path, err)
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("hubclient: parse lockfile %s: %w", path, err)
	}
	return &lf, nil
}

// RemoveLockFile deletes the lockfile. Idempotent — no error if it
// doesn't exist. ktw-hub's serve command calls this on shutdown.
func RemoveLockFile(dataDir string) error {
	path := LockFilePath(dataDir)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("hubclient: remove %s: %w", path, err)
	}
	return nil
}
