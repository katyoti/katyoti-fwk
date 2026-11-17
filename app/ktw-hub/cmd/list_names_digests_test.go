// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// ListNames carries the aggregator's manifest hash per cached member — what a
// consumer's view is compared against on reconnect. A member without an
// inventory entry, or one whose manifest has no hash, is ABSENT from the map
// rather than present with "", so the consumer can tell "the aggregator holds
// nothing for it" from "it holds something it cannot name".
func TestListNames_DigestsNameEveryCachedMemberAndOnlyThose(t *testing.T) {
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{
		{Provider: "ktw-methods", Hash: "sha256:m1"},
		{Provider: "ktw-code", Hash: "sha256:c1"},
		{Provider: "ktw-nohash"},
	}, []string{"ktw-uncached"})

	names, err := cat.Names(context.Background())
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	got := manifestDigests(context.Background(), cat, names)
	if got["ktw-methods"] != "sha256:m1" || got["ktw-code"] != "sha256:c1" {
		t.Errorf("digests = %v, want the two hashed members", got)
	}
	if _, present := got["ktw-nohash"]; present {
		t.Error("a manifest without a hash must be absent, not empty")
	}
	if _, present := got["ktw-uncached"]; present {
		t.Error("a member with no inventory entry must be absent")
	}
}
