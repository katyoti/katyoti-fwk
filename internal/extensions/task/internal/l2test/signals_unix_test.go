// SPDX-License-Identifier: Apache-2.0

//go:build tasks_l2 && !windows

package l2test

import (
	"os/exec"
	"syscall"
)

// ensureConsole: nothing to do outside Windows.
func ensureConsole() {}

// ownGroup starts cmd in a process group of its own.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interrupt sends SIGINT — a Ctrl+C — to pid.
func interrupt(pid int) error { return syscall.Kill(pid, syscall.SIGINT) }

const signalName = "SIGINT"
