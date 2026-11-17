// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// The admin-bootstrap tests (Ensure fresh/idempotent/self-heal) live with their
// owner in internal/extensions/orchestrator/admin. These two exercise the
// Service's delete-protection behaviour, using admin.Ensure only to seed state.

// TestService_DeleteAdminProtected — DeleteUniverse on admin returns a clear error.
func TestService_DeleteAdminProtected(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, _ := catalog.LoadRegistry(registryPath)
	if err := admin.Ensure(registry, adminHome); err != nil {
		t.Fatalf("admin.Ensure: %v", err)
	}
	m := &Service{registry: registry}

	_, err := m.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "admin"})
	if err == nil {
		t.Fatalf("DeleteUniverse(admin) should error")
	}
	if !strings.Contains(err.Error(), "protected") {
		t.Errorf("error message should contain 'protected', got: %v", err)
	}

	// Confirm registry entry still exists.
	if _, err := registry.Find("admin"); err != nil {
		t.Errorf("admin should still be in registry after failed delete: %v", err)
	}
}

// TestService_DeleteNonAdminWorks — non-protected universes still delete.
func TestService_DeleteNonAdminWorks(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")

	registry, _ := catalog.LoadRegistry(registryPath)
	m := &Service{registry: registry}
	isolateDataDir(t)

	// Create a regular universe.
	if _, err := m.CreateUniverse(context.Background(), api.CreateUniverseInput{
		Name:        "regular",
		DisplayName: "Regular",
		HomePath:    filepath.Join(tempDir, "ktw-universes", "regular"),
	}); err != nil {
		t.Fatalf("CreateUniverse: %v", err)
	}

	out, err := m.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "regular"})
	if err != nil {
		t.Fatalf("DeleteUniverse: %v", err)
	}
	if !out.Archived {
		t.Errorf("expected Archived=true")
	}
}
