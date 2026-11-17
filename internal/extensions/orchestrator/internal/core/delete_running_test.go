// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Deleting a universe MOVES its folder. A hub still serving that universe holds
// the folder open, and Windows then refuses the rename with a bare
// "Acces refuse" that tells the user nothing about what to do. Worse on Linux:
// the rename succeeds and the live hub keeps writing into a folder the user
// believes is gone.
//
// The GUI activates a universe IMPLICITLY on any listing call, so "I only
// looked at it" is enough to leave a hub running -- which is exactly how this
// was hit by hand.

// seedUniverse registers a universe with a real folder and returns its home path.
func seedUniverse(t *testing.T, tempDir, name string) (*catalog.Registry, string) {
	t.Helper()
	registry, err := catalog.LoadRegistry(filepath.Join(tempDir, "universes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(tempDir, "universes", name)
	if err := catalog.SaveManifest(home, &catalog.Manifest{
		Version: 1, Name: name, DisplayName: name,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(catalog.RegistryEntry{Name: name, HomePath: home}); err != nil {
		t.Fatal(err)
	}
	return registry, home
}

// writeHubLock plants a lockfile with an explicit PID, bypassing WriteLockFile
// (which substitutes the current PID for 0 -- and 0 is precisely the dead PID
// the stale case needs).
func writeHubLock(t *testing.T, home string, pid int) {
	t.Helper()
	dir := application.EnvOf(home).DataDir()
	path := hubclient.LockFilePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(hubclient.LockFile{PID: pid, NATSURL: "nats://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUniverse_RefusesWhileHubIsRunning(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("KTW_DATA", t.TempDir())
	registry, home := seedUniverse(t, tempDir, "busy")

	// This test process is, by construction, alive.
	writeHubLock(t, home, os.Getpid())

	s := &Service{registry: registry}
	_, err := s.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "busy"})
	if err == nil {
		t.Fatal("DeleteUniverse succeeded while a hub was running; it must refuse")
	}
	// The message has to name the remedy -- a bare OS error is what made the
	// original report unactionable.
	if !strings.Contains(err.Error(), "deactivate") {
		t.Errorf("error %q does not tell the user to deactivate first", err)
	}
	if _, statErr := os.Stat(home); statErr != nil {
		t.Errorf("universe folder was moved despite the refusal: %v", statErr)
	}
	if _, findErr := registry.Find("busy"); findErr != nil {
		t.Errorf("registry entry was dropped despite the refusal: %v", findErr)
	}
}

func TestDeleteUniverse_StaleLockDoesNotBlock(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("KTW_DATA", t.TempDir())
	registry, home := seedUniverse(t, tempDir, "crashed")

	// PID 0 is never a live process: a lockfile left behind by a hub that died
	// without cleaning up. One crash must not make a universe undeletable.
	writeHubLock(t, home, 0)

	s := &Service{registry: registry}
	out, err := s.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "crashed"})
	if err != nil {
		t.Fatalf("DeleteUniverse blocked by a STALE lockfile: %v", err)
	}
	if !out.Archived {
		t.Error("expected the universe to be archived")
	}
	if _, statErr := os.Stat(home); !os.IsNotExist(statErr) {
		t.Errorf("universe folder still at its original path: %v", statErr)
	}
}
