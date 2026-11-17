// SPDX-License-Identifier: Apache-2.0

package application

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProjectRootIsTheUniverseNotTheWorkingFolder: a universe that is its own
// project (no attached project, as `dev`) works in its workspace, whatever
// folder the process runs in — a served ktw-agents runs in its installation
// folder, a sidecar in its universe folder. A command, a check and a
// project-relative path all land in the workspace, never in the process's
// working folder.
func TestProjectRootIsTheUniverseNotTheWorkingFolder(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, katyotiFolderName), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_WORKSPACE", workspace)
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)
	ResetGlobals()
	t.Cleanup(ResetGlobals)

	if got := ProjectRoot(); !sameDir(got, workspace) {
		t.Errorf("ProjectRoot() = %q, want the universe's own project %q (the process runs in %q)", got, workspace, elsewhere)
	}
	if got := ProjectBase(); !sameDir(got, workspace) {
		t.Errorf("ProjectBase() = %q, want %q", got, workspace)
	}
	got, err := ProjectPath("docs/page.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(workspace, "docs", "page.md"); !sameDir(got, want) {
		t.Errorf("ProjectPath(docs/page.md) = %q, want %q", got, want)
	}
}

func sameDir(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }
