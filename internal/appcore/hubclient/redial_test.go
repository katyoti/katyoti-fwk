// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// redial_test.go — a HubClient follows the lock file to a hub that came back
// on another port (blind audit H5), in process: two embedded NATS servers
// stand in for the hub before and after its restart, and the lock file is
// rewritten the way `ktw-hub serve` rewrites it. The reconnect hook fires
// and the bus works again on the new server; the cross-process proof, with
// real binaries and a changed membership, is
// internal/test/catalog/hub_restart_test.go.

func startBus(t *testing.T) *natsembedded.EmbeddedHandle {
	t.Helper()
	opts := natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "js"), true)
	h, err := natsembedded.StartEmbedded(opts)
	if err != nil {
		t.Fatalf("start embedded NATS: %v", err)
	}
	t.Cleanup(h.Stop)
	return h
}

func TestHubClient_RedialsTheNewPortAfterTheHubRestarted(t *testing.T) {
	dataDir := t.TempDir()
	first := startBus(t)
	if err := WriteLockFile(dataDir, LockFile{PID: os.Getpid(), NATSURL: first.Server.ClientURL()}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := ConnectToHub(ctx, ConnectOptions{DataDir: dataDir, Mode: ModeReuse})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer hc.Close()
	reconnected := make(chan struct{}, 4)
	hc.OnReconnect(func() { reconnected <- struct{}{} })

	// A subscription taken on the first hub: nats-go re-establishes it on
	// the new one, so a message published there after the restart arrives.
	got := make(chan string, 4)
	if _, err := hc.MessageBus().Subscribe(ctx, "redial.probe", func(_ context.Context, m mbapi.Message) {
		got <- string(m.Data())
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// The hub goes down; its lock file with it (the hub removes it on exit).
	first.Stop()
	if err := RemoveLockFile(dataDir); err != nil {
		t.Fatal(err)
	}
	// A new hub, on whatever port it drew, writes a new lock file.
	second := startBus(t)
	if second.Server.ClientURL() == first.Server.ClientURL() {
		t.Skipf("the second server drew the first one's port %s; nothing to re-dial", second.Server.ClientURL())
	}
	if err := WriteLockFile(dataDir, LockFile{PID: os.Getpid(), NATSURL: second.Server.ClientURL()}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-reconnected:
	case <-time.After(30 * time.Second):
		t.Fatalf("no reconnect within 30s: the client keeps dialling %s while the hub listens at %s",
			first.Server.ClientURL(), second.Server.ClientURL())
	}
	if got, want := hc.NATSURL(), second.Server.ClientURL(); got != want {
		t.Errorf("NATSURL() = %s after the re-dial, want the new hub's %s", got, want)
	}
	if lf := hc.LockFile(); lf == nil || lf.NATSURL != second.Server.ClientURL() {
		t.Errorf("LockFile() = %+v after the re-dial, want the new hub's", lf)
	}

	// The link works: a publish through the NEW server reaches the
	// subscription taken on the old one.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := hc.MessageBus().Publish(ctx, "redial.probe", []byte("after")); err != nil {
			t.Fatalf("publish after the re-dial: %v", err)
		}
		select {
		case msg := <-got:
			if msg != "after" {
				t.Fatalf("got %q, want the message published after the restart", msg)
			}
			return
		case <-time.After(500 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("the subscription was not re-established on the new hub within 10s")
			}
		}
	}
}
