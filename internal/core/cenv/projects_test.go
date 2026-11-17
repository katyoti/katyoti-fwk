// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAttachedProjectsReadsTheRegistryInOrder(t *testing.T) {
	root := t.TempDir()
	body := "projects:\n  - D:/Projets/first\n  - D:/Projets/second\n"
	if err := os.WriteFile(filepath.Join(root, AttachedProjectsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := AttachedProjects(root)
	if len(got) != 2 || got[0] != "D:/Projets/first" || got[1] != "D:/Projets/second" {
		t.Fatalf("AttachedProjects = %v, want the two entries in file order", got)
	}
}

// ABSENT, EMPTY OR BROKEN → NIL, NEVER AN ERROR. A universe with no attached
// project is the self-contained layout, which is how every universe started.
func TestAttachedProjectsIsNilWhenThereIsNothingToRead(t *testing.T) {
	if got := AttachedProjects(t.TempDir()); got != nil {
		t.Errorf("no file: got %v, want nil", got)
	}
	if got := AttachedProjects(""); got != nil {
		t.Errorf("empty root: got %v, want nil", got)
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, AttachedProjectsFile), []byte("projects: [not: valid"), 0o644)
	if got := AttachedProjects(root); got != nil {
		t.Errorf("broken yaml: got %v, want nil", got)
	}
}
