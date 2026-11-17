// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
)

// TestUniverseKindsDeclareTheBuiltinKinds: with no member, the kinds a folder
// member's files may be are every kind compiled into the hub — builtin.Kinds,
// skill, task and persona among them.
func TestUniverseKindsDeclareTheBuiltinKinds(t *testing.T) {
	want, err := builtin.Kinds()
	if err != nil {
		t.Fatalf("builtin.Kinds: %v", err)
	}
	got := universeKinds(nil, nil)
	for _, n := range []string{"skill", "task", "persona"} {
		if _, ok := got.Get(n); !ok {
			t.Errorf("universeKinds does not declare %q", n)
		}
	}
	for _, n := range want.Names() {
		if _, ok := got.Get(n); !ok {
			t.Errorf("universeKinds misses the built-in kind %q", n)
		}
	}
}
