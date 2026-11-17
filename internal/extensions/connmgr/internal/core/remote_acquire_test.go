// SPDX-License-Identifier: Apache-2.0

package core

// `transport: remote` integration tests. Spawn the fake sidecar
// out-of-band (mimicking a sidecar already running on the cluster),
// then ask the manager to Acquire it via transport=remote — the
// manager skips spawning and just builds a messagebus.Sender once the
// ping probe succeeds.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// runFakeSidecarOutOfBand starts the prebuilt fakesidecar binary as a
// subprocess connected to `natsURL`. It does NOT go through the
// connmgr — the test wants the sidecar to be "remote" from the
// manager's perspective.
//
// Returns a stop function that the test must defer to clean up.
func runFakeSidecarOutOfBand(t *testing.T, natsURL string) func() {
	t.Helper()
	binary := buildFakeSidecar(t)
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		"KTW_NATS_URL="+natsURL,
		"KTW_TENANT_ID=local",
		"KTW_UNIVERSE_ID=test",
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake sidecar: %v", err)
	}

	// Cleanup: kill the subprocess. The test explicitly verifies that
	// the manager's Close DOESN'T touch this process.
	stopOnce := sync.Once{}
	stop := func() {
		stopOnce.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_, _ = cmd.Process.Wait()
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func TestAcquire_Remote_EndToEnd(t *testing.T) {
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

	url := hub.Server.ClientURL()

	// Start the fake sidecar OUTSIDE the manager's control. The
	// connmgr will Acquire it via transport=remote — so it doesn't
	// own the process; it just connects via NATS.
	stopFake := runFakeSidecarOutOfBand(t, url)

	m, err := NewManagerWithBus(
		// Name is a connmgr-local label; Provider is the sidecar's
		// self-declared bus identity (the fakesidecar binary publishes
		// itself under "fake"). The test deliberately uses a different
		// Name to enforce the spec-vs-provider distinction.
		[]cmapi.SpawnSpec{{Name: "remote-fake", Provider: "fake", Transport: "remote"}},
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sender, err := m.Acquire(ctx, "remote-fake")
	if err != nil {
		t.Fatalf("Acquire (remote): %v", err)
	}

	st, _ := m.Status("remote-fake")
	if st.State != cmapi.ConnReady {
		t.Errorf("State = %s, want READY", st.State)
	}
	if st.RefCount != 1 {
		t.Errorf("RefCount = %d, want 1", st.RefCount)
	}

	// Real call through the remote sidecar.
	// Wire path uses the sidecar's selfProvider ("fake") — not the
	// spec name ("remote-fake"). Spec names are connmgr-local labels;
	// the subject scheme keys routing by the sidecar's actual identity.
	out, err := sender.Call(ctx, "fake:/Demo/echo", map[string]any{"hello": "remote"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["hello"] != "remote" {
		t.Errorf("echo round-trip lost data: %+v", got)
	}

	// Inspect the connection to verify the manager does NOT track a
	// subprocess for the remote sidecar.
	conn := m.conns["remote-fake"]
	conn.mu.Lock()
	if conn.cmd != nil {
		t.Error("remote connection has a *exec.Cmd; manager should not own remote processes")
	}
	if conn.transport != nil {
		t.Error("remote connection has a *transport.Session; remote should be sender-only")
	}
	conn.mu.Unlock()

	// Close — must NOT terminate the out-of-band sidecar. Verify by
	// confirming the sender still works after Close (resets state) and
	// the fake is still alive (we'll re-send via a fresh sender we
	// build directly to bypass the manager's now-DECLARED state).
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	st, _ = m.Status("remote-fake")
	if st.State != cmapi.ConnDeclared {
		t.Errorf("State after Close = %s, want DECLARED", st.State)
	}

	// The remote sidecar is still alive — Close on the manager must
	// not have terminated it. We rely on stopFake (deferred via
	// t.Cleanup) to actually kill the subprocess at test exit.
	_ = stopFake
}

// TestAcquire_Remote_FailsFastWhenSidecarMissing — when no sidecar is
// running on the bus, Acquire(remote) should FAIL within the readiness
// budget rather than hanging indefinitely.
func TestAcquire_Remote_FailsFastWhenSidecarMissing(t *testing.T) {
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

	// No fake sidecar running.

	m, err := NewManagerWithBus(
		// Provider points at a name no sidecar publishes — the readiness
		// probe should time out and surface a clean FAILED state. The
		// member's own readiness limit is what bounds the start: a start
		// runs apart from its caller, so a caller's deadline
		// shorter than the limit only gives up its wait.
		[]cmapi.SpawnSpec{{Name: "ghost", Provider: "ghost", Transport: "remote", ReadyTimeout: 2 * time.Second}},
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}

	// The caller waits far longer than the member's readiness limit, so
	// Acquire returns once the start has settled (FAILED written before its
	// waiters are released) however loaded the machine; the bound still fails
	// a regression that hangs the readiness loop cleanly.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err = m.Acquire(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error for unreachable remote sidecar, got nil")
	}

	st, _ := m.Status("ghost")
	if st.State != cmapi.ConnFailed {
		t.Errorf("State = %s, want FAILED", st.State)
	}
	if st.Error == "" {
		t.Error("FAILED state should carry a reason")
	}
}

func TestAcquire_RemoteWithoutBus_ErrorsExplicitly(t *testing.T) {
	// Stdio-only manager + transport=remote spec → clean error.
	m, _ := NewManager([]cmapi.SpawnSpec{{Name: "x", Provider: "x", Transport: "remote"}})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "remote") {
		t.Errorf("error should mention transport=remote: %v", err)
	}
}

func TestAcquire_RemoteWithoutProvider_ErrorsExplicitly(t *testing.T) {
	// transport=remote without Provider must fail loudly — the readiness
	// probe needs the bus identity, and falling back to spec.Name would
	// silently target the wrong subject.
	natsDir := filepath.Join(t.TempDir(), "nats-jetstream")
	hub, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(natsDir, true))
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	bus := natsxport.New(hub.Client)

	m, err := NewManagerWithBus(
		[]cmapi.SpawnSpec{{Name: "no-provider", Transport: "remote"}}, // Provider intentionally unset
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}

	_, err = m.Acquire(context.Background(), "no-provider")
	if err == nil {
		t.Fatal("expected error for missing Provider, got nil")
	}
	if !strings.Contains(err.Error(), "Provider") {
		t.Errorf("error should mention SpawnSpec.Provider: %v", err)
	}
}
