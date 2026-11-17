// SPDX-License-Identifier: Apache-2.0

package catalog

// Live end-to-end verification for contribution-event-incremental-refresh:
// a real ktw-hub, started over NATS, emits a `contributions-changed` event when
// its catalog reloads, and a real SubscribeContributionsChanged subscriber
// receives it — the cross-process half of the refresh chain that the dispatcher
// unit tests (app/ktw-agents/cmd) drive in-process. Together they prove the full
// loop: hub Reload → NATS event → subscriber → (dispatcher re-fetch + hooks).
//
// Trigger: rewriting the universe's Univers.yaml fires the hub's
// WatchUniverseSidecars watcher → SetMembers → catalog Reload → emit.
//
// Skips cleanly (via hubharness) when bin/ktw-hub or a sidecar binary is absent.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

func TestContributionsChanged_LiveRefresh(t *testing.T) {

	var sidecars []hubharness.SidecarBin
	for _, name := range []string{"ktw-docs", "ktw-methods"} {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "live refresh: sidecar binary %s not built", name)
			return
		}
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: bin})
	}

	ws := makeWorkspace(t, sidecars)
	dataDir := t.TempDir()

	prevWS, hadWS := os.LookupEnv("KTW_WORKSPACE")
	if err := os.Setenv("KTW_WORKSPACE", ws); err != nil {
		t.Fatalf("set KTW_WORKSPACE: %v", err)
	}
	t.Cleanup(func() {
		if hadWS {
			_ = os.Setenv("KTW_WORKSPACE", prevWS)
		} else {
			_ = os.Unsetenv("KTW_WORKSPACE")
		}
	})

	hubharness.Start(t, dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("live refresh: connect to hub: %v", err)
	}
	defer hc.Close()

	// Subscribe with the REAL subscriber, AFTER startup (so the boot-time
	// MarkReady event has already fired — we only catch a NEW one).
	got := make(chan contribapi.ContributionsChangedEvent, 4)
	sub, err := manifestlocal.SubscribeContributionsChanged(ctx, hc.MessageBus(), tenantID, universeName, 50*time.Millisecond,
		func(ev contribapi.ContributionsChangedEvent) { got <- ev })
	if err != nil {
		t.Fatalf("live refresh: subscribe: %v", err)
	}
	_ = sub

	// Let the NATS subscription register before triggering.
	time.Sleep(750 * time.Millisecond)

	// Trigger: rewrite Univers.yaml (append a comment) → WatchUniverseSidecars
	// → SetMembers → catalog Reload → emit contributions-changed.
	uniPath := filepath.Join(ws, ".katyoti", "config", "Univers.yaml")
	cur, err := os.ReadFile(uniPath)
	if err != nil {
		t.Fatalf("live refresh: read Univers.yaml: %v", err)
	}
	if err := os.WriteFile(uniPath, append(cur, []byte("\n# touched by live refresh test\n")...), 0o644); err != nil {
		t.Fatalf("live refresh: rewrite Univers.yaml: %v", err)
	}

	select {
	case ev := <-got:
		t.Logf("received contributions-changed: provider=%q kinds=%v", ev.Provider, ev.KindsChanged)
	case <-time.After(25 * time.Second):
		t.Fatal("live refresh: no contributions-changed event within 25s after Univers.yaml rewrite")
	}

	// Post-event, the catalog still serves a non-empty index — the fetch path
	// the dispatcher uses (ListContributions paging) works after a reload.
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, universeName)
	all := drainIndex(t, ctx, sender)
	if len(all) == 0 {
		t.Fatal("live refresh: ListContributions empty after reload")
	}
	t.Logf("post-reload catalog serves %d contributions", len(all))
}
