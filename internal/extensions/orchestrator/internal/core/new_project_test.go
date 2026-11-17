// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A new project is created in the projects folder under its name and refused,
// before anything is created, when the name is not a valid one, a project of
// that name is registered or its folder exists - the rule of a universe name
// . A check answers the folder and the refusal, creating nothing.
func TestANewProjectIsCreatedUnderItsNameInTheProjectsFolder(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}
	projects := filepath.Join(home, "Documents", "Katyoti")
	for _, u := range []string{"main", "other", "busy"} {
		if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: u, HomePath: filepath.Join(home, "universes", u)}); err != nil {
			t.Fatal(err)
		}
	}

	// A check: the folder it would make, nothing created.
	chk, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Check: true})
	if err != nil || chk.Refused != "" || chk.ProjectDir != filepath.Join(projects, "archi") {
		t.Fatalf("check archi: %+v %v; want %s, not refused", chk, err, filepath.Join(projects, "archi"))
	}
	if _, err := os.Stat(chk.ProjectDir); !os.IsNotExist(err) {
		t.Fatalf("a check created %s", chk.ProjectDir)
	}

	// Created and attached.
	out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Universe: "main"})
	if err != nil || !out.Created || out.Universe != "main" {
		t.Fatalf("create archi: %+v %v", out, err)
	}
	if got := m.loadProjects("main").Projects; len(got) != 1 || got[0] != out.ProjectDir {
		t.Fatalf("main serves %v; want %s", got, out.ProjectDir)
	}

	// The same name again: registered, refused - by a check and by a creation.
	if chk, _ := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Check: true}); chk.Reason != "registered" || !strings.Contains(chk.Refused, "registered") {
		t.Errorf("check archi again: refused %q (%s); want registered", chk.Refused, chk.Reason)
	}
	if _, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Universe: "other"}); err == nil || !strings.Contains(err.Error(), "archi") {
		t.Errorf("archi created twice: %v", err)
	}
	if got := m.loadProjects("other").Projects; len(got) != 0 {
		t.Errorf("a refused creation attached %v", got)
	}

	// A project registered elsewhere under that name is refused too.
	shared := filepath.Join(home, "work", "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "busy", ProjectDir: shared}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "shared"}); err == nil || !strings.Contains(err.Error(), shared) {
		t.Errorf("shared created though registered at %s: %v", shared, err)
	}

	// A folder that exists is never taken over.
	taken := filepath.Join(projects, "taken")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taken, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "taken", Universe: "other"}); err == nil || out.Reason != "folder-exists" || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("taken created over its folder: %v (%s)", err, out.Reason)
	}
	if b, _ := os.ReadFile(filepath.Join(taken, "notes.txt")); string(b) != "mine" {
		t.Errorf("the existing folder was touched")
	}

	// A name that is not a universe name.
	for _, bad := range []string{"", "Bad Name", "../up", `a\b`} {
		if out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: bad}); err == nil || out.Reason != "invalid-name" {
			t.Errorf("project %q created (or not refused as invalid: %s)", bad, out.Reason)
		}
	}

	// A universe that attaches a project already takes a new one beside it
	// as a second member: typed or not, the
	// new project is keyed before its first opening by its seed's name, else
	// its folder's - CWM mints its project object before the store set joins
	// it. Said by the check, made by the creation.
	for _, typ := range []string{"", "software-project"} {
		name := "late"
		if typ != "" {
			name = "late-typed"
		}
		chk, _ := m.CreateProject(ctx, api.CreateProjectInput{Name: name, Universe: "busy", Type: typ, Check: true})
		if chk.Refused != "" || chk.Reason != "" {
			t.Errorf("check %s (type %q) in busy: refused %q (%s); want accepted beside the member", name, typ, chk.Refused, chk.Reason)
		}
		out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: name, Universe: "busy", Type: typ})
		if err != nil || !out.Created || out.Universe != "busy" {
			t.Fatalf("create %s (type %q) into busy, which attaches a member: %+v %v; want created as a second member", name, typ, out, err)
		}
	}
	if got := m.loadProjects("busy").Projects; len(got) != 3 {
		t.Errorf("busy attaches %v; want shared, late and late-typed", got)
	}
	// A project holding its project object joins as a member by attach too,
	// and a new project of a member's key is refused (registered) - its
	// folder's name or its project object's id.
	joiner := filepath.Join(home, "work", "joiner")
	if err := os.MkdirAll(joiner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(joiner, "project.cwm.yaml"), []byte("id: joined\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// It was created on this PC.
	if err := m.recordCreatedHere(joiner); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AttachProject(ctx, api.AttachProjectInput{Universe: "busy", ProjectDir: joiner}); err != nil {
		t.Fatalf("a member with its project object refused beside others: %v", err)
	}
	for _, key := range []string{"late", "joined"} {
		if out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: key, Universe: "busy"}); err == nil || out.Reason != "registered" {
			t.Errorf("a project of a member's key %q created: %v (%s)", key, err, out.Reason)
		}
	}

	// A universe created with a project of its own follows the same rule,
	// refused before it exists.
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "taken", HomePath: filepath.Join(home, "universes", "taken"), WithProject: true}); err == nil {
		t.Error("universe taken created on an existing project folder")
	}
	if _, err := registry.Find("taken"); err == nil {
		t.Error("a refused universe is registered")
	}
}
