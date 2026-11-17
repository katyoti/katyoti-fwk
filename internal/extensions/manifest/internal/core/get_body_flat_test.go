// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// flatCatalog seeds one manifest of n data-bearing entries and loads it.
func flatCatalog(t *testing.T, n int) *CatalogImpl {
	t.Helper()
	cache := NewCache(t.TempDir())
	m := makeManifest("flat", "1.0.0", "sha256:flat")
	m.Contributions = make([]contribapi.Contribution, 0, n)
	for i := 0; i < n; i++ {
		c := contribapi.Contribution{Provider: "flat", Type: "capability", ID: fmt.Sprintf("entry-%06d", i)}
		c.SetData(fmt.Appendf(nil, `{"id":"entry-%06d"}`, i))
		m.Contributions = append(m.Contributions, c)
	}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "flat"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return cat
}

// G17 — lookup cost is independent of catalog size. GetBody and
// ContributionAt answer from the by-URI index built at load; they used to
// walk a manifest's entries and, on a miss, every manifest, so a read of the
// LAST entry cost the whole catalog. The probe is the same read at two sizes
// a hundredfold apart, timed in the same process under the same load: a
// scan would show a ratio near 100, a lookup near 1. The bound of 8 leaves
// room for allocator and cache noise on a loaded machine without letting a
// linear walk through (measured 2026-09-22: ratio 1.01 indexed, 84.9
// with the scan that preceded it).
func TestGetBody_CostIsFlatAcrossCatalogSize(t *testing.T) {
	const small, large = 100, 10_000
	last := func(n int) string { return fmt.Sprintf("flat:/capability/entry-%06d", n-1) }
	ctx := context.Background()

	time := func(cat *CatalogImpl, uri string) float64 {
		r := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := cat.GetBody(ctx, uri); err != nil {
					b.Fatalf("GetBody: %v", err)
				}
				if _, ok := cat.ContributionAt(uri); !ok {
					b.Fatal("ContributionAt: not found")
				}
			}
		})
		return float64(r.NsPerOp())
	}
	smallCat, largeCat := flatCatalog(t, small), flatCatalog(t, large)
	// One warm-up each so page faults and the first map access are not the
	// measurement.
	time(smallCat, last(small))
	time(largeCat, last(large))

	ns100, ns10k := time(smallCat, last(small)), time(largeCat, last(large))
	ratio := ns10k / ns100
	t.Logf("last-entry read: %d entries %.0f ns/op, %d entries %.0f ns/op, ratio %.2f", small, ns100, large, ns10k, ratio)
	if ratio > 8 {
		t.Fatalf("lookup cost grew with catalog size: ratio %.1f between %d and %d entries (a flat lookup stays near 1)", ratio, small, large)
	}
}

// The index must also answer for an entry whose URI provider differs from
// the manifest that carries it (a layer provider), and refuse cleanly what it
// does not hold — the two cases the deleted scan existed for.
func TestGetBody_IndexPlacesLayerProvidersAndRefusesUnknowns(t *testing.T) {
	cache := NewCache(t.TempDir())
	m := makeManifest("host", "1.0.0", "sha256:host")
	layered := contribapi.Contribution{Provider: "layer-pack", Type: "persona", ID: "reviewer"}
	layered.SetData([]byte(`{"name":"reviewer"}`))
	m.Contributions = []contribapi.Contribution{layered}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "host"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	body, err := cat.GetBody(context.Background(), "layer-pack:/persona/reviewer")
	if err != nil || string(body) != `{"name":"reviewer"}` {
		t.Fatalf("layer provider's entry via its own URI: body=%q err=%v", body, err)
	}
	if got := cat.HostOf(contribapi.ResourceURI{Provider: "layer-pack", Type: "persona", ID: "reviewer"}); got != "host" {
		t.Fatalf("HostOf = %q, want the carrying manifest host", got)
	}
	if _, err := cat.GetBody(context.Background(), "host:/persona/nobody"); err == nil {
		t.Fatal("an unknown entry of a known member must be an error, not (nil, nil)")
	}
	if _, err := cat.GetBody(context.Background(), "stranger:/persona/reviewer"); err == nil {
		t.Fatal("an unknown provider must be an error")
	}
}
