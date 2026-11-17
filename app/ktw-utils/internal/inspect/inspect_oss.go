// SPDX-License-Identifier: Apache-2.0

// Package inspect provides a live KTSP infrastructure monitor TUI. The
// published build leaves the monitor out (it needs a terminal UI library the
// published build does not carry): Run answers so.
package inspect

import (
	"context"
	"errors"
)

// Run answers that the live inspector is not part of this build.
func Run(ctx context.Context, wsURL, sessionID string) error {
	return errors.New("the live inspector is not part of this build")
}
