// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// NO PROJECT INSIDE ANOTHER: attaching or
// switching to a folder inside another project or Git repository is refused,
// naming it and where the project belongs, before anything changes; a folder
// of the projects folder is accepted - created or attached - even when the
// projects folder itself lies in a repository.
func TestAProjectInsideAnotherIsRefused(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}
	for _, u := range []string{"main", "spare"} {
		if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: u, HomePath: filepath.Join(home, "universes", u)}); err != nil {
			t.Fatal(err)
		}
	}
	mkdir := func(parts ...string) string {
		d := filepath.Join(append([]string{home}, parts...)...)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	outer := mkdir("work", "outer")
	mkdir("work", "outer", ".git")
	inner := mkdir("work", "outer", "inner")
	own := mkdir("work", "own")

	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "spare", ProjectDir: inner}); err == nil ||
		!strings.Contains(err.Error(), outer) || !strings.Contains(err.Error(), "projects folder") {
		t.Fatalf("attach %s: %v; want refused naming %s and the projects folder", inner, err, outer)
	}
	if got := m.loadProjects("spare").Projects; len(got) != 0 {
		t.Fatalf("a refused attach attached %v", got)
	}
	if _, err := os.Stat(filepath.Join(inner, ".katyoti")); !os.IsNotExist(err) {
		t.Fatalf("a refused attach wrote into %s (%v)", inner, err)
	}

	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "main", ProjectDir: own}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SwitchProject(ctx, api.SwitchProjectInput{Universe: "main", ProjectDir: inner}); err == nil || !strings.Contains(err.Error(), outer) {
		t.Fatalf("switch to %s: %v; want refused naming %s", inner, err, outer)
	}
	if got := m.loadProjects("main").Projects; !slices.Equal(got, []string{own}) {
		t.Fatalf("after a refused switch main serves %v; want %s", got, own)
	}

	// The projects folder in a repository of its own: its projects are accepted.
	mkdir("Documents", ".git")
	out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Universe: "spare"})
	if err != nil || !out.Created {
		t.Fatalf("create archi in a projects folder inside a repository: %+v %v; want created", out, err)
	}
}

// A FOLDER INSIDE ANOTHER PROJECT IS REFUSED (: no
// project inside a project): a folder holding a project file under another
// project's repository - its projects/<name> included - is neither switched
// to nor attached; the refusal names the enclosing project and the projects
// folder, nothing is written into the folder, and the served project stays.
func TestAFolderInsideAnotherProjectIsRefused(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}
	for _, u := range []string{"main", "spare"} {
		if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: u, HomePath: filepath.Join(home, "universes", u)}); err != nil {
			t.Fatal(err)
		}
	}
	project := func(body string, parts ...string) string {
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
	outer := project("id: demo\ntype: software-project\n", "work", "demo")
	project("", "work", "demo", ".git")
	nested := project("id: alpha\ntype: software-project\n", "work", "demo", "projects", "alpha")
	deeper := project("id: beta\ntype: software-project\n", "work", "demo", "src", "beta")

	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "main", ProjectDir: outer}); err != nil {
		t.Fatalf("attach %s: %v", outer, err)
	}
	for _, dir := range []string{nested, deeper} {
		if _, err := m.SwitchProject(ctx, api.SwitchProjectInput{Universe: "main", ProjectDir: dir}); err == nil ||
			!strings.Contains(err.Error(), outer) || !strings.Contains(err.Error(), "projects folder") {
			t.Fatalf("switch to %s: %v; want refused naming %s and the projects folder", dir, err, outer)
		}
		if got := m.loadProjects("main").Projects; !slices.Equal(got, []string{outer}) {
			t.Fatalf("after a refused switch main serves %v; want %s", got, outer)
		}
		if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "spare", ProjectDir: dir}); err == nil || !strings.Contains(err.Error(), outer) {
			t.Fatalf("attach %s: %v; want refused naming %s", dir, err, outer)
		}
		if got := m.loadProjects("spare").Projects; len(got) != 0 {
			t.Fatalf("a refused attach attached %v", got)
		}
		if _, err := os.Stat(filepath.Join(dir, ".katyoti")); !os.IsNotExist(err) {
			t.Fatalf("a refused attach wrote into %s (%v)", dir, err)
		}
	}
}
