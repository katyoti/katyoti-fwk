// SPDX-License-Identifier: Apache-2.0

//go:build tasks_l2 && windows

package l2test

import (
	"fmt"
	"os/exec"
	"syscall"
)

// A clean stop on Windows is a console control event: Go turns CTRL_BREAK into
// os.Interrupt, which `ktw-agents` handles as a first Ctrl+C. The event reaches
// a process group sharing this process's console, so every process the harness
// starts is the leader of its own group (a break to one reaches no other) and
// this process has a console (allocated, hidden, when it has none — a child of
// a process without a console would open a window of its own).

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	user32                       = syscall.NewLazyDLL("user32.dll")
	procAllocConsole             = kernel32.NewProc("AllocConsole")
	procGetConsoleWindow         = kernel32.NewProc("GetConsoleWindow")
	procGenerateConsoleCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
	procShowWindow               = user32.NewProc("ShowWindow")
)

const (
	ctrlBreakEvent = 1
	swHide         = 0
)

// ensureConsole gives this process a console when it has none.
func ensureConsole() {
	if r, _, _ := procAllocConsole.Call(); r != 0 { // there was none: hide the new one
		if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
			_, _, _ = procShowWindow.Call(hwnd, swHide)
		}
	}
}

// ownGroup starts cmd as the leader of a process group of its own.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// interrupt sends the clean-stop signal (CTRL_BREAK) to the group pid leads.
func interrupt(pid int) error {
	if r, _, err := procGenerateConsoleCtrlEvent.Call(ctrlBreakEvent, uintptr(pid)); r == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent(CTRL_BREAK, %d): %v", pid, err)
	}
	return nil
}

const signalName = "CTRL_BREAK (console control event)"
