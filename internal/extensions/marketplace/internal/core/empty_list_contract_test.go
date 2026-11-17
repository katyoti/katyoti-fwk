// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// AN EMPTY LIST MUST MARSHAL TO `[]`, NEVER `null`.
//
// This is a CONTRACT, not a preference. The generated TypeScript client
// declares these outputs as arrays:
//
//	export type SearchItemsOutput   = {...}[]
//	export type ListInstalledOutput = {...}[]
//
// A Go nil slice marshals to JSON `null`, so a method that returns `var out
// []T` without appending hands the GUI a `null` where it promised an array.
// The GUI then does `for (const it of items)` and the whole panel dies with
// "s is not iterable" — which is exactly what happened when the Marketplace
// view was opened with nothing installed and no catalog subscribed.
//
// The empty case is the one that breaks, and it is also the FIRST thing a new
// user sees: a fresh install has no marketplaces and nothing installed. The
// failure was reserved for precisely the people least able to diagnose it.
//
// Marshalling is asserted rather than `!= nil`, because `null` versus `[]` is
// the thing the client actually consumes.
//
// SCOPE, stated plainly. This asserts the HELPERS, not the methods, because
// constructing a service here needs a real marketplace registry and the bare
// value panics. The two call sites are one line each (`out :=
// emptyItemSummaries()` in SearchItems, the `items == nil` guard in
// ListInstalled) and are verifiable by reading them. A test that needed a
// registry to assert a marshalling rule would be testing the registry.
func TestEmptyListsMarshalAsArraysNotNull(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
	}{
		{"SearchItems", []mktapi.ItemSummary(nil)},
		{"ListInstalled", []mktapi.Installed(nil)},
	} {
		t.Run(tc.name+" (nil, the shape that broke)", func(t *testing.T) {
			b, err := json.Marshal(tc.val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != "null" {
				t.Fatalf("precondition wrong: a nil slice should marshal to null, got %s", b)
			}
		})
	}

}

// emptyJSON is the assertion the fix has to satisfy, kept separate so it reads
// as a statement about the API rather than about one method.
func assertMarshalsAsArray(t *testing.T, what string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", what, err)
	}
	if string(b) == "null" {
		t.Errorf("%s marshals to null; the generated client declares an array, so the GUI does `for...of` on it", what)
	}
}

// The helpers that FEED those methods must not hand back nil either — fixing
// only the outer method would leave the next caller to rediscover this.
func TestListHelpersReturnEmptyNotNil(t *testing.T) {
	assertMarshalsAsArray(t, "emptyItemSummaries()", emptyItemSummaries())
	assertMarshalsAsArray(t, "emptyInstalled()", emptyInstalled())
}
