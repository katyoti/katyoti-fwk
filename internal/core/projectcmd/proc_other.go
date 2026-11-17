// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package projectcmd

import (
	"os/exec"
	"syscall"
)

// procTree runs the command in its own process group, so a stopped command
// takes the whole group with it: bash and the processes it started.
type procTree struct{}

func newProcTree(cmd *exec.Cmd) *procTree {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	return &procTree{}
}

func (*procTree) started(*exec.Cmd) {}
func (*procTree) close()            {}
