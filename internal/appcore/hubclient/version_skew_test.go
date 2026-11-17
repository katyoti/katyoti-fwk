// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"strings"
	"testing"
)

// A hub reused at another version than the client's is reported, naming both;
// an unknown or dev version on either side is not.
func TestHubVersionSkew(t *testing.T) {
	if s := HubVersionSkew("v1.4.0", "v1.3.0"); !strings.Contains(s, "v1.3.0") || !strings.Contains(s, "v1.4.0") {
		t.Fatalf("skew = %q, want both versions named", s)
	}
	for _, c := range [][2]string{{"v1", "v1"}, {"", "v1"}, {"v1", ""}, {"dev", "v1"}, {"v1", "dev"}} {
		if s := HubVersionSkew(c[0], c[1]); s != "" {
			t.Errorf("HubVersionSkew(%q, %q) = %q, want none", c[0], c[1], s)
		}
	}
}
