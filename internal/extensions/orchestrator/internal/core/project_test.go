// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Project attach/detach moved here from marketplace/internal/wire (the
// marketplace/universe concept split). The project methods use Env() + the
// projects.yaml helpers only (not the registry), so a zero-value Service suffices.

func TestService_AttachDetachProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root) // GlobalHomeDir
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	svc := &Service{}
	ctx := context.Background()
	project := t.TempDir()

	// attach to a universe whose central dir doesn't exist → refused
	if _, err := svc.AttachProject(ctx, api.AttachProjectInput{ProjectDir: project, Universe: "acme"}); err == nil {
		t.Fatal("attach to a missing universe should be refused")
	}

	// create the central universe dir, then attach
	seedUniverseFor(t, root, "acme")
	if _, err := svc.AttachProject(ctx, api.AttachProjectInput{ProjectDir: project, Universe: "acme"}); err != nil {
		t.Fatalf("attach: %v", err)
	}

	// end-to-end: the project now resolves config/identity to the central universe
	env := application.EnvOf(project)
	wantUniv := filepath.Join(env.GlobalHomeDir(), "universes", "acme", ".katyoti")
	if env.UniverseDir() != wantUniv {
		t.Fatalf("after attach, UniverseDir = %q, want central %q", env.UniverseDir(), wantUniv)
	}

	// detach → reverts to self-contained; idempotent
	if _, err := svc.DetachProject(ctx, api.DetachProjectInput{ProjectDir: project}); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if got := application.EnvOf(project).UniverseDir(); got != filepath.Join(project, ".katyoti") {
		t.Fatalf("after detach, UniverseDir = %q, want the project's own .katyoti", got)
	}
	if _, err := svc.DetachProject(ctx, api.DetachProjectInput{ProjectDir: project}); err != nil {
		t.Fatalf("detach should be idempotent, got: %v", err)
	}
}

func TestService_ListProjectsRegistry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	for _, u := range []string{"acme", "other"} {
		seedUniverseFor(t, root, u)
	}
	svc := &Service{}
	ctx := context.Background()

	// This exercises the registry across TWO universes: per-universe
	// register / dedupe / deregister. Two members of one universe are
	// memberBeside's rules (project_members_test.go): these bare folders are
	// not recorded as created on this PC, so a second one
	// beside the first would be refused.
	mustAttach := func(dir, universe string) {
		t.Helper()
		if _, err := svc.AttachProject(ctx, api.AttachProjectInput{ProjectDir: dir, Universe: universe}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(universe string) []string {
		t.Helper()
		out, err := svc.ListProjects(ctx, api.ListProjectsInput{Universe: universe})
		if err != nil {
			t.Fatal(err)
		}
		return out.Projects
	}
	a, b := t.TempDir(), t.TempDir()
	mustAttach(a, "acme")
	mustAttach(b, "other")

	// Each universe's registry is its own: b must not appear under acme.
	if got := list("acme"); len(got) != 1 || got[0] != mustAbs(t, a) {
		t.Fatalf("acme projects = %v, want just %s", got, a)
	}
	if got := list("other"); len(got) != 1 {
		t.Fatalf("other projects = %v, want 1", got)
	}

	mustAttach(a, "acme") // idempotent
	if got := list("acme"); len(got) != 1 {
		t.Fatalf("re-attach duplicated the entry: %v", got)
	}

	if _, err := svc.DetachProject(ctx, api.DetachProjectInput{ProjectDir: a}); err != nil {
		t.Fatal(err)
	}
	if got := list("acme"); len(got) != 0 {
		t.Fatalf("after detach acme projects = %v, want none", got)
	}
	// Detaching one universe's project must not disturb another's.
	if got := list("other"); len(got) != 1 {
		t.Fatalf("detaching from acme changed other: %v", got)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
