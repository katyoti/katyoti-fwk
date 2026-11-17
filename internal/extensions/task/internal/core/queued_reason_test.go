// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A QUEUED RUN SAYS WHY IT WAITS — its group off, its group full, the
// universe's ceiling — and a run stopped while queued ends saying it, never
// only that it was stopped.

func TestAQueuedRunSaysWhyItWaits(t *testing.T) {
	off := false
	cases := []struct {
		name  string
		cfg   taskapi.TaskGroupsConfig
		fill  []string // groups a stubborn run holds a slot of first
		group string   // the queued run's group
		want  string
	}{
		{"group off", taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, On: &off}}}, nil, "g", `group "g" is off`},
		{"group full", taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}}, []string{"g"}, "g", `group "g" is full: 1 of 1 running`},
		{"universe ceiling", taskapi.TaskGroupsConfig{MaxRunning: 1, Groups: map[string]taskapi.TaskGroup{"g": one(2), "h": one(2)}}, []string{"h"}, "g", "the universe's ceiling is reached: 1 runs of all groups running, at most 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := groupsService(t, c.cfg)
			log := lifecycleOf(svc)
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			useStubborn(svc, release)
			for _, g := range c.fill {
				holder(t, svc, g)
			}
			ctx := context.Background()
			q, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: c.group})
			if err != nil {
				t.Fatal(err)
			}
			eventually(t, "queued", func() bool { return log.has(q.RunID, engine.RunQueued) })

			if e, _ := log.last(q.RunID, engine.RunQueued); !strings.Contains(e.Reason, c.want) {
				t.Errorf("queued event reason %q; want %q", e.Reason, c.want)
			}
			if why := queuedBecause(t, svc, q.RunID); !strings.Contains(why, c.want) {
				t.Errorf("run's queuedBecause %q; want %q", why, c.want)
			}

			if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: q.RunID}); err != nil {
				t.Fatal(err)
			}
			res, err := svc.Result(ctx, taskapi.ResultRequest{RunID: q.RunID})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != string(taskapi.RunStatusCancelled) || !strings.Contains(res.Error, "queued") || !strings.Contains(res.Error, c.want) {
				t.Errorf("result %+v; want cancelled, saying it was stopped while queued and %q", res, c.want)
			}
			if e, _ := log.last(q.RunID, engine.RunCancelled); !strings.Contains(e.Reason, c.want) {
				t.Errorf("cancelled event reason %q; want %q", e.Reason, c.want)
			}
		})
	}
}

// A run that got its slot no longer carries a reason to wait.
func TestARunThatStartsDropsItsQueuedReason(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	log := lifecycleOf(svc)
	release := make(chan struct{})
	useStubborn(svc, release)
	holder(t, svc, "g")
	q, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "queued", func() bool { return log.has(q.RunID, engine.RunQueued) })
	close(release) // the holder ends: its slot goes to the queued run
	eventually(t, "started", func() bool { return log.has(q.RunID, engine.RunStarted) })
	if why := queuedBecause(t, svc, q.RunID); why != "" {
		t.Errorf("a started run's queuedBecause %q; want none", why)
	}
	_, _ = svc.Stop(context.Background(), taskapi.StopRequest{RunID: q.RunID})
}

func queuedBecause(t *testing.T, svc *Service, runID string) string {
	t.Helper()
	runs, err := svc.Runs(context.Background(), taskapi.ListRunsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs.Runs {
		if r.RunID == runID {
			return r.QueuedBecause
		}
	}
	t.Fatalf("run %s not listed", runID)
	return ""
}

// A bounded Result (waitS) on a run that cannot start answers where it is and
// why, instead of waiting for an end that does not come.
func TestABoundedResultSaysWhyARunStillQueues(t *testing.T) {
	off := false
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, On: &off}}})
	ctx := context.Background()
	q, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: q.RunID}) })
	res, err := svc.Result(ctx, taskapi.ResultRequest{RunID: q.RunID, WaitS: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(taskapi.RunStatusQueued) || !strings.Contains(res.QueuedBecause, `group "g" is off`) {
		t.Errorf("result %+v; want queued, saying its group is off", res)
	}
}
