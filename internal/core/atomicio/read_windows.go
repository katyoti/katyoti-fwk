// SPDX-License-Identifier: Apache-2.0

//go:build windows

package atomicio

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// readShared reads path with FILE_SHARE_DELETE, which os.ReadFile does not
// ask for. Without it a reader holding the file blocks the writer's rename
// onto it: readers polling an object starve its writes (seen with four
// readers: a write could not land within the retry budget). With it the
// rename replaces the file while the reader keeps the bytes it opened.
func readShared(path string) ([]byte, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if err == windows.ERROR_FILE_NOT_FOUND || err == windows.ERROR_PATH_NOT_FOUND {
			return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	return io.ReadAll(f)
}
