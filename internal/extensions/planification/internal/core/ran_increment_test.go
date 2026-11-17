// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// An increment whose only request was dropped - an approval filed before any
// run - can be run again; one whose run is filed, running or done cannot
func TestAnIncrementWithADroppedRequestCanBeRunAgain(t *testing.T) {
	inc := func(plan string) cwmapi.Object {
		caps := map[string]map[string]any{}
		if plan != "" {
			caps[api.CapRequest] = map[string]any{}
			caps[api.CapPlan] = map[string]any{"state": plan}
		}
		return cwmapi.Object{ID: "roadmap.IN-1", Capabilities: caps}
	}
	for plan, ran := range map[string]bool{"": false, api.PlanDropped: false, api.PlanDone: true, api.PlanBlocked: true, api.PlanReady: true} {
		if got := ranIncrement(inc(plan)); got != ran {
			t.Errorf("plan %q: ranIncrement = %v, want %v", plan, got, ran)
		}
	}
}
