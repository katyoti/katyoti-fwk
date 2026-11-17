// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package atomicio

import (
	"os"
	"path/filepath"
)

// syncReplaced is the step that makes a replace durable: on POSIX the rename
// is an entry of the parent directory, written to disk when the directory is
// fsynced. A variable so a test sees it run after every rename.
var syncReplaced = func(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
