// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// A UNIVERSE THAT DOES NOT LIVE CENTRALLY CAN STILL BE ATTACHED TO.
//
// This is the case the central-path assumption made impossible. `universes.yaml`
// records a `home_path` per universe precisely so one can sit anywhere, and a
// repository checkout is the ordinary case — the software and the universe that
// configures it committed together. Attach composed
// `<GlobalHomeDir>/universes/<name>/.katyoti` regardless, so a universe
// registered at a repo path was refused with a path nobody had chosen:
//
//	attach: universe "acme" not found at C:\...\universes\acme\.katyoti
//	        — create it first
//
// while the universe was registered, present, and running.
func TestAttachProject_UniverseOutsideTheCentralRoot(t *testing.T) {
	root := projectTestRoot(t)
	// The universe lives in its own folder, the way a repo checkout does.
	repo := t.TempDir()
	seedUniverseAt(t, root, "acme", repo)

	project := t.TempDir()
	svc := &Service{}
	if _, err := svc.AttachProject(context.Background(), api.AttachProjectInput{
		ProjectDir: project, Universe: "acme",
	}); err != nil {
		t.Fatalf("attach to a non-central universe: %v", err)
	}

	// AND THE RUNTIME MUST AGREE. Attach succeeding while resolution still
	// composed the central path would be worse than the refusal: the project
	// would read its config and identity from a folder that does not exist,
	// with nothing to notice.
	env := application.EnvOf(project)
	want := application.EnvOf(repo).HomeDir()
	if filepath.Clean(env.UniverseDir()) != filepath.Clean(want) {
		t.Fatalf("UniverseDir = %q, want the REGISTERED home %q —\n"+
			"attach and the runtime must resolve a universe the same way",
			env.UniverseDir(), want)
	}
}

// THE ATTACHED-PROJECTS FILE MUST NOT LAND IN THE UNIVERSE'S HOME ROOT.
//
// A universe home is commonly a REPOSITORY, and this file records absolute,
// machine-local project paths — `pr2-no-local-paths.sh` exists because such
// paths must never be committed. The first successful attach dropped a
// `projects.yaml` into the repo root; it belongs in the config root, which is
// already ignored and is where per-universe machine state lives.
func TestAttachProject_DoesNotWriteIntoTheUniverseHomeRoot(t *testing.T) {
	root := projectTestRoot(t)
	repo := t.TempDir()
	seedUniverseAt(t, root, "acme", repo)

	if _, err := (&Service{}).AttachProject(context.Background(), api.AttachProjectInput{
		ProjectDir: t.TempDir(), Universe: "acme",
	}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "projects.yaml")); err == nil {
		t.Error("projects.yaml was written to the universe home ROOT — for a universe " +
			"that is a repository checkout, that drops machine-local absolute paths " +
			"into the repository")
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(repo).HomeDir(), "projects.yaml")); err != nil {
		t.Errorf("projects.yaml is not in the universe's config root: %v", err)
	}
}

// AN UNREGISTERED NAME IS REFUSED, AND SAYS SO.
func TestAttachProject_UnregisteredUniverseIsRefused(t *testing.T) {
	root := projectTestRoot(t)
	_ = root
	if _, err := (&Service{}).AttachProject(context.Background(), api.AttachProjectInput{
		ProjectDir: t.TempDir(), Universe: "nope",
	}); err == nil {
		t.Fatal("attaching to an unregistered universe should be refused")
	}
}
