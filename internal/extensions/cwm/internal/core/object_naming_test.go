// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// A long title is cut at a word boundary: the file name never ends on half a word.
func TestSlugOfCutsOnAWordBoundary(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Short title", "short-title"},
		{"What carries the messages between the hub, the sidecars and the GUI?", "what-carries-the-messages-between-the-hub-the"},
		// The limit falling just before a dash keeps the whole word.
		{"aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeee ffff", "aaaaaaaaaa-bbbbbbbbbb-cccccccccc-dddddddddd-eeee"},
		// One word longer than the limit is cut where the limit falls.
		{"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuv"},
		{"?!", ""},
	}
	for _, c := range cases {
		got := slugOf(c.title)
		if got != c.want {
			t.Errorf("slugOf(%q) = %q, want %q", c.title, got, c.want)
		}
		if len(got) > slugOfMax {
			t.Errorf("slugOf(%q) is %d long, over %d", c.title, len(got), slugOfMax)
		}
	}
}
