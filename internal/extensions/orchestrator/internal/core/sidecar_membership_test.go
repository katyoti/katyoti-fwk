// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// newMembershipFixture creates a registry + one universe with a single
// sidecar, returning the service and the universe home path.
func newMembershipFixture(t *testing.T) (*Service, string) {
	t.Helper()
	tempDir := t.TempDir()
	s := withRegistryAt(t, filepath.Join(tempDir, "universes.yaml"))
	home := filepath.Join(tempDir, "homes", "memb")
	_, err := s.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "memb",
		DisplayName: "Membership Test",
		HomePath:    home,
		Sidecars:    []api.CreateSidecarSelection{{Name: "ktw-docs"}},
	})
	if err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}
	return s, home
}

func TestAddSidecar_AppendsRow(t *testing.T) {
	s, home := newMembershipFixture(t)

	info, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-compose"},
	})
	if err != nil {
		t.Fatalf("AddSidecar: %v", err)
	}
	if len(info.Sidecars) != 2 {
		t.Fatalf("expected 2 members, got %d", len(info.Sidecars))
	}
	added := info.Sidecars[1]
	if added.Name != "ktw-compose" || added.Transport != "nats" {
		t.Fatalf("added row wrong: %+v", added)
	}
	if len(added.Args) != 3 || added.Args[2] != "nats" {
		t.Fatalf("added args wrong (want sc ktsp nats): %v", added.Args)
	}

	// Round-trip: the on-disk manifest carries the new row.
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Sidecars) != 2 || m.Sidecars[1].Name != "ktw-compose" {
		t.Fatalf("manifest round-trip wrong: %+v", m.Sidecars)
	}
}

func TestAddSidecar_DuplicateRejected(t *testing.T) {
	s, _ := newMembershipFixture(t)
	_, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-docs"},
	})
	if err == nil || !strings.Contains(err.Error(), "already a member") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestAddSidecar_UnknownUniverse(t *testing.T) {
	s, _ := newMembershipFixture(t)
	_, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "no-such",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-docs"},
	})
	if err == nil || !strings.Contains(err.Error(), "not in registry") {
		t.Fatalf("expected registry error, got %v", err)
	}
}

func TestAddSidecar_MissingBinaryRejected(t *testing.T) {
	s, _ := newMembershipFixture(t)
	_, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-x", BinaryPath: filepath.Join(t.TempDir(), "nope.exe")},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected binary-not-found error, got %v", err)
	}
}

func TestAddSidecar_ExistingBinaryAccepted(t *testing.T) {
	s, _ := newMembershipFixture(t)
	bin := filepath.Join(t.TempDir(), "ktw-x.exe")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-x", BinaryPath: bin},
	})
	if err != nil {
		t.Fatalf("AddSidecar with existing binary: %v", err)
	}
	if info.Sidecars[1].Binary != bin {
		t.Fatalf("binary path not carried: %+v", info.Sidecars[1])
	}
}

func TestRemoveSidecar_RemovesRow(t *testing.T) {
	s, home := newMembershipFixture(t)
	if _, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb",
		Sidecar:  api.CreateSidecarSelection{Name: "ktw-compose"},
	}); err != nil {
		t.Fatal(err)
	}

	info, err := s.RemoveSidecar(context.Background(), api.RemoveSidecarInput{
		Universe: "memb", Sidecar: "ktw-docs",
	})
	if err != nil {
		t.Fatalf("RemoveSidecar: %v", err)
	}
	if len(info.Sidecars) != 1 || info.Sidecars[0].Name != "ktw-compose" {
		t.Fatalf("expected only ktw-compose left, got %+v", info.Sidecars)
	}

	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Sidecars) != 1 || m.Sidecars[0].Name != "ktw-compose" {
		t.Fatalf("manifest round-trip wrong: %+v", m.Sidecars)
	}
}

func TestRemoveSidecar_AbsentRejected(t *testing.T) {
	s, _ := newMembershipFixture(t)
	_, err := s.RemoveSidecar(context.Background(), api.RemoveSidecarInput{
		Universe: "memb", Sidecar: "ktw-ghost",
	})
	if err == nil || !strings.Contains(err.Error(), "not a member") {
		t.Fatalf("expected not-a-member error, got %v", err)
	}
}

// TestAddSidecar_SmokeSequence covers the todo's smoke: create with 1,
// add a 2nd, remove the 1st — final state matches.
func TestAddSidecar_SmokeSequence(t *testing.T) {
	s, home := newMembershipFixture(t)
	if _, err := s.AddSidecar(context.Background(), api.AddSidecarInput{
		Universe: "memb", Sidecar: api.CreateSidecarSelection{Name: "ktw-example"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveSidecar(context.Background(), api.RemoveSidecarInput{
		Universe: "memb", Sidecar: "ktw-docs",
	}); err != nil {
		t.Fatal(err)
	}
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sidecars) != 1 || m.Sidecars[0].Name != "ktw-example" {
		t.Fatalf("final state wrong: %+v", m.Sidecars)
	}
}
