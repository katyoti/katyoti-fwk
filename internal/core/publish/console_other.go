// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package publish

import (
	"os/exec"
	"syscall"
)

// NoConsoleWindow is a no-op off Windows: a child process opens no window.
func NoConsoleWindow(*exec.Cmd) {}

// Detach makes cmd outlive the process that starts it: its own session, so
// the terminal's SIGHUP never reaches it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
