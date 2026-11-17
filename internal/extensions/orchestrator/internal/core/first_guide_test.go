// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Creating a universe with a first guide writes it in Univers.yaml and the
// summary carries it, an edit keeps it; a guide that is not one of its
// sidecars' is refused, and so is a URI that names no guide.
func TestAUniverseNamesItsFirstGuide(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	ctx := context.Background()
	sidecars := []api.CreateSidecarSelection{{Name: "ktw-acme", BinaryPath: "ktw-acme.exe"}}
	create := func(name, guide string) error {
		_, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: name, DisplayName: name,
			HomePath: filepath.Join(tempDir, "homes", name), Sidecars: sidecars, FirstGuide: guide})
		return err
	}

	const guide = "ktw-acme:/guide-recipe/welcome"
	if err := create("archi", guide); err != nil {
		t.Fatal(err)
	}
	cenv.ResetUniverseManifestCache()
	if m, err := cenv.LoadUniverseManifest(filepath.Join(tempDir, "homes", "archi", ".katyoti")); err != nil || m.FirstGuide != guide {
		t.Fatalf("Univers.yaml first guide = %+v, %v; want %s", m, err, guide)
	}
	info, err := s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"})
	if err != nil || info.Summary.FirstGuide != guide {
		t.Fatalf("summary first guide = %q, %v", info.Summary.FirstGuide, err)
	}
	if _, err := s.UpdateUniverse(ctx, api.UpdateUniverseInput{Name: "archi", DisplayName: "Archi"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"}); info.Summary.FirstGuide != guide {
		t.Fatalf("an edit lost the first guide: %q", info.Summary.FirstGuide)
	}

	if err := create("other", "ktw-docs:/guide-recipe/tour"); err == nil || !strings.Contains(err.Error(), "not among the universe's sidecars") {
		t.Fatalf("a guide of no sidecar of the universe: %v; want refused", err)
	}
	if err := create("odd", "ktw-acme:/skill/welcome"); err == nil || !strings.Contains(err.Error(), "is no guide") {
		t.Fatalf("a URI naming no guide: %v; want refused", err)
	}
}

// Creating a universe with tags writes them in Univers.yaml, each once, and
// the summary carries them; an edit keeps them (e.g. a
// universe made by hand, without a solution).
func TestAUniverseIsCreatedWithItsTags(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	ctx := context.Background()
	if _, err := s.CreateUniverse(ctx, api.CreateUniverseInput{Name: "archi", DisplayName: "Archi",
		HomePath: filepath.Join(tempDir, "homes", "archi"), Tags: []string{"archifactory", " archifactory", ""}}); err != nil {
		t.Fatal(err)
	}
	cenv.ResetUniverseManifestCache()
	if m, err := cenv.LoadUniverseManifest(filepath.Join(tempDir, "homes", "archi", ".katyoti")); err != nil || strings.Join(m.Tags, ",") != "archifactory" {
		t.Fatalf("Univers.yaml tags = %+v, %v; want [archifactory]", m, err)
	}
	if _, err := s.UpdateUniverse(ctx, api.UpdateUniverseInput{Name: "archi", DisplayName: "Archi"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := s.GetUniverse(ctx, api.GetUniverseInput{Name: "archi"}); strings.Join(info.Summary.Tags, ",") != "archifactory" {
		t.Fatalf("summary tags after an edit: %v", info.Summary.Tags)
	}
}

// The hub's own guides - the shell recipes every universe has, published
// under ktw-hub - may be a universe's first guide too: an
// ArchiFactory universe starts on « Utiliser KaTyoti ». A sidecar that is not
// a member is still refused.
func TestAUniverseMayStartOnAHubGuide(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	sidecars := []api.CreateSidecarSelection{{Name: "ktw-acme", BinaryPath: "ktw-acme.exe"}}
	create := func(name, guide string) error {
		_, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{Name: name, DisplayName: name,
			HomePath: filepath.Join(tempDir, "homes", name), Sidecars: sidecars, FirstGuide: guide})
		return err
	}
	if err := create("archi", "ktw-hub:/guide-recipe/use-katyoti"); err != nil {
		t.Fatalf("a hub guide as first guide: %v; want accepted", err)
	}
	if err := create("other", "ktw-docs:/guide-recipe/tour"); err == nil {
		t.Fatal("a guide of a sidecar that is not a member: accepted; want refused")
	}
}
