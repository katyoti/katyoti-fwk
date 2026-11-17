// SPDX-License-Identifier: Apache-2.0

package core

// Real-NATS integration test for `transport: nats` Acquire. Spawns an
// embedded NATS server, builds a tiny test sidecar binary on the fly,
// then asks the manager to Acquire it and verifies the returned
// messagebus.Sender works end-to-end.
//
// The test binary is built once per test process via `go build`; the
// binary subscribes to its slot subject via the same `messagebus.Subscribe`
// production sidecars use.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// buildFakeSidecar compiles the fakesidecar package (sibling to this
// one — see ../testing/fakesidecar/main.go) and returns the binary's absolute
// path. Cached per test process via sync.Once because `go build` is
// slow (1-3s).
var (
	fakeSidecarOnce sync.Once
	fakeSidecarPath string
	fakeSidecarErr  error
)

func buildFakeSidecar(t *testing.T) string {
	t.Helper()
	fakeSidecarOnce.Do(func() {
		dir, err := os.MkdirTemp("", "connmgr-fake-sidecar-*")
		if err != nil {
			fakeSidecarErr = err
			return
		}
		bin := "fake-sidecar"
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		out := filepath.Join(dir, bin)
		// Build by package import path so the "internal" rules apply
		// from inside the module — the fake sidecar legitimately
		// imports internal/kts/... because both share the connmgr
		// internal tree.
		cmd := exec.Command("go", "build", "-o", out,
			"github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/internal/testing/fakesidecar")
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fakeSidecarErr = err
			return
		}
		fakeSidecarPath = out
	})
	if fakeSidecarErr != nil {
		t.Fatalf("buildFakeSidecar: %v", fakeSidecarErr)
	}
	return fakeSidecarPath
}

func TestAcquire_NATS_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}

	// Embedded NATS for the bus.
	natsDir := filepath.Join(t.TempDir(), "nats-jetstream")
	natsOpts := natsembedded.EmbeddedOptions(natsDir, true)
	hub, err := natsembedded.StartEmbedded(natsOpts)
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	bus := natsxport.New(hub.Client)

	// Build the fake sidecar binary.
	binary := buildFakeSidecar(t)

	// Build the manager with bus scope. Sidecar must inherit
	// KTW_NATS_URL so its own messagebus.Subscribe can connect.
	url := hub.Server.ClientURL()
	t.Setenv("KTW_NATS_URL", url) // inherited by exec.Command in spawnNATS

	m, err := NewManagerWithBus(
		[]cmapi.SpawnSpec{{Name: "fake", Transport: "nats", Binary: binary}},
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatalf("NewManagerWithBus: %v", err)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sender, err := m.Acquire(ctx, "fake")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	st, _ := m.Status("fake")
	if st.State != cmapi.ConnReady {
		t.Errorf("State after Acquire = %s, want READY", st.State)
	}
	if st.RefCount != 1 {
		t.Errorf("RefCount = %d, want 1", st.RefCount)
	}

	// Real call through the spawned sidecar via NATS.
	out, err := sender.Call(ctx, "fake:/Demo/echo", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if string(out) == "" {
		t.Errorf("got empty response")
	}

	// Acquire is idempotent: second call returns same sender, refcount=2.
	sender2, err := m.Acquire(ctx, "fake")
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if sender2 != sender {
		t.Error("second Acquire returned a different sender (should be cached)")
	}
	st, _ = m.Status("fake")
	if st.RefCount != 2 {
		t.Errorf("RefCount after second Acquire = %d, want 2", st.RefCount)
	}

	// Close terminates the spawned sidecar and resets state to DECLARED.
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	st, _ = m.Status("fake")
	if st.State != cmapi.ConnDeclared {
		t.Errorf("State after Close = %s, want DECLARED", st.State)
	}
}
