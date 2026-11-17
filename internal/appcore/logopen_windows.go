// SPDX-License-Identifier: Apache-2.0

//go:build windows

package appcore

import (
	"os"

	"golang.org/x/sys/windows"
)

// openLogFile opens path for appending with FILE_SHARE_DELETE, which
// os.OpenFile does not ask for: without it a process holding the log (the
// long-lived core) makes every other process's rotation fail. With it the log
// moves aside while others hold it, and they reopen the path at their next
// recheck.
func openLogFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.FILE_APPEND_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
