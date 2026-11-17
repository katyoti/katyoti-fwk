// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A reaction id reads back whole: the subscription id holds `:` `/` `#`, the
// event id follows the last `@`.
func TestReactionIDRoundTrips(t *testing.T) {
	r := ReactionID{Subscription: SubscriptionID("hook:ktw-x/on-create", HookOnCreate, "ktw-x:react/it"), EventID: "01a11634-215c-7166-b75e-6529c32cfac6"}
	got, ok := ParseReactionID(r.String())
	if !ok || got != r {
		t.Fatalf("ParseReactionID(%q) = %+v, %v", r.String(), got, ok)
	}
	for _, bad := range []string{"", "no-at", "@e", "s@"} {
		if _, ok := ParseReactionID(bad); ok {
			t.Errorf("ParseReactionID(%q) accepted", bad)
		}
	}
	if d := (AsyncSpec{}).WithDefaults(); d.Priority != PriorityP2 || d.Key != "${id}" || d.Attempts != 3 || d.AcceptsCaused {
		t.Errorf("defaults = %+v", d)
	}
}
