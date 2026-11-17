// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package prereqs

import "os"

// refreshPath: on macOS and Linux an installer puts its program in a folder
// already on PATH (brew's, /usr/bin, ~/.local/bin, ~/.bun/bin); the folders a
// user-scope installer uses are added when they exist.
func refreshPath() error {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	var fresh []string
	for _, d := range []string{home + "/.local/bin", home + "/.bun/bin", "/opt/homebrew/bin", "/home/linuxbrew/.linuxbrew/bin"} {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			fresh = append(fresh, d)
		}
	}
	return os.Setenv("PATH", mergePath(os.Getenv("PATH"), fresh...))
}

// persistUserPath: a shell profile is the person's to edit, so nothing is
// written there; the folder is on this process's PATH, which the checks and
// the sidecars this hub starts read.
func persistUserPath(string) error {
	return nil
}
