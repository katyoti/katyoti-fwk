// SPDX-License-Identifier: Apache-2.0

package local

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// A RUN ON AN OBJECT RECEIVES ITS CONTEXT: the object-context plugin is
// attached when the run has a target and the binary registers it — once,
// whatever the task lists, and the task's refs are never changed in place.
func TestARunOnAnObjectGetsTheObjectContextPlugin(t *testing.T) {
	has := func(s string) bool { return s == api.ObjectContextPlugin }
	none := func(string) bool { return false }
	target := map[string]any{api.PluginContextTarget: "billing.g1"}
	task := []api.PluginRef{{Type: "cwm"}}

	got := withObjectContext(task, target, has)
	if len(got) != 2 || got[1].Type != api.ObjectContextPlugin || len(task) != 1 {
		t.Fatalf("with a target: %+v (task refs %+v)", got, task)
	}
	if again := withObjectContext(got, target, has); len(again) != 2 {
		t.Errorf("attached twice: %+v", again)
	}
	for name, out := range map[string][]api.PluginRef{
		"no target":           withObjectContext(task, nil, has),
		"an empty target":     withObjectContext(task, map[string]any{api.PluginContextTarget: ""}, has),
		"no such plugin here": withObjectContext(task, target, none),
	} {
		if len(out) != 1 {
			t.Errorf("%s: %+v, want the task's refs alone", name, out)
		}
	}
}
