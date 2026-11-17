// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// The reconciler's prepared decision on a wait is a
// card of « À faire »: its option labels are words of the page, French on the
// French page, while the choice sent stays the stored label.
func TestTheReconcilersWaitOptionsAreShownInThePagesLanguage(t *testing.T) {
	c := waitChoice("verify", "billing.RQ-2", "a wait on nobody", true, "billing.RQ-1")
	for _, o := range c.Options {
		if got := optionLabel(o.Label, "en"); got != o.Label {
			t.Errorf("English page: %q shown as %q", o.Label, got)
		}
		if got := optionLabel(o.Label, "fr"); got == o.Label {
			t.Errorf("French page: the option %q is shown in English", o.Label)
		}
	}
	if got := optionLabel("Keep yearly numbering", "fr"); got != "Keep yearly numbering" {
		t.Errorf("another decision's label %q is shown as %q; want it as written", "Keep yearly numbering", got)
	}
}
