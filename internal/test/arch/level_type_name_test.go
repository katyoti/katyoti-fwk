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

// THE LEVEL IS A CAPABILITY (guarded by): whether an
// object is a project or a module is read from the level capability its type
// carries - `project`, `module` - never from a type's name. No reader tests a
// type named project or module outside the type declarations: in Go, in
// TypeScript, in the expressions and filters the packs ship and the universe
// tags' new: ids, in the tasks and skills they embed - nor against the level
// capability's own name (a type is never called CapProject). A test on a type of another name (a software module, a
// dossier d'architecture) would come back silently empty.
var levelTypeTests = []struct {
	ext string
	re  *regexp.Regexp
}{
	{".go", regexp.MustCompile(`\.Type\s*(==|!=)\s*("(project|module)"|(api\.)?Cap(Project|Module)\b)|ObjectListInput\{Type:\s*("(project|module)"|(api\.)?Cap(Project|Module)\b)|Types:\s*\[\]string\{[^}]*"(project|module)"`)},
	{".ts", regexp.MustCompile(`\.type\s*(===|!==|==|!=)\s*['"](project|module)['"]`)},
	{".tsx", regexp.MustCompile(`\.type\s*(===|!==|==|!=)\s*['"](project|module)['"]`)},
	{".yaml", regexp.MustCompile(`(?m)^\s*types:\s*\[[^\]]*\b(project|module|code-module)\b[^\]]*\]|^\s*types:[ \t]*\r?\n(?:[ \t]*-[^\n]*\n)*?[ \t]*-[ \t]*["']?(project|module|code-module)["']?[ \t]*\r?$|\btype\s*(==|!=)\s*'(project|module)'|\bnew:(project|module)\b`)},
	{".md", regexp.MustCompile(`\btype\s*(==|!=)\s*'(project|module)'`)},
}

func TestNoReaderTestsALevelTypeName(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	for _, dir := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				// Not the code: dependencies, generated bindings, test data.
				if name == "node_modules" || name == "wailsjs" || name == "testdata" || name == "dist" || name == "_compiled" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(filepath.ToSlash(p), "/tests/") {
				return nil
			}
			// A data migration converts FROM the old type names: its steps
			// read them by nature (cwm migrate.go).
			if strings.HasPrefix(name, "migrate") && strings.HasSuffix(name, ".go") {
				return nil
			}
			// The type declarations name themselves.
			if filepath.Base(filepath.Dir(p)) == "object-types" {
				return nil
			}
			ext := filepath.Ext(name)
			for _, c := range levelTypeTests {
				if c.ext != ext {
					continue
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				checked++
				if m := c.re.FindString(string(raw)); m != "" {
					rel, _ := filepath.Rel(root, p)
					t.Errorf("%s tests a level type's name (%q): ask the level capability - project or module - the type carries", filepath.ToSlash(rel), strings.TrimSpace(m))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 1000 {
		t.Fatalf("checked %d files - the walk missed the code", checked)
	}
}
