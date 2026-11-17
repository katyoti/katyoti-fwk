// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"testing"
)

// : a folder inside a project or a Git repository is enclosed by the
// nearest one above it; a project's own root is not judged inside itself.
func TestNoRepositorySitsInsideAnother(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.cwm.yaml", "id: demos\ntype: software-project\n")
	write("inner/.git/HEAD", "ref: refs/heads/main\n")

	if got := EnclosingProject(filepath.Join(root, "modules", "new")); got != root {
		t.Errorf("a folder of the project: enclosed by %q; want %s", got, root)
	}
	if got := EnclosingProject(filepath.Join(root, "inner", "src")); got != filepath.Join(root, "inner") {
		t.Errorf("a folder inside a repository below the project: enclosed by %q; want that repository", got)
	}
	if got := EnclosingProject(root); got == root {
		t.Errorf("a project's own root is judged inside itself")
	}
	outside := t.TempDir()
	if got := EnclosingProject(filepath.Join(outside, "free")); got != "" {
		rel, err := filepath.Rel(outside, got)
		if err == nil && rel == "." {
			t.Errorf("a folder of a clean place: enclosed by %q; want none there", got)
		}
	}
}
