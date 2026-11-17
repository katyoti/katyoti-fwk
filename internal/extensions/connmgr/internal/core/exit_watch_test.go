// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// A member that dies on its own (crashed, killed, taken down by a sleep) is no
// longer READY, and the next Acquire starts it again: before, the manager kept
// its dead sender, so EnsureSpawned "succeeded" and every call found no slot.
func TestAMemberThatDiesIsStartedAgain(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	hub, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "nats-jetstream"), true))
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	t.Setenv("KTW_NATS_URL", hub.Server.ClientURL())

	m, err := NewManagerWithBus(
		[]cmapi.SpawnSpec{{Name: "fake", Transport: "nats", Binary: buildFakeSidecar(t)}},
		BusScope{Bus: natsxport.New(hub.Client), TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := m.Acquire(ctx, "fake"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	m.Release("fake")
	c := m.conns["fake"]
	c.mu.Lock()
	first := c.cmd.Process.Pid
	_ = c.cmd.Process.Kill() // dies on its own, as far as the manager knows
	c.mu.Unlock()

	deadline := time.Now().Add(10 * time.Second)
	for {
		st, _ := m.Status("fake")
		if st.State == cmapi.ConnDeclared {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after its process died the member is still %s, want DECLARED", st.State)
		}
		time.Sleep(50 * time.Millisecond)
	}

	sender, err := m.Acquire(ctx, "fake")
	if err != nil {
		t.Fatalf("Acquire after the death: %v", err)
	}
	c.mu.Lock()
	second := c.cmd.Process.Pid
	c.mu.Unlock()
	if second == first {
		t.Fatalf("Acquire returned the dead process %d, want a new one", first)
	}
	if _, err := sender.Call(ctx, "fake:/Demo/echo", map[string]any{"back": true}); err != nil {
		t.Fatalf("a call to the started-again member: %v", err)
	}
	m.Release("fake")
}
