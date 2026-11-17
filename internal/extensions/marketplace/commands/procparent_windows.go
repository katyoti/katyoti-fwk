// SPDX-License-Identifier: Apache-2.0

//go:build windows

package commands

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// parentProcess answers the process that started pid and that process's
// program name, from one snapshot of the system's processes; 0 when pid is
// not found. Windows keeps a dead parent's id on its children: the caller
// checks the parent still lives.
func parentProcess(pid int) (int, string) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, ""
	}
	defer windows.CloseHandle(snap)
	parent := map[int]int{}
	program := map[int]string{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		id := int(e.ProcessID)
		parent[id] = int(e.ParentProcessID)
		program[id] = filepath.Base(windows.UTF16ToString(e.ExeFile[:]))
	}
	ppid, ok := parent[pid]
	if !ok || ppid <= 0 {
		return 0, ""
	}
	return ppid, program[ppid]
}
