// SPDX-License-Identifier: Apache-2.0

// Package shortcut gives each katyoti installation its own Start-menu and
// Desktop shortcut, named after it — « KaTyoti northwind-demo 1.004 » — so several
// versions and solutions sit side by side and each opens its own. `katyoti
// setup` makes it (the installers do not), `katyoti instance remove` removes it.
//
// Windows only: elsewhere Make and Remove do nothing.
package shortcut

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// DirEnv puts the shortcuts in one folder instead of the Start menu and the
// Desktop (tests: a shortcut is a file of the user's session).
const DirEnv = "KTW_SHORTCUT_DIR"

// Name is the shortcut's name for the installation named installation:
// « KaTyoti <installation> », the installation's own spelling when it already
// starts with katyoti; no character a file name cannot hold.
func Name(installation string) string {
	n := strings.TrimSpace(installation)
	if rest, ok := cutPrefixFold(n, "katyoti"); ok {
		n = "KaTyoti" + rest
	} else {
		n = "KaTyoti " + n
	}
	n = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, n)
	return strings.TrimSpace(n)
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// Make writes the installation's shortcuts: target (its katyoti) started
// minimized in the classic console host — katyoti is a console program that
// waits on the GUI and hides the console it holds — in its folder, with icon's
// first icon (ktw-gui's). Again, it rewrites them.
func Make(installation, target, icon string) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	dirs, err := folders()
	if err != nil {
		return nil, err
	}
	var made []string
	for _, d := range dirs {
		lnk := filepath.Join(d, Name(installation)+".lnk")
		exe, args := host(target)
		ps := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut(%s); $s.TargetPath=%s; $s.Arguments=%s; $s.WorkingDirectory=%s; $s.WindowStyle=7; $s.IconLocation=%s; $s.Description=%s; $s.Save()`,
			psQuote(lnk), psQuote(exe), psQuote(args), psQuote(filepath.Dir(target)), psQuote(icon+",0"), psQuote("Open KaTyoti ("+installation+")"))
		mk := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
		publish.NoConsoleWindow(mk) // the hub, which removes and may make them, has no console
		if out, err := mk.CombinedOutput(); err != nil {
			return made, fmt.Errorf("shortcut %s: %v: %s", lnk, err, strings.TrimSpace(string(out)))
		}
		made = append(made, lnk)
	}
	return made, nil
}

// host is what the shortcut runs for target: conhost.exe with target, because
// Windows Terminal, when it is the default terminal, opens a window for a
// console program whatever the shortcut's window style; target itself when
// conhost.exe is not there.
func host(target string) (exe, args string) {
	conhost := filepath.Join(os.Getenv("SystemRoot"), "System32", "conhost.exe")
	if _, err := os.Stat(conhost); err != nil {
		return target, ""
	}
	return conhost, `"` + target + `"`
}

// Remove deletes the installation's shortcuts.
func Remove(installation string) {
	if runtime.GOOS != "windows" {
		return
	}
	dirs, err := folders()
	if err != nil {
		return
	}
	for _, d := range dirs {
		_ = os.Remove(filepath.Join(d, Name(installation)+".lnk"))
	}
}

// folders are where the shortcuts go: the user's Start-menu programs and
// Desktop (as Windows names them: a Desktop moved to OneDrive included), or
// $KTW_SHORTCUT_DIR.
func folders() ([]string, error) {
	if d := os.Getenv(DirEnv); d != "" {
		return []string{d}, os.MkdirAll(d, 0o755)
	}
	ps := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`[Environment]::GetFolderPath('Programs'); [Environment]::GetFolderPath('Desktop')`)
	publish.NoConsoleWindow(ps)
	out, err := ps.Output()
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			dirs = append(dirs, l)
		}
	}
	return dirs, nil
}

// psQuote is s as a PowerShell single-quoted string.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
