// SPDX-License-Identifier: Apache-2.0

//go:build windows

package publish

import (
	"os/exec"
	"syscall"
)

const (
	// createNoWindow is CREATE_NO_WINDOW: the child is a console application that
	// gets a console, but no window.
	createNoWindow = 0x08000000
	// createNewProcessGroup is CREATE_NEW_PROCESS_GROUP: no console signal of
	// the starter's (Ctrl+C, Ctrl+Break) reaches the child.
	createNewProcessGroup = 0x00000200
)

// NoConsoleWindow starts cmd with no console window. A background process —
// a hub started by a client, a sidecar started by the hub, a supervised
// external process — is a console application; started from a process that
// has no console of its own (a GUI), Windows opens a new terminal window for
// each one. Its standard streams still work: they are pipes or the null
// device, never the window. The child's console is hidden, and every console
// program it starts in turn inherits it: no window down the tree.
func NoConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// Detach makes cmd outlive the process that starts it: its own process group,
// so no console signal of its starter reaches it, and a hidden console of its
// own (NoConsoleWindow), so closing the starter's terminal does not end it.
//
// Never DETACHED_PROCESS: a process with no console at all gives each console
// program it starts a new terminal window — a hub's manifest probe of every
// member, after each build, flashed one window per sidecar.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
	NoConsoleWindow(cmd)
}
