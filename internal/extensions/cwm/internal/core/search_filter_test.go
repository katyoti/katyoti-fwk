// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectSearch narrows by an ObjectFilter after its own filters and the query,
// in both modes — what a reference field's picker searches with: its types,
// and a `when` over the object vocabulary.
func TestObjectSearchTakesAnObjectFilter(t *testing.T) {
	svc, ctx := fulltextFixture(t)
	ids := func(out api.ObjectSearchOutput) string {
		var s []string
		for _, o := range out.Objects {
			s = append(s, o.ID)
		}
		return strings.Join(s, ",")
	}
	got, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{Filter: &api.ObjectFilter{Types: []string{"memo"}}})
	if err != nil || ids(got) != "m.memo" || got.Total != 1 {
		t.Fatalf("types filter: %s (%d), %v", ids(got), got.Total, err)
	}
	got, err = svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "re", Filter: &api.ObjectFilter{When: "parent_id == 'm.root'"}})
	if err != nil || ids(got) != "m.body" {
		t.Fatalf("query then when: %s, %v — want only the child of m.root matching \"re\"", ids(got), err)
	}
	got = ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache", Filter: &api.ObjectFilter{Types: []string{"note"}, When: "id != 'm.cache'"}})
	if hitIDs(got) != "m.body" {
		t.Fatalf("fulltext + filter: %s", hitIDs(got))
	}
	// A `when` that cannot be read admits nothing.
	got, _ = svc.ObjectSearch(ctx, api.ObjectSearchInput{Filter: &api.ObjectFilter{When: "nope("}})
	if len(got.Objects) != 0 || got.Capped {
		t.Fatalf("an unreadable when: %s capped=%v", ids(got), got.Capped)
	}
}
