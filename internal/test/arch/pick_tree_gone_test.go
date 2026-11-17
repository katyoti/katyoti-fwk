// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The generic picker replaced the pick tree as the one way to choose items
// ; deleted the pick-tree node - its kind, its
// GUI renderer, its model - and it does not come back: no code, page or
// panel names it.
var pickTree = regexp.MustCompile(`pick-tree|PickTree|pickTree`)

func TestThePickTreeStaysDeleted(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	walk := func(dir string, keep func(rel string) bool) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") || !keep(rel) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			if m := pickTree.FindString(string(raw)); m != "" {
				t.Errorf("%s names %q - the pick tree was deleted: use the generic picker", rel, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	code := func(rel string) bool {
		return strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".tsx") || strings.HasSuffix(rel, ".json")
	}
	walk("app/ktw-display-lib/src", code)
	walk("app/ktw-gui/frontend/src", code)
	walk("internal/core/render", code)
	panels := func(rel string) bool {
		return strings.Contains(rel, "/object-panels/") && strings.HasSuffix(rel, ".yaml")
	}
	walk("app", panels)
	walk("internal/extensions", panels)
	if checked < 200 {
		t.Fatalf("checked %d files: the roots are wrong", checked)
	}
}
