// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The delivery gate's « dropped » reads an increment by its release state,
// an object with a lifecycle by that lifecycle only, and a bare request by
// its plan: a done feature whose round was dropped is not dropped, a dropped
// feature is dropped whatever its round says (review).
func TestDroppedObjectReadsTheLifecycleBeforeTheRound(t *testing.T) {
	obj := func(typ, lifecycle, plan string) cwmapi.Object {
		o := cwmapi.Object{Type: typ, Capabilities: map[string]map[string]any{}}
		if lifecycle != "" {
			o.Capabilities[cwmapi.CapLifecycle] = map[string]any{"state": lifecycle}
		}
		if plan != "" {
			o.Capabilities[api.CapRequest] = map[string]any{}
			o.Capabilities[api.CapPlan] = map[string]any{"state": plan}
		}
		return o
	}
	for _, c := range []struct {
		name string
		o    cwmapi.Object
		want bool
	}{
		{"a dropped increment", obj(api.IncrementType, api.IncrementDropped, ""), true},
		{"an increment whose proposal was dropped", obj(api.IncrementType, "open", api.PlanDropped), false},
		{"a dropped request", obj("request", "", api.PlanDropped), true},
		{"a done request", obj("request", "", api.PlanDone), false},
		{"a done feature whose round was dropped", obj("feature", api.StateDone, api.PlanDropped), false},
		{"a dropped feature whose round is done", obj("feature", api.StateDropped, api.PlanDone), true},
		{"a dropped feature with no round", obj("feature", api.StateDropped, ""), true},
		{"a feature in idea with no round", obj("feature", "idea", ""), false},
	} {
		if got := droppedObject(c.o); got != c.want {
			t.Errorf("%s: droppedObject = %v, want %v", c.name, got, c.want)
		}
	}
}
