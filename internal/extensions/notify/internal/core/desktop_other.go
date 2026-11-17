// SPDX-License-Identifier: Apache-2.0

//go:build !windows && !darwin

package core

import (
	"fmt"
	"os/exec"
)

// pushDesktopNotification shells out to notify-send, the freedesktop.org
// notification client. Absent on headless hosts — the caller logs and moves on.
func pushDesktopNotification(title, message string) error {
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return fmt.Errorf("notify-send not found: %w", err)
	}
	return exec.Command(bin, title, message).Run()
}
