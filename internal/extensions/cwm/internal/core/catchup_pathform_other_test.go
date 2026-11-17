// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// otherPathForm is a symlink to dir: git answers the resolved folder.
func otherPathForm(t *testing.T, dir string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "linked-project")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("no symlink: %v", err)
	}
	return link
}
