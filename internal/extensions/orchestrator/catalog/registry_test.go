// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"path/filepath"
	"testing"
)

func TestRegistry_LoadEmptyMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "universes.yaml")
	r, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry(missing): %v", err)
	}
	if got := r.List(); len(got) != 0 {
		t.Fatalf("expected empty registry, got %d entries", len(got))
	}
}

func TestRegistry_AddListFindRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "universes.yaml")
	r, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	if err := r.Add(RegistryEntry{Name: "alpha", HomePath: "/tmp/alpha"}); err != nil {
		t.Fatalf("Add alpha: %v", err)
	}
	if err := r.Add(RegistryEntry{Name: "beta", HomePath: "/tmp/beta"}); err != nil {
		t.Fatalf("Add beta: %v", err)
	}

	// Adding a duplicate must error.
	if err := r.Add(RegistryEntry{Name: "alpha", HomePath: "/tmp/alpha2"}); err == nil {
		t.Fatalf("Add duplicate alpha: expected error, got nil")
	}

	// Find existing.
	got, err := r.Find("beta")
	if err != nil {
		t.Fatalf("Find beta: %v", err)
	}
	if got.HomePath != "/tmp/beta" {
		t.Fatalf("Find beta: HomePath=%q, want /tmp/beta", got.HomePath)
	}

	// Find missing.
	if _, err := r.Find("missing"); err == nil {
		t.Fatalf("Find missing: expected error, got nil")
	}

	// List in insertion order.
	all := r.List()
	if len(all) != 2 || all[0].Name != "alpha" || all[1].Name != "beta" {
		t.Fatalf("List: got %v, want [alpha, beta]", all)
	}

	// Remove existing.
	removed, err := r.Remove("alpha")
	if err != nil || !removed {
		t.Fatalf("Remove alpha: removed=%v err=%v", removed, err)
	}

	// Remove missing is idempotent.
	removed, err = r.Remove("alpha")
	if err != nil || removed {
		t.Fatalf("Remove alpha (second time): removed=%v err=%v", removed, err)
	}
}

func TestRegistry_ProtectedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "universes.yaml")
	r, _ := LoadRegistry(path)
	if err := r.Add(RegistryEntry{Name: "admin", HomePath: "/u/admin", Protected: true}); err != nil {
		t.Fatalf("Add admin: %v", err)
	}
	if err := r.Add(RegistryEntry{Name: "personal", HomePath: "/u/personal"}); err != nil {
		t.Fatalf("Add personal: %v", err)
	}

	// Reload from disk; Protected must persist.
	r2, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry (reload): %v", err)
	}
	admin, err := r2.Find("admin")
	if err != nil {
		t.Fatalf("Find admin: %v", err)
	}
	if !admin.Protected {
		t.Fatalf("admin.Protected = false after reload, want true")
	}
	personal, err := r2.Find("personal")
	if err != nil {
		t.Fatalf("Find personal: %v", err)
	}
	if personal.Protected {
		t.Fatalf("personal.Protected = true, want false")
	}
}

func TestRegistry_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "universes.yaml")
	r, _ := LoadRegistry(path)
	if err := r.Add(RegistryEntry{Name: "uni1", HomePath: "/u/1"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := r.Add(RegistryEntry{Name: "uni2", HomePath: "/u/2"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Reload from disk; insertion order preserved.
	r2, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry (reload): %v", err)
	}
	got := r2.List()
	if len(got) != 2 || got[0].Name != "uni1" || got[1].Name != "uni2" {
		t.Fatalf("reload: got %v, want [uni1, uni2]", got)
	}
}
