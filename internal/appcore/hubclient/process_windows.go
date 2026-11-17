// SPDX-License-Identifier: Apache-2.0

//go:build windows

package hubclient

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// signalHub on Windows: SIGTERM doesn't exist for child processes in
// the Unix sense, and CTRL_BREAK_EVENT requires creating the process
// in a new process group at spawn time. For now we Kill outright —
// the hub's deferred lockfile cleanup won't run, but processAlive
// (above) detects dead PIDs reliably so subsequent ConnectToHub calls
// see ErrStaleLockfile and the FindOrSpawn fallback re-spawns.
//
// (CTRL_BREAK_EVENT support would require setting
// SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP} on the spawn
// in connectSpawn — left as a future refinement.)
func signalHub(p *os.Process) error {
	return p.Kill()
}

// isTransientOpenError reports a file open refused because another process
// holds the file exclusively for the moment — a sharing or lock violation.
// See the retry note on ReadLockFile: an on-access scanner holds a file the
// hub has just renamed into place, and the reader must wait, not fail.
func isTransientOpenError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
