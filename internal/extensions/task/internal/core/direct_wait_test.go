// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A run waiting inside a tool call for a person at a live UI gives its group
// slot back while the person answers — another run may run meanwhile — and
// takes it again after.
func TestAWaitForAPersonGivesTheSlotBack(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	if err := g.admit("r1", "g", 0); err != nil {
		t.Fatal(err)
	}
	if err := g.acquire(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	m := newSuspensionManager()
	m.slots = g
	during := -1
	if err := m.WhileWaiting(context.Background(), "r1", func() { during, _ = g.load("g") }); err != nil {
		t.Fatal(err)
	}
	if during != 0 {
		t.Errorf("running while the person answers = %d, want 0 (the slot given back)", during)
	}
	if after, _ := g.load("g"); after != 1 {
		t.Errorf("running after = %d, want the slot taken again", after)
	}
}
