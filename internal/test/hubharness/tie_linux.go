// SPDX-License-Identifier: Apache-2.0

//go:build linux

package hubharness

import (
	"os/exec"
	"syscall"
)

func tieSelf() error { return nil }

// tieCmd gives the process its own group (the harness's cleanup ends it whole)
// and SIGKILL when the test process dies.
func tieCmd(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
