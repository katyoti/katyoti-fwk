// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
)

// A folder member is declared and runs no process: Acquire refuses it with
// ErrFolderMember, never as a member the universe does not declare.
func TestAFolderMemberIsNeverSpawned(t *testing.T) {
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "pack", Folder: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Acquire(context.Background(), "pack")
	if !errors.Is(err, cmapi.ErrFolderMember) {
		t.Fatalf("Acquire(folder member) = %v, want ErrFolderMember", err)
	}
	if errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
		t.Fatal("a folder member is declared: not 'not in universe'")
	}
}
