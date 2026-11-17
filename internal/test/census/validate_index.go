// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// indexOverheadPerEntry is what one wire row costs beyond its inline data and
// its metadata: URI, provider, type, id, the layer descriptors (name, hash,
// size, media type per layer), provenance and the verdict. Measured 2026-09-25
// over the 21 goldens: the largest descriptor-only row is under 700 B; 1 KiB
// leaves room for a fourth layer without hiding a body that leaked inline.
const indexOverheadPerEntry = 1024

// indexEnvelope is the cost of an index with nothing in it (the JSON array
// itself); a binary that publishes no entries (ktw-test) has a 2-byte index
// and a ceiling that must not read zero.
const indexEnvelope = 64

// assertIndexWithinBands: the index a binary puts on the wire costs what its
// entry count explains, never what its content weighs (goal G13).
//
// Two guards, both over the WIRE shape (every layer above the inline band
// dropped, as the hub's stripBodies does before answering the index listing,
// CatalogService/ListContributions):
//
//  1. no single row carries inline bytes above the inline band — a body that
//     leaked past DropInline is the one way a large layer reaches the index;
//  2. the whole index is at most entries × (meta budget + inline band +
//     overhead) — a ceiling every entry can reach and none can exceed, so the
//     total grows with the count and only with the count.
//
// The per-binary size is logged so the census RECORDS it (the probe agreed in
// goals.md); it is not part of the golden because a byte count churns on every
// content edit and would drown the diff that matters.
func assertIndexWithinBands(t *testing.T, name string, kinds contribapi.Kinds, m *mbapi.Manifest) {
	t.Helper()
	wire := make([]contribapi.Contribution, len(m.Contributions))
	copy(wire, m.Contributions)
	var over []string
	ceiling := 0
	for i := range wire {
		wire[i].DropInline() // rebinds a fresh map; the census manifest is untouched
		for layer, l := range wire[i].Layers {
			if l.Inline != nil && len(l.Inline) > contribapi.BandInline {
				over = append(over, fmt.Sprintf("  %s: layer %q carries %d inline bytes on the wire, band is %d",
					wire[i].ResolvedURI(), layer, len(l.Inline), contribapi.BandInline))
			}
		}
		budget := kinds.Budget(wire[i].Type)
		if budget <= 0 {
			budget = 128 // an undeclared budget is assertEveryKindIsComplete's finding; keep the ceiling honest
		}
		ceiling += budget + contribapi.BandInline + indexOverheadPerEntry
	}
	ceiling += indexEnvelope
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("census[%s]: marshal wire index: %v", name, err)
	}
	t.Logf("census[%s]: wire index %d B for %d entries (ceiling %d B, %.0f%% used)",
		name, len(raw), len(wire), ceiling, 100*float64(len(raw))/float64(max(ceiling, 1)))
	if len(raw) > ceiling {
		over = append(over, fmt.Sprintf("  whole index: %d B for %d entries, ceiling %d B — the index grew beyond what the entry count explains",
			len(raw), len(wire), ceiling))
	}
	if len(over) == 0 {
		return
	}
	sort.Strings(over)
	t.Errorf("census[%s]: the wire index is not bounded by its entry count (G13):\n%s\n\n"+
		"A layer above the inline band must not be inline on the wire; metadata must stay within the kind's budget.",
		name, strings.Join(over, "\n"))
}
