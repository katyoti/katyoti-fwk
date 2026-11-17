// SPDX-License-Identifier: Apache-2.0

//go:build windows

package hubharness

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// descendants answers the processes pid started, and theirs, from one
// snapshot of the system's processes (Windows keeps a dead parent's id on its
// children, so the snapshot is taken while the hub still lives).
func descendants(pid int) []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	children := map[int][]int{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		children[int(e.ParentProcessID)] = append(children[int(e.ParentProcessID)], int(e.ProcessID))
	}
	return walkTree(pid, children)
}
