// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import (
	"fmt"
	"os/exec"
)

// launchTerminalWindow on Unix is a best-effort: we try common terminal
// emulators (gnome-terminal, konsole, xterm) and ask them to host the shell
// in cwd. Phase 1 of the launcher targets Windows; this branch keeps the
// build green and provides a usable fallback for dev on macOS/Linux.
func launchTerminalWindow(t *TerminalInfo, cwd string) error {
	emulators := []struct {
		bin  string
		args func(shell, cwd string) []string
	}{
		{"gnome-terminal", func(s, d string) []string { return []string{"--working-directory", d, "--", s} }},
		{"konsole", func(s, d string) []string { return []string{"--workdir", d, "-e", s} }},
		{"xterm", func(s, d string) []string { return []string{"-e", "cd " + d + " && " + s} }},
		{"open", func(s, d string) []string { return []string{"-a", "Terminal", d} }}, // macOS
	}
	for _, e := range emulators {
		if path, err := exec.LookPath(e.bin); err == nil {
			cmd := exec.Command(path, e.args(t.Path, cwd)...)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("start %s via %s: %w", t.Label, e.bin, err)
			}
			go func() { _ = cmd.Wait() }()
			return nil
		}
	}
	return fmt.Errorf("no terminal emulator found (tried gnome-terminal/konsole/xterm/open)")
}
