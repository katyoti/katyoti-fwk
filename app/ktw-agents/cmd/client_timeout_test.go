// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"
	"time"
)

// A client verb waits as long as its method advertises, else ten minutes;
// `action run` waits up to an hour, the serving side bounding the target by
// the target's own timeout - so a 16-minute prove-wait is never cut at ten.
func TestClientTimeout(t *testing.T) {
	for _, c := range []struct {
		slot, method string
		advertised   time.Duration
		want         time.Duration
	}{
		{"requests", "get", 0, clientCallTimeout},
		{"requests", "check", 30 * time.Minute, 30 * time.Minute},
		{"action", "run", 0, actionRunCeiling},
		{"action", "run", 2 * time.Minute, actionRunCeiling},
	} {
		if got := clientTimeout(c.slot, c.method, c.advertised); got != c.want {
			t.Errorf("%s/%s advertising %s: %s, want %s", c.slot, c.method, c.advertised, got, c.want)
		}
	}
	if actionRunCeiling <= 16*time.Minute {
		t.Errorf("action run waits %s: less than a sidecar's longest prove-wait (16 min)", actionRunCeiling)
	}
}
