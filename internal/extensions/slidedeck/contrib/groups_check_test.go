// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The groups ktw-docs ships are sound: named after their file, no nested
// include, no lint error against the built-in layouts.
func TestKtwDocsSlideGroupsAreSound(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-docs", "embed", "slide-groups")
	if fs := CheckSlideGroups(os.DirFS(dir), "."); len(fs) != 0 {
		t.Fatalf("findings:\n%s", strings.Join(fs, "\n"))
	}
}

// The check is falsifiable: an unknown key, a nested include and a lint error
// are each named.
func TestCheckSlideGroupsNamesBadFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"g/a.yaml": {Data: []byte("name: a\ndescription: A\nslides: |\n  <!-- slide: include=b -->\ncolour: red\n")},
		"g/c.yaml": {Data: []byte("name: c\ndescription: C\nslides: |\n  <!-- slide: layout=hero -->\n  ## X\n")},
	}
	got := strings.Join(CheckSlideGroups(fsys, "g"), "\n")
	for _, want := range []string{"field colour not found", "unknown layout"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	nested := fstest.MapFS{"g/a.yaml": {Data: []byte("name: a\ndescription: A\nslides: |\n  ## One\n\n  ---\n\n  <!-- slide: include=b -->\n")}}
	if got := strings.Join(CheckSlideGroups(nested, "g"), "\n"); !strings.Contains(got, "includes another group") {
		t.Errorf("nested include not named:\n%s", got)
	}
}
