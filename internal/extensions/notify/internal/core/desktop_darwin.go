// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os/exec"
	"strings"
)

// pushDesktopNotification posts to the macOS Notification Center via
// osascript — no cgo, no external dependency, available on every macOS host.
func pushDesktopNotification(title, message string) error {
	script := "display notification " + quoteAppleScript(message) +
		" with title " + quoteAppleScript(title)
	return exec.Command("osascript", "-e", script).Run()
}

// quoteAppleScript wraps s as an AppleScript string literal, escaping the
// backslashes and double quotes that would otherwise break out of it.
func quoteAppleScript(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}
