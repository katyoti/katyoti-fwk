// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A background child's end is told to its parent (ChildEventsTopic) — what a
// parent awaiting its children as a machine waits on — and the parent can
// list its children to see which ended.
func TestAChildsEndIsToldToItsParent(t *testing.T) {
	ch := engine.NewChannel(engine.NewMemoryEventBus())
	told := make(chan struct{}, 2)
	ch.Bus().Subscribe(engine.ChildEventsTopic("parent-1"), func(string, any) { told <- struct{}{} })
	m := NewBackgroundTaskManager(ch).(*bgManager)
	m.entries["child-1"] = &bgEntry{record: taskapi.TaskRecord{ID: "child-1", ParentRunID: "parent-1", Status: taskapi.TaskCompleted}}
	m.entries["other"] = &bgEntry{record: taskapi.TaskRecord{ID: "other", ParentRunID: "parent-2"}}

	m.emitChildTerminal("child-1", "t", "parent-1", "")
	m.emitChildTerminal("child-1", "t", "parent-1", "boom")
	for i := 0; i < 2; i++ {
		select {
		case <-told:
		case <-time.After(time.Second):
			t.Fatalf("end %d not told to the parent", i+1)
		}
	}
	if kids := m.Children("parent-1"); len(kids) != 1 || kids[0].ID != "child-1" {
		t.Errorf("children = %+v; want the parent's one child", kids)
	}
}
