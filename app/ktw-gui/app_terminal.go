// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// TerminalInfo describes a shell discovered on the host. The frontend renders
// the list returned by GetInstalledTerminals and calls LaunchTerminal(ID, cwd)
// when the user clicks an entry.
//
// Icon is a Tabler icon name (e.g. "terminal-2", "brand-windows", "git-branch")
// that the frontend maps to a `@tabler/icons-react` component.
type TerminalInfo struct {
	ID    string `json:"id"`    // stable identifier (cmd, powershell, pwsh, git-bash, wsl, bash, zsh)
	Label string `json:"label"` // human-friendly name shown to the user
	Icon  string `json:"icon"`  // Tabler icon name (no IconXxx prefix)
	Path  string `json:"path"`  // resolved absolute path to the shell binary
}

// GetInstalledTerminals scans the host for known shells and returns the ones
// that are actually installed. Empty slice is a valid result (e.g. minimal
// container) — frontend should show an "no terminals detected" empty state.
//
// Wails-bound: window.go.main.App.GetInstalledTerminals().
func (a *App) GetInstalledTerminals() []TerminalInfo {
	var found []TerminalInfo

	// Helper: append iff path resolves and exists.
	add := func(id, label, icon string, candidates ...string) {
		for _, c := range candidates {
			if c == "" {
				continue
			}
			// PATH lookup.
			if !filepath.IsAbs(c) {
				if resolved, err := exec.LookPath(c); err == nil {
					found = append(found, TerminalInfo{ID: id, Label: label, Icon: icon, Path: resolved})
					return
				}
				continue
			}
			// Absolute path probe.
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				found = append(found, TerminalInfo{ID: id, Label: label, Icon: icon, Path: c})
				return
			}
		}
	}

	switch runtime.GOOS {
	case "windows":
		add("cmd", "Command Prompt", "brand-windows", "cmd.exe")
		add("powershell", "Windows PowerShell", "terminal-2", "powershell.exe")
		add("pwsh", "PowerShell 7+", "terminal-2", "pwsh.exe")
		add("git-bash", "Git Bash", "git-branch",
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
		)
		add("wsl", "WSL", "brand-debian", "wsl.exe")
	default:
		// macOS/Linux best-effort. Phase 1 targets Windows; this branch lets
		// the GUI still launch *something* during dev on a non-Windows box.
		add("bash", "Bash", "terminal-2", "bash")
		add("zsh", "Zsh", "terminal-2", "zsh")
		add("fish", "Fish", "terminal-2", "fish")
	}

	return found
}

// LaunchTerminal spawns the shell identified by id as a detached external
// window with cwd as the working directory. On Windows the new window is
// created with CREATE_NEW_CONSOLE so it doesn't share ktw-gui's hidden
// console. The spawned process is intentionally NOT tracked — the OS owns
// it once it's launched.
//
// Empty cwd falls back to ktw-gui's own cwd. If the resolved cwd doesn't
// exist, the call returns an error before spawning so the user gets a
// clear message instead of a flashed-and-gone window.
//
// Wails-bound: window.go.main.App.LaunchTerminal(id, cwd).
func (a *App) LaunchTerminal(id string, cwd string) error {
	terminals := a.GetInstalledTerminals()
	var t *TerminalInfo
	for i := range terminals {
		if terminals[i].ID == id {
			t = &terminals[i]
			break
		}
	}
	if t == nil {
		return fmt.Errorf("terminal %q is not installed", id)
	}

	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd != "" {
		if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
			return fmt.Errorf("workspace %q is not a directory", cwd)
		}
	}

	return launchTerminalWindow(t, cwd)
}
