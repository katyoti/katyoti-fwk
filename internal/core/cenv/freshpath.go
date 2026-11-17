// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FreshPath is the PATH a process started now gets: on Windows the
// registry's machine PATH then the user's, in their order - where an
// installer writes - followed by the entries of current the registry does not
// hold (what this process added). A running process keeps the PATH it started
// with; this is how it sees a program installed since, ahead of an older entry
// of the same name (the Windows Store's python.exe stub before a Python just
// installed). Outside Windows, current unchanged.
func FreshPath(current string) string {
	return registryFirst(registryPath(), current)
}

// registryFirst joins the registry's entries, in order, then the entries of
// current it does not already hold.
func registryFirst(registry []string, current string) string {
	var parts []string
	add := func(list string) {
		for _, p := range filepath.SplitList(list) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			dup := false
			for _, q := range parts {
				if samePathEntry(p, q) {
					dup = true
					break
				}
			}
			if !dup {
				parts = append(parts, p)
			}
		}
	}
	for _, r := range registry {
		add(r)
	}
	add(current)
	return strings.Join(parts, string(os.PathListSeparator))
}

func samePathEntry(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
