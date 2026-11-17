// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package bash

import (
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to run in its own process group
// so we can kill the entire tree on timeout.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group rooted at the given PID.
func killProcessGroup(pid int) error {
	// Negative PID sends signal to the entire process group.
	return syscall.Kill(-pid, syscall.SIGKILL)
}
