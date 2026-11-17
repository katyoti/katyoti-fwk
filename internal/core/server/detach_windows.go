// SPDX-License-Identifier: Apache-2.0

package server

import (
	"os/exec"
	"syscall"
)

// applyDetach puts the child in its own process group so a Ctrl-C in the
// parent's console doesn't tear the background server down with it.
func applyDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
