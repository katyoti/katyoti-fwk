// SPDX-License-Identifier: Apache-2.0

package core

// End-to-end smoke for the idle-timer + SIGTERM-with-grace path. Spawns
// a real fakesidecar via `transport: nats` with `IdleTimeout: 200ms`,
// Acquires + Releases, waits for the timer to fire, and verifies:
//   - the FSM transitions READY → IDLE_DRAINING → DECLARED on its own;
//   - the spawned subprocess actually exits (no leak);
//   - a subsequent Acquire spawns a NEW process (fresh refCount,
//     new pid).
//
// Complements the in-process FSM tests in `manager_test.go` (which
// synthesise the state by direct field mutation) by exercising the
// real spawn → idle → respawn cycle the way production code does.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

func TestAcquire_NATS_IdleTimer_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}

	natsDir := filepath.Join(t.TempDir(), "nats-jetstream")
	hub, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(natsDir, true))
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	bus := natsxport.New(hub.Client)

	binary := buildFakeSidecar(t)
	t.Setenv("KTW_NATS_URL", hub.Server.ClientURL())

	const idleTimeout = 200 * time.Millisecond
	m, err := NewManagerWithBus(
		[]cmapi.SpawnSpec{{
			Name:        "fake",
			Transport:   "nats",
			Binary:      binary,
			IdleTimeout: idleTimeout,
		}},
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// First Acquire spawns the subprocess and brings the connection to READY.
	if _, err := m.Acquire(ctx, "fake"); err != nil {
		t.Fatalf("Acquire #1: %v", err)
	}
	st, _ := m.Status("fake")
	firstPID, err := pidFor(m, "fake")
	if err != nil {
		t.Fatalf("pidFor #1: %v", err)
	}
	if st.State != cmapi.ConnReady {
		t.Fatalf("State after Acquire = %s, want READY", st.State)
	}

	// Release: refCount → 0 → idle timer arms.
	m.Release("fake")

	// Poll until the timer fires (state → DECLARED). The timer is armed
	// for ~200ms; the idle handler then calls closeConn which races
	// transport.Close against the GracefulShutdown timer (default 5s).
	// We give it up to 8s — well above both budgets — to keep CI noise
	// minimal. `waitForState` (in manager_test.go) fatals on timeout.
	waitForState(t, m, "fake", cmapi.ConnDeclared, 8*time.Second)

	// Verify the subprocess actually exited. The fakesidecar listens
	// for SIGTERM and exits on signal; closeConn's stdio path (used by
	// nats-transport's subprocess handle) racing transport.Close
	// should have ensured exit by now.
	if hubclient.ProcessAlive(firstPID) {
		// Give the OS another moment — Wait() can return before the
		// kernel fully reaps in pathological cases on Windows.
		time.Sleep(500 * time.Millisecond)
		if hubclient.ProcessAlive(firstPID) {
			t.Errorf("first sidecar pid %d still alive after idle disconnect — process leak", firstPID)
		}
	}

	// Acquire #2: should spawn a FRESH process (not reuse the dead one).
	if _, err := m.Acquire(ctx, "fake"); err != nil {
		t.Fatalf("Acquire #2: %v", err)
	}
	secondPID, err := pidFor(m, "fake")
	if err != nil {
		t.Fatalf("pidFor #2: %v", err)
	}
	if secondPID == firstPID {
		t.Errorf("Acquire #2 returned same pid %d — should have spawned fresh", firstPID)
	}
	st, _ = m.Status("fake")
	if st.RefCount != 1 {
		t.Errorf("RefCount after re-Acquire = %d, want 1", st.RefCount)
	}
}

// pidFor reads the live cmd's pid from the manager's internal state.
// Helper for the integration test only — the API doesn't expose pids
// (and shouldn't; pids are an implementation detail of the spawn path).
func pidFor(m *ManagerImpl, name string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.conns[name]
	if !ok {
		return 0, os.ErrNotExist
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil || c.cmd.Process == nil {
		return 0, os.ErrNotExist
	}
	return c.cmd.Process.Pid, nil
}
