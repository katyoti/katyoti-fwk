// SPDX-License-Identifier: Apache-2.0

//go:build windows

package atomicio

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileRenameInfoEx is FILE_RENAME_INFO with its Flags member (FileRenameInfoEx).
type fileRenameInfoEx struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// replaceFile renames tmp onto path with POSIX semantics (Windows 10 1607+,
// NTFS): the name is replaced even while readers hold the old file open —
// they keep reading the bytes they opened. os.Rename (MoveFileEx) refuses a
// destination with an open handle, so readers polling an object starved its
// writes. Where POSIX semantics are not supported it falls back to MoveFileEx, as
// os.Rename does, with write-through.
func replaceFile(tmp, path string) error {
	if err := posixRename(tmp, path); err == nil {
		return nil
	}
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// os.Rename with write-through: the move is on disk when it returns.
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: tmp, New: path, Err: err}
	}
	return nil
}

func posixRename(tmp, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	src, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(src, windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	name, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	nameBytes := (len(name) - 1) * 2 // without the terminator
	var info fileRenameInfoEx
	size := int(unsafe.Offsetof(info.FileName)) + len(name)*2
	buf := make([]byte, size)
	p := (*fileRenameInfoEx)(unsafe.Pointer(&buf[0]))
	p.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	p.FileNameLength = uint32(nameBytes)
	copy(unsafe.Slice(&p.FileName[0], len(name)), name)
	return windows.SetFileInformationByHandle(h, windows.FileRenameInfoEx, &buf[0], uint32(size))
}
