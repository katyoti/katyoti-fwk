// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"testing"
	"time"
)

// A HUB SPAWNED WITH Detach OUTLIVES ITS CLIENT (`--hub
// keep`): Close drops the connection only; the next connect finds the hub
// running; nothing owns it. Real spawn, skipped in -short mode.
func TestConnectToHub_FindOrSpawn_Detach_LeavesTheHubRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	dataDir := t.TempDir()
	t.Setenv("KTW_DATA", dataDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c1, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeFindOrSpawn, HubBinaryPath: binary, Detach: true})
	if err != nil {
		t.Fatalf("spawn detached: %v", err)
	}
	pid := c1.LockFile().PID
	t.Cleanup(func() { // the test, not the client, ends the hub
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	})
	if !c1.Spawned() {
		t.Fatal("first call: Spawned should be true (we started it)")
	}
	if c1.ownedHub != nil {
		t.Fatal("a detached hub is owned by nobody: ownedHub must be nil")
	}
	if err := c1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !ProcessAlive(pid) {
		t.Fatalf("the detached hub (pid %d) died with its client's Close", pid)
	}

	// The next connect finds it, and does not own it either.
	c2, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeFindOrSpawn, HubBinaryPath: binary})
	if err != nil {
		t.Fatalf("reuse after detach: %v", err)
	}
	if c2.Spawned() || c2.ownedHub != nil {
		t.Fatal("second call: found the kept hub, must not have spawned or own one")
	}
	if c2.LockFile().PID != pid {
		t.Fatalf("reused pid %d, want the kept hub %d", c2.LockFile().PID, pid)
	}
	// A client that found the hub — the GUI opened on a universe started
	// without it — leaves it running when it closes.
	if err := c2.Close(); err != nil {
		t.Fatalf("close the finder: %v", err)
	}
	if !ProcessAlive(pid) {
		t.Fatalf("the hub (pid %d) died with a client that merely found it", pid)
	}
}
