// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// copyIn runs one `copy` entry: a file to its destination, or a folder's
// selected files under it. It answers how many files it copied; a folder that
// selects none is refused (a selector that matches nothing is a mistake).
func (e *Engine) copyIn(cp api.Copy) (int, error) {
	return e.copyInto(cp, e.RT.UniverseHome())
}

// copyInto is copyIn into the universe whose config root is home.
func (e *Engine) copyInto(cp api.Copy, home string) (int, error) {
	src := e.source(cp.From)
	dst := filepath.Join(home, filepath.FromSlash(cp.To))
	info, err := os.Stat(src)
	if err != nil {
		if cp.Optional && errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		if len(cp.Select) > 0 || len(cp.Exclude) > 0 {
			return 0, fmt.Errorf("select and exclude apply to a folder, %s is a file", cp.From)
		}
		if cp.To == "" || strings.HasSuffix(filepath.ToSlash(cp.To), "/") {
			dst = filepath.Join(dst, filepath.Base(src))
		}
		return 1, copyFile(src, dst)
	}
	n := 0
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		if (len(cp.Select) > 0 && !matchAny(cp.Select, rel)) || matchAny(cp.Exclude, rel) {
			return nil
		}
		n++
		return copyFile(p, filepath.Join(dst, filepath.FromSlash(rel)))
	})
	if err == nil && n == 0 {
		err = fmt.Errorf("no file selected in %s", cp.From)
	}
	return n, err
}

// source resolves a copy's from: absolute, under the home folder (`~/`), else
// relative to the case folder.
func (e *Engine) source(from string) string {
	if rest, ok := strings.CutPrefix(filepath.ToSlash(from), "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, filepath.FromSlash(rest))
		}
	}
	if filepath.IsAbs(from) {
		return from
	}
	return filepath.Join(e.Case.Dir, filepath.FromSlash(from))
}

// matchAny is whether rel (slash form) matches a glob: on the whole relative
// path, or on the file name for a glob without a `/`.
func matchAny(globs []string, rel string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, rel); ok {
			return true
		}
		if !strings.Contains(g, "/") {
			if ok, _ := path.Match(g, path.Base(rel)); ok {
				return true
			}
		}
	}
	return false
}
