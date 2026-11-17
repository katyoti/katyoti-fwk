// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// universe_harness_test.go — one way to stand up the contract universe.
//
// The setup (resolve binaries → workspace → KTW_WORKSPACE → hub → sender) was
// written out inside TestCatalogContract. A second suite needing the same
// universe would have copied it, and a copied harness drifts: the comment about
// steering the hub away from the repo's live .katyoti is the kind of hard-won
// detail that survives in one copy and not the other.

// startContractUniverse builds an isolated universe holding `sidecarsToInclude`,
// starts a hub over it, and returns a sender addressed to it.
//
// Skips (or fails under KTW_E2E) when a sidecar binary is missing. Every
// resource is registered with t.Cleanup, so a caller just calls it.
func startContractUniverse(t *testing.T) *messagebus.Sender {
	t.Helper()

	var sidecars []hubharness.SidecarBin
	for _, name := range sidecarsToInclude {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "catalog universe: sidecar binary %s not built", name)
			return nil
		}
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: bin})
	}

	// The hub derives HomeDir from KTW_WORKSPACE (= <ws>/.katyoti), NOT from
	// KTW_DATA, so KTW_WORKSPACE must be set BEFORE it starts —
	// hubharness.Start spawns with os.Environ(), so the value is inherited.
	// This deliberately steers the hub AWAY from the repo's live .katyoti onto
	// a throwaway universe.
	ws := makeWorkspace(t, sidecars)
	dataDir := t.TempDir() // hub lockfile + state, independent of HomeDir
	// The aggregator's caches (tier 1 under <CacheDir>/sidecars, the content
	// store under <CacheDir>/contributions) go to a throwaway dir too — they
	// used to land in the developer's real XDG cache, so a contract run could
	// read a manifest a previous run had cached and write one the next real
	// hub would trust.
	t.Setenv("KTW_CACHE", t.TempDir())

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
	t.Cleanup(cancel)
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir: dataDir,
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		t.Fatalf("catalog universe: connect to hub (dataDir=%s): %v", dataDir, err)
	}
	t.Cleanup(func() { hc.Close() })

	return messagebus.NewSender(hc.MessageBus(), tenantID, universeName)
}
