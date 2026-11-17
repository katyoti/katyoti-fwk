// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// inline_band_test.go — the band that was documented and not implemented.
//
// §3 says content at or under 4 KiB is "carried in the index". Until c22 the
// code had two bands and an envelope field called `Body` quietly playing the
// third. These tests are what makes the row in that table true.

type pickerBody struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Hashtags    []string `json:"hashtags,omitempty"`
}

// Listing a kind must fetch nothing per entry. This is the property c12
// measured and the one removing `Body` could have silently undone: if picker
// fields had moved to a fetched layer, every listing would pay a round trip per
// row for the thing listings exist to show.
func TestListingAKindFetchesNothingPerEntry(t *testing.T) {
	const n = 182
	entries := make([]contribapi.Contribution, 0, n)
	for i := 0; i < n; i++ {
		body, err := json.Marshal(pickerBody{
			Name: fmt.Sprintf("skill-%03d", i), Description: "a skill",
			Hashtags: []string{"a", "b"},
		})
		if err != nil {
			t.Fatal(err)
		}
		c := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: fmt.Sprintf("skill-%03d", i)}
		c.SetData(body)
		entries = append(entries, c)
	}
	// The index plane: exactly what a consumer receives over the wire.
	for i := range entries {
		entries[i].DropInline()
	}

	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: entries}}, nil)
	// No content lane AT ALL. A listing that needed one could not even run,
	// which is a sharper assertion than counting fetches.
	got, err := mbapi.QueryTyped[pickerBody](context.Background(),
		mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil {
		t.Fatalf("listing an index-plane kind failed: %v", err)
	}
	if len(got) != n {
		t.Fatalf("listed %d entries, want %d", len(got), n)
	}
	for _, e := range got {
		if e.Body.Name == "" || e.Body.Description == "" || len(e.Body.Hashtags) != 2 {
			t.Fatalf("an entry lost its picker fields: %+v", e.Body)
		}
	}
}

// Above the inline band, the index carries the descriptor and not the content.
// That is the same table read the other way, and it is what keeps an index
// bounded by the NUMBER of contributions rather than their size.
func TestContentAboveTheInlineBandLeavesTheIndex(t *testing.T) {
	small, _ := json.Marshal(pickerBody{Name: "small"})
	large, _ := json.Marshal(pickerBody{Name: "large", Description: strings.Repeat("x", 8<<10)})

	var a, b contribapi.Contribution
	a.SetData(small)
	b.SetData(large)
	if contribapi.BandOf(int64(len(small))) != contribapi.BandNameInline {
		t.Fatal("the small fixture is not in the inline band")
	}
	if contribapi.BandOf(int64(len(large))) == contribapi.BandNameInline {
		t.Fatal("the large fixture is in the inline band")
	}

	// A producer holds what it just built, whatever the band.
	if len(a.Data()) == 0 || len(b.Data()) == 0 {
		t.Fatal("a producer lost content it had in hand")
	}

	a.DropInline()
	b.DropInline()
	if len(a.Data()) == 0 {
		t.Error("inline-band content left the index; the band means it stays")
	}
	if len(b.Data()) != 0 {
		t.Error("content above the inline band is still resident")
	}
	// Both keep the descriptor. Losing that would turn "not carried right now"
	// into "there is nothing here", which is a different and wrong answer.
	if a.DataHash() == "" || b.DataHash() == "" {
		t.Error("dropping content also dropped the descriptor")
	}
	if got := b.Layers[contribapi.LayerData].Size; got != int64(len(large)) {
		t.Errorf("descriptor size = %d, want %d", got, len(large))
	}
}

// The hash is over the content, not over whether the content happens to be
// carried — so an index entry and the producer's entry address the same bytes.
func TestDroppingContentDoesNotChangeItsAddress(t *testing.T) {
	body, _ := json.Marshal(pickerBody{Name: "x", Description: strings.Repeat("y", 8<<10)})
	var c contribapi.Contribution
	c.SetData(body)
	before := c.DataHash()
	c.DropInline()
	if c.DataHash() != before {
		t.Errorf("address changed when the content stopped being carried: %s -> %s", before, c.DataHash())
	}
	if before != contribapi.HashContent(body) {
		t.Errorf("data layer hash %s is not the content's address", before)
	}
}
