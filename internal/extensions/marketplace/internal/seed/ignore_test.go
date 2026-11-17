// SPDX-License-Identifier: Apache-2.0

package seed

import (
	"os"
	"path/filepath"
	"testing"
)

// What the project's own .gitignore ignores is left out of its seed: a folder
// by name anywhere, a file pattern, a path from the folder; the rest is kept.
func TestStageLeavesOutWhatTheProjectIgnores(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		".gitignore":                   "# machine output\ntest_results/\n*.log\n/build\n!keep.log\n",
		"README.md":                    "x",
		"test_results/run1/out.txt":    "x",
		"modules/m/test_results/a.txt": "x",
		"modules/m/doc.md":             "x",
		"trace.log":                    "x",
		"build/bin.exe":                "x",
		"docs/build/page.md":           "x",
	}
	for p, body := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dst := t.TempDir()
	if _, err := Stage(src, "", dst); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dst, ProjectPart)
	for _, kept := range []string{"README.md", "modules/m/doc.md", "docs/build/page.md", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(kept))); err != nil {
			t.Errorf("%s should be in the seed: %v", kept, err)
		}
	}
	for _, gone := range []string{"test_results", "modules/m/test_results", "trace.log", "build"} {
		if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(gone))); err == nil {
			t.Errorf("%s is ignored by the project and should be left out", gone)
		}
	}
}
