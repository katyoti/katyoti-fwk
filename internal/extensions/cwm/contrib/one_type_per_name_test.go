// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ONE PACK DECLARES ONE TYPE NAME. The service keeps one type per name, the
// last one loaded, so two packs declaring `goal` do not fail: one silently
// replaces the other, and which one depends on which sidecars a universe runs
// and in what order. That is how goals lost their lifecycle and workflow in a
// universe where a bare legacy `goal` won. A pack that adds to a type it does
// not own attaches a capability to it (`attach: {types: [...]}`).
func TestNoTwoPacksDeclareOneObjectType(t *testing.T) {
	root := repoRootForTypes(t)
	var dirs []string
	for _, pattern := range []string{"app/*/embed/cwm/object-types", "internal/extensions/*/contrib/embed/cwm/object-types"} {
		m, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, m...)
	}
	by := map[string][]string{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			rel, _ := filepath.Rel(root, filepath.Join(d, e.Name()))
			name := strings.TrimSuffix(e.Name(), ".yaml")
			by[name] = append(by[name], filepath.ToSlash(rel))
		}
	}
	if len(by) < 30 {
		t.Fatalf("only %d object types found under %d pack folders — the scan is not working", len(by), len(dirs))
	}
	var dup []string
	for name, files := range by {
		if len(files) > 1 {
			sort.Strings(files)
			dup = append(dup, name+": "+strings.Join(files, ", "))
		}
	}
	sort.Strings(dup)
	if len(dup) > 0 {
		t.Errorf("an object type declared by more than one pack — the one loaded last silently wins:\n  %s", strings.Join(dup, "\n  "))
	}
}

func repoRootForTypes(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
