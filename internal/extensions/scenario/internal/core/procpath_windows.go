// SPDX-License-Identifier: Apache-2.0

//go:build windows

package core

import "golang.org/x/sys/windows"

// processPath is a running process's executable path; "" when unknown. It
// asks Windows directly (QueryFullProcessImageName on a limited-information
// handle): no PowerShell started per process, which cost a second or more for
// every hub lock the leak guard judged.
func processPath(pid int) string {
	if pid <= 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
