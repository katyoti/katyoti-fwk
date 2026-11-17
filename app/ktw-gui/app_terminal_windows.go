// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"syscall"
)

// CREATE_NEW_CONSOLE — the new process gets its own console window instead
// of sharing ktw-gui's (hidden) console. Without this the spawned shell
// inherits the parent's GUI subsystem console and the window never appears.
//
// DETACHED_PROCESS would *also* skip console inheritance, but then cmd.exe /
// powershell won't have a TTY and `Get-ChildItem` etc. will fail. NEW_CONSOLE
// gives them a fresh console window — exactly what we want.
const createNewConsole = 0x00000010

func launchTerminalWindow(t *TerminalInfo, cwd string) error {
	cmd := exec.Command(t.Path)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewConsole,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", t.Label, err)
	}
	// Detach: we deliberately don't Wait — the OS owns the process now and
	// will reap it when the user closes the window.
	go func() { _ = cmd.Wait() }()
	return nil
}
