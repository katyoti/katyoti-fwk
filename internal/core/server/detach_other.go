// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package server

import (
	"os/exec"
	"syscall"
)

// applyDetach starts the child in a new session so it survives the parent's
// terminal closing and doesn't receive the parent's terminal signals.
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
