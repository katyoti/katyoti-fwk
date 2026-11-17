// SPDX-License-Identifier: Apache-2.0

//go:build windows

package orchestration

import (
	"io/fs"
	"os"
	"syscall"
)

// fileLinkCount is how many names the opened file has, read on its handle
// (GetFileInformationByHandle) so it is the file being read.
func fileLinkCount(f *os.File, _ fs.FileInfo) (uint64, error) {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return 0, err
	}
	return uint64(d.NumberOfLinks), nil
}
