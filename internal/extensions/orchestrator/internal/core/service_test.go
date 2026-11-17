// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// withRegistryAt returns a Service with a fresh empty registry at the given path.
func withRegistryAt(t *testing.T, registryPath string) *Service {
	t.Helper()
	registry, err := catalog.LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("catalog.LoadRegistry: %v", err)
	}
	return &Service{registry: registry}
}

func TestService_CreateThenList(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	homeDir := filepath.Join(tempDir, "homes", "acme")

	s := withRegistryAt(t, registryPath)

	info, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "acme",
		DisplayName: "Acme",
		Icon:        "R",
		Color:       "violet",
		HomePath:    homeDir,
	})
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}
	if info.Summary.Name != "acme" {
		t.Fatalf("CreateUniverse: name=%q", info.Summary.Name)
	}

	out, err := s.ListUniverses(context.Background(), api.ListUniversesInput{})
	if err != nil {
		t.Fatalf("ListUniverses: %v", err)
	}
	if len(out.Universes) != 1 {
		t.Fatalf("expected 1 universe, got %d", len(out.Universes))
	}
	if out.Universes[0].Name != "acme" || out.Universes[0].DisplayName != "Acme" {
		t.Fatalf("ListUniverses: got %+v", out.Universes[0])
	}
}

func TestService_GetUniverse(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))

	_, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "acme",
		DisplayName: "ACME",
		Icon:        "S",
		Color:       "blue",
		HomePath:    filepath.Join(tempDir, "homes", "acme"),
	})
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	info, err := s.GetUniverse(context.Background(), api.GetUniverseInput{Name: "acme"})
	if err != nil {
		t.Fatalf("GetUniverse: %v", err)
	}
	if info.Summary.Name != "acme" {
		t.Fatalf("GetUniverse: got %q", info.Summary.Name)
	}

	if _, err := s.GetUniverse(context.Background(), api.GetUniverseInput{Name: "missing"}); err == nil {
		t.Fatalf("GetUniverse(missing): expected error, got nil")
	}
}

func TestService_DeleteUniverseArchives(t *testing.T) {
	isolateDataDir(t)
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))

	homeDir := filepath.Join(tempDir, "homes", "todelete")
	_, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "todelete",
		DisplayName: "ToDelete",
		HomePath:    homeDir,
	})
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	out, err := s.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "todelete"})
	if err != nil {
		t.Fatalf("DeleteUniverse: %v", err)
	}
	if !out.Archived {
		t.Fatalf("DeleteUniverse: Archived=false")
	}
	if out.ArchivePath == "" {
		t.Fatalf("DeleteUniverse: ArchivePath empty")
	}

	// Idempotent — second delete returns Archived=false but no error.
	out2, err := s.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "todelete"})
	if err != nil {
		t.Fatalf("DeleteUniverse (second): %v", err)
	}
	if out2.Archived {
		t.Fatalf("DeleteUniverse (second): expected Archived=false")
	}
}

func TestService_CreateRejectsDuplicate(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))

	in := api.CreateUniverseInput{
		Name:        "dup",
		DisplayName: "Dup",
		HomePath:    filepath.Join(tempDir, "homes", "dup"),
	}
	if _, err := s.CreateUniverse(context.Background(), in); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	// Different home path so the path-exists check doesn't trigger first.
	in.HomePath = filepath.Join(tempDir, "homes", "dup2")
	if _, err := s.CreateUniverse(context.Background(), in); err == nil {
		t.Fatalf("second Create with same name: expected error, got nil")
	}
}

func TestService_CreateValidatesName(t *testing.T) {
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))

	for _, bad := range []string{"", "Has Caps", "has_underscore", "space space", "way-too-long-" + string(make([]byte, 70))} {
		_, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
			Name:        bad,
			DisplayName: "x",
			HomePath:    filepath.Join(tempDir, "homes", "test"),
		})
		if err == nil {
			t.Errorf("CreateUniverse(%q): expected error, got nil", bad)
		}
	}
}

func TestValidUniverseName(t *testing.T) {
	good := []string{"a", "abc", "ab-cd", "ms06c", "123-abc"}
	for _, n := range good {
		if !validUniverseName(n) {
			t.Errorf("validUniverseName(%q) = false, want true", n)
		}
	}
	bad := []string{"", "Aa", "a_b", "a b", "a/b"}
	for _, n := range bad {
		if validUniverseName(n) {
			t.Errorf("validUniverseName(%q) = true, want false", n)
		}
	}
}

// isolateDataDir points KTW_DATA at a fresh temp directory for the duration of
// the test.
//
// Without it these tests read REAL MACHINE STATE. DeleteUniverse refuses to
// delete a universe whose hub is still running, and it finds the hub through
// application.EnvOf(homePath).DataDir() — which, for a universe under
// t.TempDir() with no Settings.yaml, falls back to the XDG default
// (<LOCALAPPDATA>/katyoti/universes/no-universe) rather than staying inside the
// temp dir. So the lock lookup escapes the test.
//
// The symptom is a test that passes alone and fails in a full run: any hub
// alive at that moment — including one started by an EARLIER test in the same
// serialized suite — makes the delete refuse. Observed 2026-09-19 as
// "universe %q is still running (ktw-hub pid 16676)" from two tests whose own
// universes had never been started, with that pid gone by the time it was
// checked.
func isolateDataDir(t *testing.T) {
	t.Helper()
	t.Setenv("KTW_DATA", t.TempDir())
}
