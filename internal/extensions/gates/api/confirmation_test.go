// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A confirmation item names the object, the gate, the requirement and the
// wait: it reads back whole, and any other item a human task satisfies (a
// verification, a step, a malformed one) is not a confirmation.
func TestAConfirmationItemNamesItsObjectGateRequirementAndWait(t *testing.T) {
	item := ConfirmationItem("roadmap.RL-033", "ready-to-release", "changelog-updated", "2026-10-24T07:00:00.5Z")
	if item != "gate:roadmap.RL-033/ready-to-release/changelog-updated@2026-10-24T07:00:00.5Z" {
		t.Fatalf("item = %q", item)
	}
	c, ok := ParseConfirmationItem(item)
	if !ok || c != (Confirmation{Object: "roadmap.RL-033", Gate: "ready-to-release", Requirement: "changelog-updated", Since: "2026-10-24T07:00:00.5Z"}) {
		t.Fatalf("read back = %+v, %v", c, ok)
	}
	for _, other := range []string{
		"ready-to-release/change-log-confirmed", // a bare requirement, no object nor wait
		"verification:mail.AC-015",
		"gate:roadmap.RL-033/ready-to-release@2026-10-24",        // no requirement
		"gate:roadmap.RL-033/ready-to-release/changelog-updated", // no wait
		"gate:/ready-to-release/changelog-updated@2026-10-24",    // no object
	} {
		if c, ok := ParseConfirmationItem(other); ok {
			t.Errorf("%q read as a confirmation: %+v", other, c)
		}
	}
}
