// SPDX-License-Identifier: Apache-2.0

package hubclient

// Real-spawn integration tests — exercise ModeSpawn and ModeFindOrSpawn
// against the actually-built ktw-hub binary. Skipped in -short mode.
//
// These are slower (~3-5s each because ktw-hub starts an embedded
// NATS server) but they're the only way to validate the lockfile
// race + connection sequence end-to-end.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// hubBinary returns the path to a built ktw-hub binary, skipping the
// test if it's not present. The caller is expected to `make ktw-hub`
// before running tests in non-short mode.
func hubBinary(t *testing.T) string {
	t.Helper()
	// Walk up to the module root so we find ./bin/ktw-hub regardless
	// of the test's working directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := "ktw-hub"
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "bin", bin)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Skipf("hub binary not built (run `make ktw-hub` first); cwd=%s", wd)
	return ""
}

func TestConnectToHub_Spawn_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)

	// Use a TempDir as DataDir so we don't collide with an existing
	// universe's hub. The spawned ktw-hub will write its lockfile here.
	dataDir := t.TempDir()
	t.Setenv("KTW_DATA", dataDir) // drive the spawned hub's Env().DataDir() to our tempdir

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := ConnectToHub(ctx, ConnectOptions{
		DataDir:       dataDir,
		Mode:          ModeSpawn,
		HubBinaryPath: binary,
	})
	if err != nil {
		t.Fatalf("ConnectToHub(Spawn): %v", err)
	}
	defer c.Close()

	// We should have a live NATS connection.
	if c.NATSURL() == "" {
		t.Error("NATSURL is empty after Spawn")
	}
	if c.MessageBus() == nil {
		t.Error("MessageBus is nil after Spawn")
	}
	// The hub we spawned should be tracked.
	if c.ownedHub == nil {
		t.Error("ownedHub is nil — its Wait is owed nowhere")
	}
	pid := c.LockFile().PID

	// Close drops the NATS conn; the hub, with no client left, ends by itself
	// and removes its lockfile, Close waiting for it.
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if ProcessAlive(pid) {
		t.Fatalf("after Close: hub PID %d, left with no client, still runs", pid)
	}
	if lf, _ := ReadLockFile(dataDir); lf != nil && lf.PID == pid {
		t.Errorf("after the hub ended: its lockfile is left: %+v", lf)
	}
}

// A SPAWNER LEAVING NEVER CUTS OFF THE HUB'S OTHER CLIENTS: the
// client that spawned the hub closes while another uses it — its Close returns
// without waiting for an end — and the hub still runs, the other client still
// connected; once that one closes too, the hub ends. Real hub, skipped in -short mode.
func TestConnectToHub_ASpawnerClosingLeavesTheHubToItsOtherClients(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	dataDir := t.TempDir()
	t.Setenv("KTW_DATA", dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	spawner, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeFindOrSpawn, HubBinaryPath: binary})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	other, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeFindOrSpawn, HubBinaryPath: binary})
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if !spawner.Spawned() || other.Spawned() {
		t.Fatalf("spawned: first=%v second=%v, want the first only", spawner.Spawned(), other.Spawned())
	}
	pid := spawner.LockFile().PID
	start := time.Now()
	_ = spawner.Close()
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("the spawner's Close waited %v on a hub another client uses", took)
	}

	time.Sleep(2 * time.Second)
	if !ProcessAlive(pid) || !other.handle.IsConnected() {
		t.Fatalf("the spawner's Close ended the hub under its other client (alive=%v connected=%v)", ProcessAlive(pid), other.handle.IsConnected())
	}

	_ = other.Close()
	deadline := time.Now().Add(30 * time.Second)
	for ProcessAlive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("the hub (pid %d) outlived its last client by 30s", pid)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestConnectToHub_FindOrSpawn_SpawnsThenReuses(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)

	dataDir := t.TempDir()
	t.Setenv("KTW_DATA", dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First call: spawns (no hub running).
	c1, err := ConnectToHub(ctx, ConnectOptions{
		DataDir:       dataDir,
		Mode:          ModeFindOrSpawn,
		HubBinaryPath: binary,
	})
	if err != nil {
		t.Fatalf("first ConnectToHub: %v", err)
	}
	if c1.ownedHub == nil {
		t.Error("first call: ownedHub should be set (we spawned)")
	}
	url1 := c1.NATSURL()

	// Second call (same DataDir, hub still running): reuses.
	c2, err := ConnectToHub(ctx, ConnectOptions{
		DataDir:       dataDir,
		Mode:          ModeFindOrSpawn,
		HubBinaryPath: binary,
	})
	if err != nil {
		t.Fatalf("second ConnectToHub (reuse): %v", err)
	}
	if c2.ownedHub != nil {
		t.Error("second call: ownedHub should be nil (we reused, didn't spawn)")
	}
	if c2.NATSURL() != url1 {
		t.Errorf("reused hub URL = %q, want %q", c2.NATSURL(), url1)
	}

	// Cleanup: each Close drops its connection; the hub ends with the last.
	_ = c2.Close()
	_ = c1.Close()
}
