// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A theme's files are named inside its folder: a declaration edited on the
// object cannot reach the module's other files.
func TestSafeAssetPath(t *testing.T) {
	for p, want := range map[string]bool{
		"logo.png": true, "bg/sand.jpg": true, "icons/a-b.svg": true,
		"../notes/secret.md": false, "bg/../../x": false, "/etc/x": false,
		`bg\x.png`: false, "C:x": false, "": false, "./logo.png": false, "bg//x": false,
	} {
		if got := safeAssetPath(p); got != want {
			t.Errorf("safeAssetPath(%q) = %v, want %v", p, got, want)
		}
	}
}
