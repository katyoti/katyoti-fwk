// SPDX-License-Identifier: Apache-2.0

package ktsp

import "testing"

// TestNegotiateProtocol: within-range (incl. the baseline for a zero/unstamped
// version) is accepted; anything above current or below min is refused
// (fail-closed).
func TestNegotiateProtocol(t *testing.T) {
	cases := []struct {
		name string
		peer int
		ok   bool
	}{
		{"current", KTSPProtocolVersion, true},
		{"min", KTSPProtocolMinSupported, true},
		{"zero-is-baseline", 0, true}, // unstamped descriptor → baseline major (1)
		{"above-current", KTSPProtocolVersion + 1, false},
	}
	// A genuinely below-min peer is only representable once min > 1 (0 means
	// "unstamped = baseline", not "too old"). Exercise it when the range allows.
	if KTSPProtocolMinSupported > 1 {
		cases = append(cases, struct {
			name string
			peer int
			ok   bool
		}{"below-min", KTSPProtocolMinSupported - 1, false})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := NegotiateProtocol(c.peer)
			if c.ok && err != nil {
				t.Fatalf("peer=%d should be accepted, got: %v", c.peer, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("peer=%d should be refused (fail-closed), got nil", c.peer)
			}
		})
	}
}
