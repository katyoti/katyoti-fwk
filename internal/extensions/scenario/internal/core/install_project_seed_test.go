// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A CASE DECLARING ITS PROJECT'S TYPE GETS THAT PROJECT OBJECT:
// the install leaves the project object's seed - its type and title - in the
// project's store, where CWM reads it at the first opening, as it reads a
// typed project's the hub creates. A case naming no type, or a project that
// already carries its project object, gets no seed: CWM's default, or the
// object it has.
func TestACaseProjectTypeIsSeeded(t *testing.T) {
	project := t.TempDir()
	if err := seedProjectObject(project, api.CaseProject{From: "project", Type: " dossier-architecture ", Title: "ArchiFactory"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(application.EnvOf(project).HomeDir(), cwmapi.ProjectSeedFile))
	if err != nil {
		t.Fatalf("no project seed in the project's store: %v", err)
	}
	var seed cwmapi.ProjectSeed
	if err := config.DecodeBytes(data, marshal.YAML, &seed); err != nil {
		t.Fatal(err)
	}
	if seed.Type != "dossier-architecture" || seed.Title != "ArchiFactory" || seed.Name != "" {
		t.Errorf("seed = %+v; want type dossier-architecture, title ArchiFactory, no name (the folder's)", seed)
	}

	for name, c := range map[string]struct {
		p      api.CaseProject
		object bool
	}{
		"no type":            {p: api.CaseProject{From: "project", Title: "ArchiFactory"}},
		"its project object": {p: api.CaseProject{Type: "dossier-architecture"}, object: true},
	} {
		dir := t.TempDir()
		if c.object {
			if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte("id: x\ntype: software-project\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := seedProjectObject(dir, c.p); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(application.EnvOf(dir).HomeDir(), cwmapi.ProjectSeedFile)); err == nil {
			t.Errorf("%s: a project seed was written", name)
		}
	}
}

// The case file reads its project's type and title.
func TestACaseReadsItsProjectType(t *testing.T) {
	dir := t.TempDir()
	body := "name: typed\nuniverse: {sidecars: [ktw-agents]}\nproject: {from: project, type: dossier-architecture, title: ArchiFactory}\nstages: [{id: s1}]\n"
	if err := os.WriteFile(filepath.Join(dir, "test-case.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := api.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Project.Type != "dossier-architecture" || !strings.EqualFold(c.Project.Title, "ArchiFactory") {
		t.Errorf("project = %+v", c.Project)
	}
}

// A CASE'S UNIVERSE STANDS FOR ITS SOLUTION: the solution a
// case declares is recorded in its Univers.yaml as a solution's install
// records it, so a project whose type needs it (a dossier d'architecture,
// ) opens there at every start, not only the first; a case declaring
// none leaves Univers.yaml as it is.
func TestACaseUniverseStandsForItsSolution(t *testing.T) {
	home := t.TempDir()
	if err := orchcatalog.SaveManifest(home, &orchcatalog.Manifest{Version: 1, Name: "scn-archifactory", Tags: []string{"archifactory"}}); err != nil {
		t.Fatal(err)
	}
	univers := filepath.Join(application.EnvOf(home).ConfigDir(), "Univers.yaml")
	before, _ := os.ReadFile(univers)
	if err := setUniverseSolution(home, " "); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(univers); string(after) != string(before) {
		t.Fatal("a case declaring no solution changed Univers.yaml")
	}
	if err := setUniverseSolution(home, "archifactory"); err != nil {
		t.Fatal(err)
	}
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if m.Solution != "archifactory" || m.Name != "scn-archifactory" || len(m.Tags) != 1 {
		t.Fatalf("the universe after install: %+v", m)
	}
}
