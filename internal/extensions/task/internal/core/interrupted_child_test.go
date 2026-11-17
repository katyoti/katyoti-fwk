// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A restored parent learns how its background child, interrupted by the
// restart, ended — by the id it knows it by: task-status says failed and why,
// its notification is queued, its children list it, await-child is woken.
// (After a restart the child was "not found".)
func TestAParentLearnsItsInterruptedBackgroundChild(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	first := durableService(t, dir)
	useStubborn(first, release)
	bgID, err := first.bgMgr.Register("parent-1", taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g", MaxDurationS: 600})
	if err != nil {
		t.Fatal(err)
	}
	var child runManifest
	eventually(t, "the child's run saved", func() bool {
		ms, _ := first.runs.manifests()
		for _, m := range ms {
			if m.Request.BackgroundID == bgID {
				child = m
				return true
			}
		}
		return false
	})
	began(t, first.runs, child.RunID)
	orphan(t, first.runs, child.RunID)

	second := durableService(t, dir)
	useStubborn(second, release)
	told := make(chan struct{}, 4)
	second.channel.Bus().Subscribe(engine.ChildEventsTopic("parent-1"), func(string, any) { told <- struct{}{} })
	second.RestoreRuns(ctx)

	rec, err := second.bgMgr.TaskStatus(bgID)
	if err != nil {
		t.Fatalf("task-status of the interrupted child: %v; want it known", err)
	}
	if rec.Status != taskapi.TaskFailed || !strings.Contains(rec.Error, "restart") {
		t.Errorf("child = %s %q; want failed, interrupted by the restart", rec.Status, rec.Error)
	}
	if ns := second.bgMgr.Notifications("parent-1"); len(ns) != 1 || ns[0].TaskID != bgID {
		t.Errorf("parent's notifications = %+v; want its child's end", ns)
	}
	if kids := second.bgMgr.Children("parent-1"); len(kids) != 1 {
		t.Errorf("parent's children = %+v", kids)
	}
	select {
	case <-told:
	case <-time.After(2 * time.Second):
		t.Error("the parent was not told its child ended")
	}
}

func TestARestoreDecidesChildrenFirst(t *testing.T) {
	ms := []runManifest{
		{RunID: "parent", Request: taskapi.TaskRunRequest{}},
		{RunID: "child", Request: taskapi.TaskRunRequest{ParentRunID: "parent"}},
		{RunID: "other", Request: taskapi.TaskRunRequest{}},
	}
	got := childrenFirst(ms)
	if got[0].RunID != "child" || got[1].RunID != "parent" || got[2].RunID != "other" {
		t.Errorf("order = %s %s %s; want the child first, the rest as they were", got[0].RunID, got[1].RunID, got[2].RunID)
	}
}

// A background task whose run fails is failed, with its run's reason — it was
// reported completed (its parent told "completed").
func TestABackgroundTaskWhoseRunFailsIsFailed(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, failer{})
	id, err := svc.bgMgr.Register("parent-9", taskapi.TaskRunRequest{TaskName: "failer", MaxDurationS: 60})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "ended", func() bool {
		rec, err := svc.bgMgr.TaskStatus(id)
		return err == nil && rec.Status != taskapi.TaskRunning && rec.Status != taskapi.TaskPending
	})
	rec, _ := svc.bgMgr.TaskStatus(id)
	if rec.Status != taskapi.TaskFailed || !strings.Contains(rec.Error, "the disk is full") {
		t.Errorf("task = %s %q; want failed with its run's reason", rec.Status, rec.Error)
	}
}
