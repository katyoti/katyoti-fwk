// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// TestParseToolPattern locks the wildcard-parse fix: a glob METHOD must parse
// (ParseToolURI rejects it as non-kebab), while structural tokens stay strict.
func TestParseToolPattern(t *testing.T) {
	// glob method survives, returned verbatim
	u, err := ParseToolPattern("ktw-example:leanix/*")
	if err != nil {
		t.Fatalf("glob method must parse: %v", err)
	}
	if u.Provider != "ktw-example" || u.SlotType != "leanix" || u.Method != "*" || u.Instance != "default" {
		t.Errorf("bad glob parse: %+v", u)
	}
	// prefix glob
	if _, err := ParseToolPattern("ktw-weather:weather-service/get-*"); err != nil {
		t.Errorf("prefix glob must parse: %v", err)
	}
	// exact method still parses + validates (identical to ParseToolURI)
	if _, err := ParseToolPattern("ktw-example:leanix/search"); err != nil {
		t.Errorf("exact method must parse: %v", err)
	}
	// ParseToolURI MUST still reject the glob (strict contract unchanged)
	if _, err := ParseToolURI("ktw-example:leanix/*"); err == nil {
		t.Error("ParseToolURI must still reject a glob method")
	}
	// structural tokens stay strict even with a glob method
	if _, err := ParseToolPattern("ktw-example:Leanix/*"); err == nil {
		t.Error("non-kebab slot must be rejected even with a glob method")
	}
}
