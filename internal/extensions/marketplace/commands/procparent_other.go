// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package commands

import (
	"os"
	"strconv"
	"strings"
)

// parentProcess answers the process that started pid and that process's
// program name, read from /proc (Linux); 0 where /proc is not there.
func parentProcess(pid int) (int, string) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, ""
	}
	// pid (comm) state ppid …: comm may hold spaces and parentheses, so the
	// fields are read after its last ')'.
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, ""
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 2 {
		return 0, ""
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil || ppid <= 0 {
		return 0, ""
	}
	comm, _ := os.ReadFile("/proc/" + strconv.Itoa(ppid) + "/comm")
	return ppid, strings.TrimSpace(string(comm))
}
