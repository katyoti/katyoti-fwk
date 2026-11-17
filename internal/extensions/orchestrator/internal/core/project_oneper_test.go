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

// projectTestRoot points the process at a fresh root.
//
// application.Env() caches GlobalHomeDir process-wide, so `t.Setenv` alone is
// ignored after the first test in a package — three tests here silently built
// fixtures under their own TempDir while the code read the first test's.
// ResetGlobals is the supported way to drop that cache.
func projectTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	return root
}

// seedUniverseFor makes a CENTRAL universe real enough for attach to accept
// it: its config root on disk AND an entry in the registry.
//
// The registry half is new. Attach used to compose the central path by
// convention, so a universe that existed on disk was attachable whether or not
// anything had registered it. It now asks the registry where the universe
// lives — because `home_path` exists so a universe can live anywhere — and an
// unregistered name is refused.
func seedUniverseFor(t *testing.T, root, name string) {
	t.Helper()
	seedUniverseAt(t, root, name, filepath.Join(root, "universes", name))
}

// seedUniverseAt registers a universe at an ARBITRARY home — the case the
// central-path assumption made impossible.
func seedUniverseAt(t *testing.T, root, name, home string) {
	t.Helper()
	if err := os.MkdirAll(application.EnvOf(home).HomeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := catalog.LoadRegistry(filepath.Join(root, "universes.yaml"))
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if err := reg.Add(catalog.RegistryEntry{Name: name, HomePath: home}); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// A REFUSAL MUST LEAVE NOTHING BEHIND. The guard first sat after the
// project.yaml write, so a rejected project was left pointing at a universe it
// was not attached to — a refusal that still mutated the caller.
func TestAttachProject_RefusalWritesNothing(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	seedUniverseFor(t, root, uni)
	a, b := filepath.Join(root, "pa"), filepath.Join(root, "pb")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: b}); err == nil {
		t.Fatal("expected a refusal")
	}
	if _, err := os.Stat(filepath.Join(b, ".katyoti", "project.yaml")); err == nil {
		t.Error("the refused project was left pointing at a universe it is not attached to")
	}
}

// Re-attaching the SAME project stays idempotent: the rule is "one project", not
// "attach once". Breaking this would make a repeated setup script fail.
func TestAttachProject_SameProjectIsIdempotent(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	seedUniverseFor(t, root, uni)
	a := filepath.Join(root, "pa")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	for i := 0; i < 2; i++ {
		if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: a}); err != nil {
			t.Fatalf("attach %d: %v", i+1, err)
		}
	}
	out, err := s.ListProjects(context.Background(), api.ListProjectsInput{Universe: uni})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Projects) != 1 {
		t.Errorf("got %d projects, want 1 — re-attaching duplicated the entry", len(out.Projects))
	}
}

// ONE UNIVERSE PER PROJECT. The project's CWM store lives in the project, so a
// second universe on the same folder is a second hub writing the same store.
// The attach is refused, names the universe holding it and a way out, and
// leaves the pointer and both registries as they were.
func TestAttachProject_RefusesASecondUniverse(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "first")
	seedUniverseFor(t, root, "second")
	p := filepath.Join(root, "proj")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "first", ProjectDir: p}); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	_, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "second", ProjectDir: p})
	if err == nil {
		t.Fatal("a second universe attached a project another universe serves — two hubs on one store")
	}
	if !strings.Contains(err.Error(), `"first"`) || !strings.Contains(err.Error(), "detach") {
		t.Errorf("error %q must name the universe holding it and how to detach it", err)
	}
	if got := projectUniverse(p); got != "first" {
		t.Errorf("the refusal rewrote the project's pointer to %q", got)
	}
	if out, _ := s.ListProjects(context.Background(), api.ListProjectsInput{Universe: "second"}); len(out.Projects) != 0 {
		t.Errorf("the refused universe lists the project: %v", out.Projects)
	}
	if out, _ := s.ListProjects(context.Background(), api.ListProjectsInput{Universe: "first"}); len(out.Projects) != 1 {
		t.Errorf("the first universe lost the project: %v", out.Projects)
	}
}

// A STALE POINTER IS TAKEN OVER: the universe it names no longer lists the
// project (detached by hand, its registry reset) — nothing serves the store.
func TestAttachProject_TakesOverAStalePointer(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "first")
	seedUniverseFor(t, root, "second")
	p := filepath.Join(root, "proj")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "first", ProjectDir: p}); err != nil {
		t.Fatal(err)
	}
	if err := s.deregisterProject("first", p); err != nil { // the pointer stays, the registry forgets
		t.Fatal(err)
	}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "second", ProjectDir: p}); err != nil {
		t.Fatalf("a stale pointer must be taken over: %v", err)
	}
	if got := projectUniverse(p); got != "second" {
		t.Errorf("pointer = %q, want second", got)
	}
}

// A pointer naming a universe that does not exist any more is stale too.
func TestAttachProject_TakesOverAPointerToAGoneUniverse(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "second")
	p := filepath.Join(root, "proj")
	if err := os.MkdirAll(application.EnvOf(p).HomeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(application.EnvOf(p).HomeDir(), "project.yaml"), []byte("universe: gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "second", ProjectDir: p}); err != nil {
		t.Fatalf("a pointer to a gone universe must be taken over: %v", err)
	}
}

// Detach must free the slot, or the one-project rule becomes a one-way door.
func TestAttachProject_DetachFreesTheSlot(t *testing.T) {
	root := projectTestRoot(t)
	uni := "shared"
	seedUniverseFor(t, root, uni)
	a, b := filepath.Join(root, "pa"), filepath.Join(root, "pb")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DetachProject(context.Background(), api.DetachProjectInput{ProjectDir: a}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: uni, ProjectDir: b}); err != nil {
		t.Errorf("after detaching, a new project could not attach: %v", err)
	}
}
