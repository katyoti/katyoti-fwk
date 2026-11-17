// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package core

import (
	"os"
	"strconv"
)

// processPath is a running process's executable path; "" when unknown.
func processPath(pid int) string {
	if pid <= 0 {
		return ""
	}
	p, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return ""
	}
	return p
}
