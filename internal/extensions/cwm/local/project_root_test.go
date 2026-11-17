// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// THE OBJECT CORPUS IS READ FROM THE PROJECT FOLDER, NOT THE CONFIG ROOT.
//
// The index used to join `modules/` onto the CONFIG root, so it scanned
// `<project>/.katyoti/modules/` and never `<project>/modules/` — where the 474
// committed objects actually are. Every authored module was invisible while
// the tree rendered perfectly, which is the worst shape of this failure: it
// looks like it works.
func TestDeriveProjectRootStepsUpFromAConfigRoot(t *testing.T) {
	project := t.TempDir()
	cfgRoot := application.EnvOf(project).HomeDir()

	got := deriveProjectRoot(cfgRoot)
	if filepath.Clean(got) != filepath.Clean(project) {
		t.Fatalf("deriveProjectRoot(%q) = %q, want the project folder %q —\n"+
			"objects live in <project>/modules/, committed beside the code (G2)",
			cfgRoot, got, project)
	}
}

// AND IT REFUSES TO STEP UP FROM ANYTHING ELSE.
//
// Substrate tests pass a bare temp dir playing the config root's part. Stepping
// up from one of those would point the object scan at the SYSTEM TEMP FOLDER
// and walk whatever happens to be in it — a fallback far worse than the bug.
func TestDeriveProjectRootRefusesABarePath(t *testing.T) {
	bare := t.TempDir() // not any parent's config root
	if got := deriveProjectRoot(bare); got != "" {
		t.Errorf("deriveProjectRoot(%q) = %q, want empty — stepping up from a bare "+
			"path scans its parent, which for a test is the system temp folder", bare, got)
	}
	if got := deriveProjectRoot(""); got != "" {
		t.Errorf("deriveProjectRoot(\"\") = %q, want empty", got)
	}
}

// AN ATTACHED PROJECT CARRIES THE WHOLE WORKSPACE — STATE AND CORPUS.
//
// A universe that lives under `<GlobalHomeDir>/universes/<name>/` has nothing
// beside its config root but config and keys: its content is the repository
// ATTACHED to it, recorded in `<config root>/projects.yaml`. Stepping up to
// the parent there scans an empty `modules/` while the attached corpus is
// never read — the same "looks like it works" failure as the config-root bug
// above, one level out. And the node store, index and lock are workspace
// state: they belong in the PROJECT's own `.katyoti`, beside the modules,
// not in the universe folder.
func TestResolveWorkspaceRootsFollowsTheAttachedProject(t *testing.T) {
	universe := t.TempDir()
	project := t.TempDir()
	cfgRoot := application.EnvOf(universe).HomeDir()
	if err := os.MkdirAll(cfgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "projects:\n  - " + filepath.ToSlash(project) + "\n"
	if err := os.WriteFile(filepath.Join(cfgRoot, cenv.AttachedProjectsFile), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}

	store, corpus := resolveWorkspaceRoots(cfgRoot, "")
	if filepath.Clean(corpus) != filepath.Clean(project) {
		t.Fatalf("corpus root = %q, want the ATTACHED project %q, not the universe folder", corpus, project)
	}
	if want := application.EnvOf(project).HomeDir(); filepath.Clean(store) != filepath.Clean(want) {
		t.Fatalf("store root = %q, want the PROJECT's config root %q — CWM state lives in the project, "+
			"never beside the universe config", store, want)
	}
}

// AND WITHOUT ONE, THE ROOTS GIVEN — the corpus one step up from the config
// root, exactly what every universe living in its own checkout relies on.
func TestResolveWorkspaceRootsFallsBackToTheParent(t *testing.T) {
	project := t.TempDir()
	cfgRoot := application.EnvOf(project).HomeDir()
	store, corpus := resolveWorkspaceRoots(cfgRoot, "")
	if filepath.Clean(store) != filepath.Clean(cfgRoot) {
		t.Fatalf("store root = %q, want the config root %q unchanged", store, cfgRoot)
	}
	if filepath.Clean(corpus) != filepath.Clean(project) {
		t.Fatalf("corpus root = %q, want the parent %q", corpus, project)
	}
	// An explicit corpus root is kept verbatim.
	if _, corpus := resolveWorkspaceRoots(cfgRoot, "/explicit"); corpus != "/explicit" {
		t.Fatalf("explicit ProjectRoot = %q, want kept", corpus)
	}
}
