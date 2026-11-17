// SPDX-License-Identifier: Apache-2.0

//go:build windows

package atomicio

import (
	"os"

	"golang.org/x/sys/windows"
)

// syncReplaced is the step that makes a replace durable: on Windows the
// renamed file is flushed through to disk (FlushFileBuffers writes its data
// and its metadata, the new name included). The handle shares read, write and
// delete, like the readers (readShared), so it never blocks one. A variable
// so a test sees it run after every rename.
var syncReplaced = func(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer windows.CloseHandle(h)
	if err := windows.FlushFileBuffers(h); err != nil {
		return &os.PathError{Op: "flush", Path: path, Err: err}
	}
	return nil
}
