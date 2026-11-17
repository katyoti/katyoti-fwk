// SPDX-License-Identifier: Apache-2.0

//go:build windows

package oslock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// The locked range lies far past the file's content: a lock on Windows also
// blocks reading the range it covers, and the content (the holder's note) is
// for everyone to read.
const lockOffsetHigh = 0x40000000

func lockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	ol.OffsetHigh = lockOffsetHigh
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrLocked
	}
	if err != nil {
		return wrap(f.Name(), err)
	}
	return nil
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	ol.OffsetHigh = lockOffsetHigh
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
