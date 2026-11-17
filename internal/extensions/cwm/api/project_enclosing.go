// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// EnclosingProject is the project or Git repository dir lies inside - the
// nearest folder above it holding a project object or a .git - or "" when
// none (: no repository ever sits inside another
// : a project never holds projects). dir itself is not
// judged: a project is its own root.
func EnclosingProject(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for p := filepath.Dir(abs); p != abs; abs, p = p, filepath.Dir(p) {
		for _, mark := range []string{projectfile.FileName, ".git"} {
			if _, err := os.Stat(filepath.Join(p, mark)); err == nil {
				return p
			}
		}
	}
	return ""
}
