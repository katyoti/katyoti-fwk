// SPDX-License-Identifier: Apache-2.0

package seed

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ignored are the patterns of a folder's own .gitignore: what the project
// itself says is not part of it (machine output, test results) is not part of
// its seed either. The common forms only — a name anywhere (`test_results/`,
// `*.log`), a path from the folder (`/build`, `docs/out`), a trailing `/` for
// folders only; a negation (`!…`) is not supported and ignored.
type ignored []ignoreRule

type ignoreRule struct {
	pattern  string
	anchored bool // matches the path from the folder, not a name anywhere
	dirOnly  bool
}

func readIgnore(dir string) ignored {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	var out ignored
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "!") {
			continue
		}
		r := ignoreRule{}
		if strings.HasSuffix(l, "/") {
			r.dirOnly, l = true, strings.TrimSuffix(l, "/")
		}
		if strings.HasPrefix(l, "/") || strings.Contains(l, "/") {
			r.anchored, l = true, strings.TrimPrefix(l, "/")
		}
		r.pattern = l
		out = append(out, r)
	}
	return out
}

// match reports whether rel (slash path from the folder) is ignored.
func (ig ignored) match(rel string, isDir bool) bool {
	for _, r := range ig {
		if r.dirOnly && !isDir {
			continue
		}
		subject := path.Base(rel)
		if r.anchored {
			subject = rel
		}
		if ok, _ := path.Match(r.pattern, subject); ok {
			return true
		}
	}
	return false
}
