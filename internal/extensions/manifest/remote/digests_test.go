// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"strings"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// Digests is ListNames read for its hashes: one call, no entries, the map the
// consumer's reconnect check compares its view against.
func TestRemoteDigests_ReadsHashesFromListNames(t *testing.T) {
	f := &fakeSender{names: mbapi.ListNamesOutput{
		Names:  []string{"ktw-methods", "ktw-code", "ktw-uncached"},
		Hashes: map[string]string{"ktw-methods": "sha256:m1", "ktw-code": "sha256:c1"},
	}}
	got, err := manifestremote.New(f).Digests(context.Background())
	if err != nil {
		t.Fatalf("Digests: %v", err)
	}
	if len(got) != 2 || got["ktw-methods"] != "sha256:m1" || got["ktw-code"] != "sha256:c1" {
		t.Fatalf("Digests = %v, want the two hashed members", got)
	}
	if len(f.paths) != 1 || !strings.HasSuffix(f.paths[0], "/ListNames") {
		t.Fatalf("paths = %v, want one ListNames call", f.paths)
	}
}
