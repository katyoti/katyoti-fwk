// SPDX-License-Identifier: Apache-2.0

package core

// End-to-end test for the catalog-refresh path: spawn the fake sidecar
// in NATS mode via the manager, call resources/list over the bus, save
// the returned Manifest into a temp inventory dir. This exercises the
// full hub-side refresh-by-spawn flow without requiring the
// manifestlocal/Catalog wiring (Inventory.Save is the smallest piece
// the test needs and it's stable in core).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

func TestCatalogRefresh_EndToEnd(t *testing.T) {
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

	url := hub.Server.ClientURL()
	t.Setenv("KTW_NATS_URL", url)

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

	// Call resources/get-manifest — the fake sidecar's stub returns a
	// hard-coded Manifest. This is the same wire shape ktw-hub catalog
	// refresh uses.
	rawIn, _ := json.Marshal(mbapi.ContributionGetManifestInput{})
	out, err := sender.Call(ctx, "fake:/resources/get-manifest", json.RawMessage(rawIn))
	if err != nil {
		t.Fatalf("Send resources/get-manifest: %v", err)
	}

	var manifestOut mbapi.ContributionGetManifestOutput
	if err := json.Unmarshal(out, &manifestOut); err != nil {
		t.Fatalf("decode ContributionGetManifestOutput: %v", err)
	}
	if manifestOut.Manifest == nil {
		t.Fatal("nil Manifest in ContributionGetManifestOutput")
	}
	if manifestOut.Manifest.Provider != "fake" {
		t.Errorf("Provider = %q, want fake", manifestOut.Manifest.Provider)
	}
	if len(manifestOut.Manifest.Contributions) != 1 {
		t.Errorf("Contributions len = %d, want 1", len(manifestOut.Manifest.Contributions))
	}

	// Save to a per-sidecar cache and verify the manifest lands at
	// `<root>/fake@<key>/manifest.json` (2026-06-05 redesign — replaces the
	// prior flat-dir `<root>/fake-*.json` layout). Cache.Save stats a
	// binary to record exe_size + exe_mtime in metadata.yaml — fake
	// it with a placeholder file in tempdir.
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	cache := manifestlocal.NewCache(cacheRoot)
	fakeExe := filepath.Join(t.TempDir(), "fake.exe")
	if err := os.WriteFile(fakeExe, []byte{0}, 0o644); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	if err := cache.Save("fake", manifestOut.Manifest, fakeExe, manifestlocal.SourceDigests{}); err != nil {
		t.Fatalf("Cache.Save: %v", err)
	}

	// Verify the canonical files exist under the record's folder — keyed by
	// the member name and the binary the record was taken from.
	for _, want := range []string{"manifest.json", "metadata.yaml"} {
		if _, err := os.Stat(filepath.Join(cache.RecordDir("fake", fakeExe), want)); err != nil {
			t.Errorf("expected %s under %s: %v", want, cache.RecordDir("fake", fakeExe), err)
		}
	}
	_ = strings.HasPrefix // keep import live if other paths grow back

	// Verify Cache.Load returns the same shape.
	loaded, _, err := cache.Load("fake", fakeExe)
	if err != nil {
		t.Fatalf("Cache.Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Cache.Load returned nil")
	}
	if loaded.Provider != "fake" || loaded.Version != "0.0.1" {
		t.Errorf("loaded = %+v, want fake/0.0.1", loaded)
	}
}
