// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"bytes"
	"testing"
)

// bigBody is above the inline band, so DropInline actually drops it. A body
// inside the inline band is KEPT by design, and a test built on one would
// pass without exercising anything.
func bigBody() []byte { return bytes.Repeat([]byte("x"), int(BandInline)+1) }

// TestDropInline_DoesNotMutateTheSource pins the invariant four call sites
// already assumed and none of them had: taking a Contribution by value and
// calling DropInline on the copy must not disturb the original.
//
// `Layers` is a map, so a struct copy shares it. Every "copy then drop" site
// was therefore reaching into the cache it was copying to protect — the hub's
// own comment said "mutating in place would corrupt the cache, so we copy each
// entry before nilling Body", and the copy did not do that.
//
// The user-visible consequence: listing the index (which drops inline bytes on
// the way out) destroyed the bodies in the live catalog, so every later
// GetBody for a contribution above the inline band returned empty.
func TestDropInline_DoesNotMutateTheSource(t *testing.T) {
	orig := Contribution{Provider: "p", Type: "task", ID: "t"}
	orig.SetData(bigBody())
	wantHash := orig.DataHash()
	if len(orig.Data()) == 0 {
		t.Fatal("setup: original must carry inline data")
	}

	// Exactly what every call site does: range by value, drop on the copy.
	cp := orig
	cp.DropInline()

	if len(cp.Data()) != 0 {
		t.Error("the copy must have its inline bytes dropped")
	}
	if len(orig.Data()) == 0 {
		t.Fatal("dropping inline bytes on a COPY emptied the ORIGINAL — " +
			"Layers is a map, so a struct copy shares it")
	}
	if orig.DataHash() != wantHash {
		t.Errorf("original hash changed: %s -> %s", wantHash, orig.DataHash())
	}
}

// TestDropInline_KeepsTheDescriptor: dropping bytes must not drop the hash.
// The index plane needs hash-without-bytes; a drop that removed the descriptor
// would make the contribution look bodyless and GetBody would never be called.
func TestDropInline_KeepsTheDescriptor(t *testing.T) {
	c := Contribution{Provider: "p", Type: "task", ID: "t"}
	c.SetData(bigBody())
	want := c.DataHash()
	c.DropInline()
	if c.DataHash() != want {
		t.Errorf("DataHash lost on drop: %q, want %q", c.DataHash(), want)
	}
}

// TestDropInline_RepeatedIsStable — the hub drops on every list. Doing it twice
// must be the same as doing it once.
func TestDropInline_RepeatedIsStable(t *testing.T) {
	c := Contribution{Provider: "p", Type: "task", ID: "t"}
	c.SetData(bigBody())
	c.DropInline()
	after := c.DataHash()
	c.DropInline()
	if c.DataHash() != after {
		t.Errorf("second drop changed the descriptor: %q -> %q", after, c.DataHash())
	}
}
