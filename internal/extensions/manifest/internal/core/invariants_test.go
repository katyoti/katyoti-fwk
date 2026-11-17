// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// invariants_test.go — I1, I2 and I4, asserted rather than inspected.
//
// These are load-bearing properties: a decision that breaks one is wrong by
// construction. They were argued from the code's shape, which is exactly the
// kind of claim that stops being true quietly — someone adds a fetch on a miss
// and every listing starts contacting a sidecar again, and nothing says so.
//
// The mechanism here is the sharpest one available: the catalog is wired with a
// content lane whose fetcher FAILS THE TEST if it is ever called. A code path
// that reached for a sidecar could not complete; it would name itself.

// universeOf builds a body-free index — what a consumer actually receives —
// over a fetcher that must never run.
func universeOf(t *testing.T, n int) *core.MemoryCatalog {
	t.Helper()
	entries := make([]contribapi.Contribution, 0, n)
	for i := 0; i < n; i++ {
		body, err := json.Marshal(map[string]string{"requires": "none"})
		if err != nil {
			t.Fatal(err)
		}
		c := contribapi.Contribution{
			Provider: "ktw-docs", Type: "skill",
			ID:       "skill-" + strings.Repeat("x", 1) + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Metadata: &contribapi.ContributionMeta{Name: "a skill", Description: "d"},
		}
		c.SetData(body)
		c.DropInline() // the index plane: exactly what crosses the wire
		entries = append(entries, c)
	}
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{
		Provider: "ktw-docs", Contributions: entries,
	}}, nil)
	cat.SetContentLane(core.NewContentStore(t.TempDir()),
		func(context.Context, string, string) ([]byte, error) {
			t.Fatal("I1/I2 violated: this path contacted a contributor")
			return nil, nil
		})
	return cat
}

// I1 — discovering what the universe offers must not contact any sidecar.
func TestI1_DiscoveryContactsNoContributor(t *testing.T) {
	cat := universeOf(t, 182)
	got, err := mbapi.Query(context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if len(got) != 182 {
		t.Fatalf("discovered %d entries, want 182", len(got))
	}
	// And it discovered something USEFUL without reaching anything: the picker
	// fields are resident, which is the reason discovery can be free at all.
	for _, c := range got {
		if c.ResolvedMetadata().Name == "" {
			t.Fatalf("%s discovered without a name", c.ResolvedURI())
		}
	}
}

// I2 — `list` never contacts a sidecar. `read` is the deliberate exception and
// is not exercised here, precisely because it IS allowed to.
func TestI2_ListContactsNoContributor(t *testing.T) {
	cat := universeOf(t, 50)
	names, err := cat.Names(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("list returned nothing")
	}
	if _, err := cat.GetContributionsByType(context.Background(), "skill"); err != nil {
		t.Fatalf("list by kind failed: %v", err)
	}
}

// I4 — the wire is body-free: an index entry carries the descriptor and not the
// content, so payload size stays independent of pack size.
func TestI4_TheWireIsBodyFree(t *testing.T) {
	cat := universeOf(t, 182)
	entries, err := cat.GetContributionsByType(context.Background(), "skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range entries {
		if c.DataHash() == "" {
			t.Fatalf("%s crossed the wire with no descriptor — its content is unreachable", c.ResolvedURI())
		}
	}
	// Payload independent of content: the whole set encodes small.
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if perEntry := len(raw) / len(entries); perEntry > 512 {
		t.Errorf("index entry averages %d bytes; the wire is carrying content", perEntry)
	}
}

// I5 — the cache is reconstructible and disposable: a consumer whose content
// store is empty still lists everything, because listing never needed it.
func TestI5_AnEmptyCacheStillLists(t *testing.T) {
	cat := universeOf(t, 20) // a fresh TempDir store, never written to
	got, err := mbapi.Query(context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil || len(got) != 20 {
		t.Fatalf("listing with an empty cache: %d entries, %v", len(got), err)
	}
}
