// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package hubclient

import (
	"os"
	"syscall"
)

// signalHub sends SIGTERM to the spawned ktw-hub subprocess so it can
// drain cleanly (remove its lockfile, drain NATS subscriptions, etc).
// Stop races SIGTERM against a grace timer, escalating to
// Kill on expiry.
func signalHub(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// isTransientOpenError: POSIX opens do not fail because another process
// holds the file, so no open error is transient here. See process_windows.go
// and the retry note on ReadLockFile.
func isTransientOpenError(error) bool { return false }
