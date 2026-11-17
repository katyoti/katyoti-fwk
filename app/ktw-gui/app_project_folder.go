// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectFolderOpen opens one of a universe's project folders in the system's
// file explorer — the left panel's « Ouvrir le dossier ». Only a folder the
// universe attaches (its own folder when it attaches none) is opened; dir ""
// is the first of them. Answers an error text, empty on success.
//
// Wails-bound: window.go.main.App.ProjectFolderOpen(universe, dir).
func (a *App) ProjectFolderOpen(universeName, dir string) string {
	target, ok := projectRootMatch(dir, a.projectRootsOf(universeName))
	if !ok {
		return fmt.Sprintf("%q is not a project of universe %q", dir, universeName)
	}
	if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
		return fmt.Sprintf("%s is not a folder", target)
	}
	if err := openWithSystem(target); err != nil {
		return err.Error()
	}
	return ""
}

// projectRootMatch answers the root dir names among roots (dir "" is the
// first), compared cleaned and case-insensitively on Windows' separators; a
// folder outside them is refused, so the binding never opens an arbitrary path.
func projectRootMatch(dir string, roots []string) (string, bool) {
	if len(roots) == 0 {
		return "", false
	}
	if dir == "" {
		return roots[0], true
	}
	want := filepath.Clean(dir)
	for _, r := range roots {
		if strings.EqualFold(filepath.Clean(r), want) {
			return r, true
		}
	}
	return "", false
}
