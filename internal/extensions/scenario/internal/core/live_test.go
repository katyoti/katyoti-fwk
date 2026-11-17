// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// : a test universe seeded from a live project runs on a copy of
// its data - the project object and the modules folder - and never writes the
// live project; refresh-project takes the live data again.

// liveProject is a project as a live repository holds it: data, code, a
// .katyoti/ store and a .git/ folder.
func liveProject(t *testing.T) string {
	t.Helper()
	live := t.TempDir()
	for rel, body := range map[string]string{
		"project.cwm.yaml":                  "id: live\ntype: project\n",
		"modules/m/objects/m.cwm.yaml":      "id: m\ntype: module\n",
		"modules/m/features/F-001.cwm.yaml": "id: m.F-001\ntype: feature\n",
		"internal/code.go":                  "package code\n",
		".katyoti/config/workspace.yaml":    "version: \"1\"\n",
		".git/HEAD":                         "ref: refs/heads/main\n",
	} {
		p := filepath.Join(live, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return live
}

func liveEngine(t *testing.T, live string) *Engine {
	t.Helper()
	return NewEngine(&api.Case{Name: "real-data", Dir: t.TempDir(), Project: api.CaseProject{Live: live}}, t.TempDir(), "", io.Discard)
}

func TestACaseTakesTheLiveProjectsDataAndNeverWritesIt(t *testing.T) {
	live := liveProject(t)
	before := treeOf(t, live)
	e := liveEngine(t, live)
	if err := e.seedLive(); err != nil {
		t.Fatal(err)
	}
	got := treeOf(t, e.RT.Project())
	for _, want := range []string{"project.cwm.yaml", "modules/m/objects/m.cwm.yaml", "modules/m/features/F-001.cwm.yaml"} {
		if got[want] != before[want] {
			t.Errorf("the case's project lacks %s", want)
		}
	}
	for rel := range got {
		if strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, ".katyoti/") || strings.HasPrefix(rel, ".git/") {
			t.Errorf("the case took %s: only the data is copied", rel)
		}
	}
	// An object written in the test universe stays there.
	written := filepath.Join(e.RT.Project(), "modules", "m", "features", "F-002.cwm.yaml")
	if err := os.WriteFile(written, []byte("id: m.F-002\ntype: feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.RT.Project(), "modules", "m", "features", "F-001.cwm.yaml"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := treeOf(t, live); !sameTree(before, after) {
		t.Fatal("the live project changed")
	}
}

func TestRefreshTakesTheLiveDataAgain(t *testing.T) {
	live := liveProject(t)
	e := liveEngine(t, live)
	if err := e.seedLive(); err != nil {
		t.Fatal(err)
	}
	// The case wrote an object; the live project moved on.
	if err := os.WriteFile(filepath.Join(e.RT.Project(), "modules", "m", "features", "F-009.cwm.yaml"), []byte("case only"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "modules", "m", "features", "F-003.cwm.yaml"), []byte("id: m.F-003\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := treeOf(t, e.RT.Project())
	if got["modules/m/features/F-003.cwm.yaml"] == "" {
		t.Error("the refresh did not take the live project's new object")
	}
	if _, kept := got["modules/m/features/F-009.cwm.yaml"]; kept {
		t.Error("the refresh kept what the case wrote: the copy is taken afresh")
	}
}

// : a live repository holding project folders below its root
// gives the case every one of them - each project object and its
// modules folder - and still nothing else; the live repository is only read.
func TestALiveProjectTakesEveryProjectFolder(t *testing.T) {
	live := liveProject(t)
	for rel, body := range map[string]string{
		"projects/a/project.cwm.yaml":                    "id: a\ntype: project\n",
		"projects/a/modules/ma/objects/ma.cwm.yaml":      "id: ma\ntype: module\n",
		"projects/a/src/main.go":                         "package main\n",
		"projects/a/.katyoti/config/workspace.yaml":      "version: \"1\"\n",
		"dossiers/b/project.cwm.yaml":                    "id: b\ntype: project\ncapabilities:\n  project:\n    modules_dir: docs/modules\n",
		"dossiers/b/docs/modules/mb/objects/mb.cwm.yaml": "id: mb\ntype: module\n",
		"dossiers/b/docs/readme.md":                      "code\n",
		"node_modules/x/project.cwm.yaml":                "id: x\ntype: project\n",
		"modules/m/nested/project.cwm.yaml":              "id: inside-a-module\ntype: project\n",
	} {
		p := filepath.Join(live, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := treeOf(t, live)
	e := liveEngine(t, live)
	if err := e.seedLive(); err != nil {
		t.Fatal(err)
	}
	got := treeOf(t, e.RT.Project())
	want := []string{
		"project.cwm.yaml", "modules/m/objects/m.cwm.yaml", "modules/m/features/F-001.cwm.yaml",
		"modules/m/nested/project.cwm.yaml", // a module's own file, copied with its module
		"projects/a/project.cwm.yaml", "projects/a/modules/ma/objects/ma.cwm.yaml",
		"dossiers/b/project.cwm.yaml", "dossiers/b/docs/modules/mb/objects/mb.cwm.yaml",
	}
	for _, w := range want {
		if got[w] == "" || got[w] != before[w] {
			t.Errorf("the case's project lacks %s", w)
		}
	}
	if len(got) != len(want) {
		var all []string
		for rel := range got {
			all = append(all, rel)
		}
		t.Errorf("the case took %d files, want only the %d data files: %v", len(got), len(want), all)
	}
	if after := treeOf(t, live); !sameTree(before, after) {
		t.Fatal("the live repository changed")
	}

	// A project folder the live repository drops is gone at the refresh.
	if err := os.RemoveAll(filepath.Join(live, "dossiers", "b")); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshProject(context.Background()); err != nil {
		t.Fatal(err)
	}
	got = treeOf(t, e.RT.Project())
	if _, kept := got["dossiers/b/project.cwm.yaml"]; kept {
		t.Error("the refresh kept a project folder the live repository dropped")
	}
	if got["projects/a/modules/ma/objects/ma.cwm.yaml"] == "" {
		t.Error("the refresh lost a project folder the live repository holds")
	}
}

func TestALiveProjectOverlappingTheCaseIsRefused(t *testing.T) {
	e := NewEngine(&api.Case{Name: "loop", Dir: t.TempDir()}, t.TempDir(), "", io.Discard)
	e.Case.Project.Live = e.RT.Dir // the case's own folder holds its project
	if err := os.MkdirAll(filepath.Join(e.RT.Dir, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.seedLive(); err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("seedLive = %v, want the overlap refused", err)
	}
}

func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
