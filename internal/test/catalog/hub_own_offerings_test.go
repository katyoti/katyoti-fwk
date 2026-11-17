// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// TestUniverseListsTheHubsOwnOfferings_OverTheBus — e24 / N11, e26,
// cross-process: a running hub over a universe with no members at all still
// advertises its own offerings AND the system set at universe reach, resolves
// every system kind, serves its manifest, and answers a Read of one of its own
// entries and of a system entry (the hub is its own producer; it is not a
// member to spawn).
// Skips without bin/ktw-hub.
func TestUniverseListsTheHubsOwnOfferings_OverTheBus(t *testing.T) {
	ws := makeWorkspace(t, nil)
	t.Setenv("KTW_WORKSPACE", ws)
	t.Setenv("KTW_CACHE", t.TempDir())
	dataDir := t.TempDir()
	stop := hubharness.Start(t, dataDir)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	defer hc.Close()
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, universeName)
	cat := manifestremote.New(sender)

	services, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "service", Catalog: cat})
	if err != nil {
		t.Fatalf("Query services: %v", err)
	}
	found := false
	var uris []string
	for _, s := range services {
		uris = append(uris, s.ResolvedURI())
		if s.ResolvedURI() == "ktw-hub:/service/catalog-service" {
			found = true
		}
	}
	if !found {
		t.Fatalf("universe services = %v, want ktw-hub:/service/catalog-service", uris)
	}

	dataTypes, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "data-type", Catalog: cat})
	if err != nil {
		t.Fatalf("Query data-types: %v", err)
	}
	// The hub is the universe's one carrier of the system set (e26): with no
	// member at all, the system data-types are in the view, and a read of one
	// is answered by the hub; so is a read of the hub's own entries.
	readOne := map[string]bool{}
	for _, d := range dataTypes {
		if readOne[d.Provider] || (d.Provider != "system" && d.Provider != hubSelfProvider) {
			continue
		}
		content, _, err := readContent(ctx, sender, d.ResolvedURI())
		if err != nil || len(content) == 0 {
			t.Fatalf("Read(%s) through the hub: %d bytes, err=%v", d.ResolvedURI(), len(content), err)
		}
		readOne[d.Provider] = true
	}
	for _, p := range []string{"system", hubSelfProvider} {
		if !readOne[p] {
			t.Errorf("no %s data-type in a universe with no member to read", p)
		}
	}

	// Kinds resolve at universe reach from the hub member alone.
	kinds, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		t.Fatalf("KindsAt: %v", err)
	}
	for _, k := range []string{"skill", "task", "persona", "data-type", "contribution-type", "service"} {
		if _, ok := kinds.Get(k); !ok {
			t.Errorf("kind %q does not resolve at universe reach with the hub as the only member", k)
		}
	}

	// The hub's per-provider manifest is fetchable like any member's.
	m, err := cat.Manifest(ctx, hubSelfProvider)
	if err != nil || m == nil {
		t.Fatalf("Manifest(%s): %v", hubSelfProvider, err)
	}
	systemEntries := 0
	for _, c := range m.Contributions {
		if c.Provider == "system" {
			systemEntries++
		}
	}
	if systemEntries == 0 {
		t.Errorf("the hub's manifest (%d entries) carries no system entry", len(m.Contributions))
	}
}
