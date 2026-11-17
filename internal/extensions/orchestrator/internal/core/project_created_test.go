// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// UNTIL IN-1419, ONLY A REPOSITORY CREATED ON THIS PC JOINS AS A MEMBER
// . A universe's first project is attached as it is; beside
// it, a repository a creation verb of this PC made (project create here) is
// attached, a cloned one is refused before anything is written, the
// refusal saying why and naming IN-1419.
func TestOnlyARepositoryCreatedOnThisPCJoinsAsAMember(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}
	for _, u := range []string{"main", "own"} {
		if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: u, HomePath: filepath.Join(home, "universes", u)}); err != nil {
			t.Fatal(err)
		}
	}
	repo := func(body string, parts ...string) string {
		d := filepath.Join(append([]string{home}, parts...)...)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if err := os.WriteFile(filepath.Join(d, "project.cwm.yaml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}

	// The universe's first project: a clone, attached as it is.
	first := repo("id: demo\ntype: software-project\n", "clones", "demo")
	repo("", "clones", "demo", ".git")
	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "main", ProjectDir: first}); err != nil {
		t.Fatalf("a first project is attached as it is: %v", err)
	}

	// Created here, in a universe of its own, opened there (its project
	// object), detached, then attached beside the clone: accepted.
	made, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "made", Universe: "own"})
	if err != nil || !made.Created {
		t.Fatalf("create made: %+v %v", made, err)
	}
	repo("id: made\ntype: software-project\n", strings.Split(strings.TrimPrefix(made.ProjectDir, home+string(filepath.Separator)), string(filepath.Separator))...)
	if _, err := m.DetachProject(ctx, api.DetachProjectInput{ProjectDir: made.ProjectDir}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "main", ProjectDir: made.ProjectDir}); err != nil {
		t.Fatalf("a repository created on this PC refused beside another: %v", err)
	}

	// A clone beside them: refused, naming the third-party sidecars, nothing written.
	clone := repo("id: theirs\ntype: software-project\n", "clones", "theirs")
	_, err = m.AttachProject(ctx, api.AttachProjectInput{Universe: "main", ProjectDir: clone})
	if err == nil {
		t.Fatal("a cloned repository was attached beside the universe's members")
	}
	for _, want := range []string{"securing third-party sidecars", "not created on this PC", clone} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not say %q", err, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(application.EnvOf(clone).HomeDir(), "project.yaml")); statErr == nil {
		t.Error("the refused clone was left pointing at the universe")
	}
	if got := m.loadProjects("main").Projects; len(got) != 2 {
		t.Errorf("main attaches %v; want the first project and made", got)
	}
}
