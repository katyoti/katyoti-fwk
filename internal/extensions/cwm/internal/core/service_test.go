// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// freshWorkspace creates a tempdir with a minimal workspace.yaml +
// the parent_id edge-kind YAML. Used by every assembly test.
func freshWorkspace(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	paths := storage.NewPaths(root)
	if err := storage.WriteWorkspaceRaw(root,
		[]byte("version: \"1\"\nbudgets:\n  records_max_size_bytes: 8192\n")); err != nil {
		t.Fatalf("workspace.yaml: %v", err)
	}
	// Edge kind for parent_id.
	ek := &api.EdgeTypeRegistration{
		Name: api.EdgeTypeParentID, From: "node", To: "node",
		Cardinality: "N:1",
	}
	ekData, _ := marshal.Marshal(ek, marshal.YAML)
	if err := atomicio.WriteAtomic(filepath.Join(paths.EdgeTypesDir(), string(ek.Name)+".yaml"), ekData, 0o644); err != nil {
		t.Fatalf("write edge kind: %v", err)
	}
	return root
}

// --- Assembly ---------------------------------------------------

func TestNewServiceConstructsCleanly(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{
		WorkspaceRoot: root,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
}

func TestNewServiceMissingWorkspaceRootRejected(t *testing.T) {
	_, err := New(stdctx.Background(), &CWMServiceConfig{})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("err = %v want ErrInvalidInput", err)
	}
}

func TestServiceLockfileAcquired(t *testing.T) {
	// In-process lockfile semantics: a second New() from the
	// same OS process is treated as a "renew" (case 2 of the
	// lockfile state machine), not as a foreign holder.
	// Multi-process refusal is exercised by the lockfile package's
	// own tests via on-disk record fakery. Here we just verify
	// the lockfile gets written.
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	if _, err := os.Stat(filepath.Join(root, ".lock")); err != nil {
		t.Errorf("expected .lock file at %s: %v", root, err)
	}
}
