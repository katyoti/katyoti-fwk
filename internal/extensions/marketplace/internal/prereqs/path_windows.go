// SPDX-License-Identifier: Apache-2.0

//go:build windows

package prereqs

import (
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"golang.org/x/sys/windows/registry"
)

// refreshPath gives this process the PATH a new process would get
// (cenv.FreshPath): a program just installed is found, ahead of an older entry
// of its name - the Windows Store's python.exe stub before the Python just
// installed.
func refreshPath() error {
	return os.Setenv("PATH", cenv.FreshPath(os.Getenv("PATH")))
}

// persistUserPath puts dir first on the user's PATH in the registry, ahead of
// the Store's python.exe stub (userPathWith).
func persistUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cur, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	next := userPathWith(cur, dir, func(p string) string {
		if exp, err := registry.ExpandString(p); err == nil {
			return exp
		}
		return p
	})
	if next == cur {
		return nil
	}
	return k.SetExpandStringValue("Path", next)
}
