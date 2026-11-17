// SPDX-License-Identifier: Apache-2.0

//go:build windows

package server

import (
	"errors"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code Windows reports for a process that has not
// exited (STILL_ACTIVE).
const stillActive = 259

// processRunning reports whether the process has not exited. Only a pid no
// process has (ERROR_INVALID_PARAMETER) or one that exited (its exit code) is
// dead: a process that cannot be opened for any other reason - rights, a
// protected process, a system under load - exists, since taking a live
// process for a dead one replaces what still runs.
func processRunning(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true // cannot tell: not taken for dead
	}
	return code == stillActive
}
