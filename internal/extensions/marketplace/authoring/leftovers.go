// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
)

// Leftovers are the swap leftovers (selfupdate.IsLeftover: `*.old-*`,
// `*.new-*`, `*.old`) found anywhere in a published tree, relative to root and
// sorted. A published tree holds only what is published: every file of it is
// synced or served to each subscriber.
func Leftovers(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() && selfupdate.IsLeftover(d.Name()) {
			rel, _ := filepath.Rel(root, path)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

// SweepLeftovers removes every swap leftover from a published tree and returns
// what it removed (relative to root). A leftover it cannot remove is an error:
// published, it would reach every subscriber.
func SweepLeftovers(root string) ([]string, error) {
	found, err := Leftovers(root)
	if err != nil {
		return nil, err
	}
	for _, rel := range found {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return found, nil
}
