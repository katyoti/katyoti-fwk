// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"testing"
	"time"
)

// A caller's wait for the hub to start a member outlasts the hub's own bound
// on that start - a member's declared readiness limit (ktw-agents: 1 min)
// plus the short start back-off the hub waits out - so a member still
// coming up is waited for, not reported as a request timeout.
func TestEnsureSpawnTimeout_OutlastsTheHubsStart(t *testing.T) {
	const hubBound = time.Minute + 10*time.Second
	if ensureSpawnTimeout <= hubBound {
		t.Fatalf("ensureSpawnTimeout = %s, want more than the hub's bound on a start (%s)", ensureSpawnTimeout, hubBound)
	}
}
