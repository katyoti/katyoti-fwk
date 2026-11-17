// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"fmt"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleWindow   = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcList = kernel32.NewProc("GetConsoleProcessList")
	procShowWindow         = user32.NewProc("ShowWindow")
)

const (
	swHide = 0
	swShow = 5
)

// ownConsole is the console window katyoti alone holds — it was started from
// its shortcut, not from a terminal the person types in — or 0.
func ownConsole() uintptr {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return 0
	}
	var pids [2]uint32
	n, _, _ := procGetConsoleProcList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		return 0
	}
	return hwnd
}

// hideOwnConsole hides the console katyoti was started in from its shortcut:
// it stays (ktw-gui and the processes it starts share it instead of each
// opening a window) but is not shown. The returned function shows it again
// with err and waits for Enter, so a failed launch is not silent; it does
// nothing when the console was not hidden or err is nil.
func hideOwnConsole(errw io.Writer) func(err error) {
	hwnd := ownConsole()
	if hwnd == 0 {
		return func(error) {}
	}
	procShowWindow.Call(hwnd, swHide)
	return func(err error) {
		if err == nil {
			return
		}
		procShowWindow.Call(hwnd, swShow)
		fmt.Fprintf(errw, "katyoti: %v\npress Enter to close", err)
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
	}
}
