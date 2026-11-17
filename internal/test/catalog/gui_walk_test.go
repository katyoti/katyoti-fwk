// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// TestGUIWalk_EveryEntryOfEveryKindOpens is the Resources pane's walk, without
// the pane: for every URI in the universe it makes exactly the calls the
// pane's bindings make — the envelope (`mbapi.Get`), the kind registry
// (`KindsAt`), the provider's manifest, and `OpenLayer` for every layer the
// kind declares and the entry carries — and asserts that nothing fails except
// an explicit bulk-band refusal. The default layer must yield bytes for every
// entry, including the ones whose content lives with the producer (a skill's
// markdown reaches the hub's Read route).
//
// This is goals G28's probe until the GUI has browser automation, and it is
// also G5 probe (b) and G11 probe (b) over the wire: six real sidecars behind
// a real hub, one universe, every kind.
//
// When bin/ktw-host is built, the universe also serves the e19 files-only pack
// — a kind declared by files, with no code anywhere for it (ms-contribution-ext
// e22). The pane has one detail view for every kind (meta card, kind card,
// layers table, one tab per layer through OpenContributionLayer); the walk
// proves that view reaches the pack's kind like any other: every layer its
// declaration names opens, with zero refusals.
func TestGUIWalk_EveryEntryOfEveryKindOpens(t *testing.T) {
	long.Test(t)
	sender, cat, packed := startGUIWalkUniverse(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	if packed {
		waitListed(t, sender, []string{contribapi.ResourceTypeContributionType, fixtureKind(t)},
			[]string{fixtureKindURI, fixtureEntryURI}, 45*time.Second)
	}

	entries, err := mbapi.Query(ctx, q)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("empty universe")
	}
	kinds, err := mbapi.KindsAt(ctx, q)
	if err != nil {
		t.Fatalf("kind registry: %v", err)
	}

	var failures []string
	fail := func(uri, what string, err error) {
		failures = append(failures, fmt.Sprintf("  %s: %s: %v", uri, what, err))
	}
	opened, refused, nonDefault := 0, 0, 0
	byKind := map[string]int{}
	manifests := map[string]bool{}
	// Per layer of the fixture entry: "opened", or why it was not.
	packLayers := map[string]string{}

	for _, e := range entries {
		uri := e.ResolvedURI()
		byKind[e.Type]++

		// The envelope, addressed — what the detail pane shows first.
		if _, gerr := mbapi.Get(ctx, q, uri); gerr != nil {
			fail(uri, "envelope", gerr)
			continue
		}
		// Its kind is declared (or is the meta-model's own base case).
		decl, declared := kinds.Get(e.Type)
		if !declared && !contribapi.IsMetaKind(e.Type) {
			fail(uri, "kind", fmt.Errorf("%q has no declaration in this universe", e.Type))
			continue
		}
		// Its provider has a manifest with identity lines. `system` is the
		// meta-model every binary embeds — not a sidecar, so no manifest of
		// its own; its identity is immutable by construction.
		if e.Provider != contribapi.SystemProvider && !manifests[e.Provider] {
			m, merr := cat.Manifest(ctx, e.Provider)
			if merr != nil {
				fail(uri, "provider manifest", merr)
			} else if len(m.Providers) == 0 {
				fail(uri, "provider identity", fmt.Errorf("manifest of %s records no provider identities", e.Provider))
			}
			manifests[e.Provider] = true
		}

		// Every layer the kind declares AND the entry carries opens, or is
		// refused for the one reason the model allows.
		def := kinds.DefaultLayer(e.Type)
		for name, ref := range e.Layers {
			if declared {
				if _, ok := decl.Layers[name]; !ok {
					fail(uri, "layer "+name, fmt.Errorf("carried but not declared by kind %q", e.Type))
				}
			}
			rc, got, oerr := cat.OpenLayer(ctx, uri, name)
			if uri == fixtureEntryURI {
				packLayers[name] = "not verified"
				if oerr != nil {
					packLayers[name] = oerr.Error()
				}
			}
			if oerr != nil {
				switch {
				case ref.IsBulk() && strings.Contains(oerr.Error(), "bulk"):
					refused++ // the bulk band: streamed out of band by hash, never on the control plane
				case name != def && strings.Contains(oerr.Error(), "not the kind's default layer"):
					// A non-default layer the aggregator does not hold: refused by
					// name until the provider contract can serve layers
					// (idea: provider-layer-contract). Explicit, so allowed.
					nonDefault++
				default:
					fail(uri, "open "+name, oerr)
				}
				continue
			}
			b, rerr := io.ReadAll(rc)
			_ = rc.Close()
			if rerr != nil {
				fail(uri, "read "+name, rerr)
				continue
			}
			if got.Hash != ref.Hash || contribapi.HashContent(b) != ref.Hash {
				fail(uri, "layer "+name, fmt.Errorf("content does not match the index (index %s, opened %s, bytes %s)", ref.Hash, got.Hash, contribapi.HashContent(b)))
				continue
			}
			if name == def && len(b) == 0 {
				fail(uri, "default layer "+name, fmt.Errorf("opened to zero bytes"))
				continue
			}
			opened++
			if uri == fixtureEntryURI {
				packLayers[name] = "opened"
			}
		}
	}

	if packed {
		assertPackEntryOpened(t, kinds, packLayers)
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		if len(failures) > 60 {
			failures = append(failures[:60], fmt.Sprintf("  … and %d more", len(failures)-60))
		}
		t.Errorf("GUI walk: %d failure(s) over %d entries:\n%s", len(failures), len(entries), strings.Join(failures, "\n"))
	}
	kindNames := make([]string, 0, len(byKind))
	for k := range byKind {
		kindNames = append(kindNames, k)
	}
	sort.Strings(kindNames)
	t.Logf("GUI walk: %d entries across %d kinds (%s) · %d layers opened · %d bulk refusals · %d non-default layers refused by name · %d providers",
		len(entries), len(byKind), strings.Join(kindNames, ", "), opened, refused, nonDefault, len(manifests))
}
