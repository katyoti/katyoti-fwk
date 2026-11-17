// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import "errors"

// captureOwnWindowPNG has no native implementation outside Windows yet: the
// feedback form opens without a screenshot.
func captureOwnWindowPNG() ([]byte, error) {
	return nil, errors.New("window capture is not supported on this platform")
}
