// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package appcore

import "os"

// openLogFile opens path for appending; a file held open by another process
// can be moved aside on this platform.
func openLogFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
