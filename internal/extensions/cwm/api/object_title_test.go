// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// TestDisplayTitleOrder — an object is called by its name, else by its id.
func TestDisplayTitleOrder(t *testing.T) {
	o := Object{ID: "m.g1", Type: "goal"}
	if o.DisplayTitle() != "m.g1" {
		t.Errorf("bare object: %q, want the id", o.DisplayTitle())
	}
	o.Name = "One invoice run"
	if o.DisplayTitle() != "One invoice run" {
		t.Errorf("named object: %q", o.DisplayTitle())
	}
}
