// SPDX-License-Identifier: Apache-2.0

//go:build windows

package hubclient

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processInfo answers pid's program name and the process that started it,
// from one snapshot of the system's processes; ok false when pid is not
// found. Windows keeps a dead parent's id on its children: a caller naming
// the parent checks it still lives.
func processInfo(pid int) (program string, ppid int, ok bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", 0, false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if int(e.ProcessID) == pid {
			return filepath.Base(windows.UTF16ToString(e.ExeFile[:])), int(e.ParentProcessID), true
		}
	}
	return "", 0, false
}
