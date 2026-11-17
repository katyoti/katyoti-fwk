// SPDX-License-Identifier: Apache-2.0

package catalog

// hub_restart_test.go — a consumer's universe view survives a hub restart
// (blind audit H5).
//
// The embedded NATS listens on a random port, and a client learns it from the
// lock file once, at connect. A hub that restarts comes back on another port:
// the client used to reconnect forever to the old URL, so `OnReconnect` — the
// one hook that resyncs a consumer's view — could never fire, and the view
// answered from memory for the life of the process. Now the client re-reads
// the lock file on every reconnect attempt and dials what it names, so the
// reconnect happens, the hook fires, and the resync runs.
//
// What this proves, end to end with real binaries: a hub, a consumer connected
// through hubclient, the hub stopped, the membership changed while it is down,
// a NEW hub started on a new port over the same data dir — and the consumer's
// reconnect hook fires, after which the aggregator's digests (what ResyncView
// compares) name the new member and its manifest is readable. Skips (fails
// under KTW_E2E) when the binaries are not built.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// writeUniverseMembers rewrites the workspace's Univers.yaml with the given
// membership — the same shape makeWorkspace writes, over the same file.
func writeUniverseMembers(t *testing.T, ws string, sidecars []hubharness.SidecarBin) {
	t.Helper()
	var block strings.Builder
	for _, sc := range sidecars {
		block.WriteString(fmt.Sprintf("\n  - name: %s\n    transport: stdio\n    binary: %s\n", sc.Name, sc.Binary))
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: Catalog Contract
icon: T
color: gray
sidecars:%s
`, universeName, block.String())
	if err := os.WriteFile(filepath.Join(ws, ".katyoti", "config", "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("rewrite Univers.yaml: %v", err)
	}
}

func TestHubRestart_ConsumerViewConvergesOnTheNewPort(t *testing.T) {
	docs := binaryFor(t, "ktw-docs")
	methods := binaryFor(t, "ktw-methods")
	if docs == "" || methods == "" {
		hubharness.MissingBinary(t, "hub restart: ktw-docs or ktw-methods not built")
		return
	}
	one := []hubharness.SidecarBin{{Name: "ktw-docs", Binary: docs}}
	two := append(one, hubharness.SidecarBin{Name: "ktw-methods", Binary: methods})

	ws := makeWorkspace(t, one)
	dataDir := t.TempDir()
	t.Setenv("KTW_WORKSPACE", ws)
	t.Setenv("KTW_CACHE", t.TempDir())

	stop := hubharness.Start(t, dataDir)
	first, err := hubclient.ReadLockFile(dataDir)
	if err != nil || first == nil {
		t.Fatalf("first hub's lock file: lf=%v err=%v", first, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to the first hub: %v", err)
	}
	defer hc.Close()
	reconnected := make(chan struct{}, 8)
	hc.OnReconnect(func() { reconnected <- struct{}{} })

	remote := manifestremote.New(messagebus.NewSender(hc.MessageBus(), tenantID, universeName))
	before, err := remote.Digests(ctx)
	if err != nil {
		t.Fatalf("digests before the restart: %v", err)
	}
	if _, has := before["ktw-docs"]; !has {
		t.Fatalf("the first hub does not serve ktw-docs: digests=%v", before)
	}
	if _, has := before["ktw-methods"]; has {
		t.Fatalf("ktw-methods is not a member yet, but the first hub serves it: digests=%v", before)
	}

	// The hub goes away, and while it is down the universe changes.
	stop()
	writeUniverseMembers(t, ws, two)

	// A new hub over the same data dir — on a new port, or the old URL would
	// answer again by luck and prove nothing about the re-dial.
	var second *hubclient.LockFile
	for attempt := 1; attempt <= 3; attempt++ {
		stopAgain := hubharness.Start(t, dataDir)
		second, err = hubclient.ReadLockFile(dataDir)
		if err != nil || second == nil {
			t.Fatalf("second hub's lock file: lf=%v err=%v", second, err)
		}
		if second.NATSURL != first.NATSURL {
			break
		}
		t.Logf("attempt %d: the new hub drew the old port %s; restarting it", attempt, second.NATSURL)
		stopAgain()
		second = nil
	}
	if second == nil {
		t.Fatalf("three hubs in a row drew the old port %s; cannot exercise the re-dial", first.NATSURL)
	}
	t.Logf("hub restarted: %s → %s", first.NATSURL, second.NATSURL)

	// The reconnect hook is what runs ResyncView in production; without a
	// re-dial it never fires, because the client keeps dialling the old URL.
	select {
	case <-reconnected:
	case <-time.After(90 * time.Second):
		t.Fatalf("the consumer never reconnected: still dialling %s while the hub listens at %s (the reconnect hook, and with it ResyncView, never ran)",
			first.NATSURL, second.NATSURL)
	}
	if got := hc.NATSURL(); got != second.NATSURL {
		t.Errorf("after the re-dial the client reports %s, want the new hub's %s", got, second.NATSURL)
	}

	// Converged: the digests the consumer compares on reconnect name the new
	// member, and its manifest is readable through the new link.
	deadline := time.Now().Add(60 * time.Second)
	for {
		after, derr := remote.Digests(ctx)
		if derr == nil {
			if h, has := after["ktw-methods"]; has && h != "" {
				m, merr := remote.Manifest(ctx, "ktw-methods")
				if merr != nil || m == nil || m.Hash != h {
					t.Fatalf("ktw-methods digest %s is announced but its manifest does not follow: m=%v err=%v", h, m, merr)
				}
				t.Logf("converged: %d providers after the restart, ktw-methods=%s", len(after), h)
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("reconnected, but the view never converged: digests=%v err=%v", after, derr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
