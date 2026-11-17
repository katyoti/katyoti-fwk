// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The templates ktw-docs ships are sound: each decodes strictly, is named
// after its file, and its includes resolve to partial templates.
func TestKtwDocsTemplatesAreSound(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "app", "ktw-docs", "embed", "content-templates")
	if fs := CheckTemplates(os.DirFS(dir), "."); len(fs) != 0 {
		t.Fatalf("findings:\n%s", strings.Join(fs, "\n"))
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) < 2 {
		t.Fatalf("ktw-docs ships %d templates", len(ents))
	}
}

// The check is falsifiable: an unknown key, a wrong name and a missing
// include are each named.
func TestCheckTemplatesNamesBadFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"t/a.yaml": {Data: []byte("name: a\ndescription: A\nsections:\n  - {id: x, title: X, intent: Y, colour: red}\n")},
		"t/b.yaml": {Data: []byte("name: other\ndescription: B\nsections:\n  - {id: x, include: nowhere}\n")},
	}
	got := strings.Join(CheckTemplates(fsys, "t"), "\n")
	for _, want := range []string{`unknown field "colour"`, `name "other" is not its file's name`, "no template in reach"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
