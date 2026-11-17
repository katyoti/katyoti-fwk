// SPDX-License-Identifier: Apache-2.0

package universe

import "testing"

// : the universe view asks the slots of members only - a
// layer provider (system, the universe's own) has no manifest at the
// aggregator, and asking it warned on every view built; when the membership
// is unknown, every provider is asked as before.
func TestTheViewAsksTheSlotsOfMembersOnly(t *testing.T) {
	members := map[string]bool{"ktw-agents": true, "ktw-methods": true}
	for _, c := range []struct {
		provider string
		want     bool
	}{{"ktw-agents", true}, {"ktw-methods", true}, {"system", false}, {"universe", false}} {
		if got := asksSlots(members, c.provider); got != c.want {
			t.Errorf("provider %s: asked %v, want %v", c.provider, got, c.want)
		}
	}
	if !asksSlots(nil, "system") {
		t.Error("membership unknown: a provider was not asked; want every provider asked")
	}
}
