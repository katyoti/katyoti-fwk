// SPDX-License-Identifier: Apache-2.0

//go:build windows

package bash

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setProcGroup configures the command to create a new process group on Windows.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup kills the process tree rooted at the given PID.
// Uses taskkill /T /F which terminates the process and all child processes.
func killProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", pid))
	return kill.Run()
}
