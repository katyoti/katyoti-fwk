// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package hubclient

import (
	"os"
	"strconv"
	"strings"
)

// processInfo answers pid's program name and the process that started it,
// read from /proc (Linux); ok false where /proc does not show pid.
func processInfo(pid int) (program string, ppid int, ok bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", 0, false
	}
	// pid (comm) state ppid …: comm may hold spaces and parentheses, so the
	// fields are read after its last ')'.
	s := string(data)
	open, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return "", 0, false
	}
	fields := strings.Fields(s[end+1:])
	if len(fields) < 2 {
		return "", 0, false
	}
	ppid, _ = strconv.Atoi(fields[1])
	return s[open+1 : end], ppid, true
}
