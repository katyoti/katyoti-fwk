// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

func keepAliveOf(t *testing.T, svc *Service, task string) taskapi.KeepAliveInfo {
	t.Helper()
	gs, err := svc.Groups(context.Background(), taskapi.GroupsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range gs.KeepAlive {
		if k.Task == task {
			return k
		}
	}
	t.Fatalf("no kept-alive %q in %+v", task, gs.KeepAlive)
	return taskapi.KeepAliveInfo{}
}

func runsOf(t *testing.T, svc *Service, task string) []taskapi.RunInfo {
	t.Helper()
	out, err := svc.Runs(context.Background(), taskapi.ListRunsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var mine []taskapi.RunInfo
	for _, r := range out.Runs {
		if r.TaskName == task {
			mine = append(mine, r)
		}
	}
	return mine
}

func fastKeepAlive(t *testing.T) {
	old := keepAliveFirstBackoff
	keepAliveFirstBackoff = 20 * time.Millisecond
	t.Cleanup(func() { keepAliveFirstBackoff = old })
}

// A kept-alive task that ends is started again after its backoff; ending too
// often stops it, with a service warning — not a spin.
func TestAKeptAliveTaskIsStartedAgainThenStoppedIfItKeepsEnding(t *testing.T) {
	fastKeepAlive(t)
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, failer{})
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "failer"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.keepAliveSweep(ctx)
	eventually(t, "stopped after ending too often", func() bool { return keepAliveOf(t, svc, "failer").State == "stopped" })
	if n := len(runsOf(t, svc, "failer")); n != keepAliveMaxEnds {
		t.Errorf("%d runs; want it started %d times, then stopped", n, keepAliveMaxEnds)
	}
	for _, r := range runsOf(t, svc, "failer") {
		if r.Origin.Kind != taskapi.OriginKeepAlive {
			t.Errorf("run %s origin %+v; want keep-alive", r.RunID, r.Origin)
		}
	}
	if k := keepAliveOf(t, svc, "failer"); !strings.Contains(k.Reason, "ended 5 times") {
		t.Errorf("reason %q", k.Reason)
	}
	eventually(t, "a service warning", func() bool {
		for _, w := range svc.serviceWarnings() {
			if strings.Contains(w, "failer") {
				return true
			}
		}
		return false
	})
}

// One run at a time; a waiting kept-alive run survives a restart as itself —
// no second run is started beside it.
func TestAKeptAliveTaskRunsOnceAndSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	first := durableService(t, dir)
	first.useKeepAlive([]taskapi.KeepAliveTask{{Task: "waiter", Group: "g"}})
	first.keepAliveSweep(ctx)
	first.keepAliveSweep(ctx)
	runs := runsOf(t, first, "waiter")
	if len(runs) != 1 {
		t.Fatalf("%d runs; want one", len(runs))
	}
	id := runs[0].RunID
	eventually(t, "waiting", func() bool { return status(t, first, id) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, id)

	second := durableService(t, dir)
	second.useKeepAlive([]taskapi.KeepAliveTask{{Task: "waiter", Group: "g"}})
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	swept := make(chan struct{})
	go second.ServeRestores(sctx, func() { close(swept) })
	<-swept
	runs = runsOf(t, second, "waiter")
	if len(runs) != 1 || runs[0].RunID != id {
		t.Fatalf("runs after the restart = %+v; want the one run, resumed", runs)
	}
	if k := keepAliveOf(t, second, "waiter"); k.State != "running" || k.RunID != id {
		t.Errorf("kept-alive = %+v; want it running as %s", k, id)
	}
	wake(t, second, id)
}

// A person stopping a kept-alive run stops the task — it is not started again
// — until a person runs it by hand.
func TestStoppingAKeptAliveRunStopsItUntilRunByHand(t *testing.T) {
	fastKeepAlive(t)
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "waiter"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.keepAliveSweep(ctx)
	r := runsOf(t, svc, "waiter")[0]
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusCancelled })
	time.Sleep(60 * time.Millisecond) // past a backoff
	svc.keepAliveSweep(ctx)
	if n := len(runsOf(t, svc, "waiter")); n != 1 {
		t.Fatalf("%d runs; a stopped kept-alive task was started again", n)
	}
	if k := keepAliveOf(t, svc, "waiter"); k.State != "stopped" || !strings.Contains(k.Reason, "by a person") {
		t.Errorf("kept-alive = %+v; want it stopped by a person", k)
	}

	h, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	svc.keepAliveSweep(ctx)
	if k := keepAliveOf(t, svc, "waiter"); k.State != "running" || k.RunID != h.RunID {
		t.Errorf("kept-alive = %+v; want it kept running again, the run by hand as its run", k)
	}
	if n := len(runsOf(t, svc, "waiter")); n != 2 {
		t.Errorf("%d runs; want the stopped one and the one by hand, no third", n)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, h.RunID) == taskapi.RunStatusWaiting })
	wake(t, svc, h.RunID)
}

// A kept-alive run the restart interrupted (a kind not restarted) is started
// again at once — no backoff: the process went down, not the task.
func TestAKeptAliveRunInterruptedByARestartStartsAgainAtOnce(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	first := durableService(t, dir)
	useStubborn(first, release)
	first.useKeepAlive([]taskapi.KeepAliveTask{{Task: "stubborn", Group: "g"}})
	before := stubbornActions.Load()
	first.keepAliveSweep(ctx)
	eventually(t, "working", func() bool { return stubbornActions.Load() > before })
	old := runsOf(t, first, "stubborn")[0].RunID
	orphan(t, first.runs, old)

	second := durableService(t, dir)
	useStubborn(second, release)
	second.useKeepAlive([]taskapi.KeepAliveTask{{Task: "stubborn", Group: "g"}})
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	swept := make(chan struct{})
	go second.ServeRestores(sctx, func() { close(swept) })
	<-swept
	var fresh string
	for _, r := range runsOf(t, second, "stubborn") {
		if r.RunID != old {
			fresh = r.RunID
		}
	}
	if fresh == "" {
		t.Fatalf("runs = %+v; want a new run started at once", runsOf(t, second, "stubborn"))
	}
	// Still an end: a task that keeps taking its process down is stopped too.
	second.keepAlive.mu.Lock()
	ends := len(second.keepAlive.state["stubborn"].ends)
	second.keepAlive.mu.Unlock()
	if ends != 1 {
		t.Errorf("%d ends counted; want the interruption counted", ends)
	}
}

// Any run of a kept-alive task is its run, whoever started it: a person
// stopping a run started by hand stops the task; one that ends is started again
// after a backoff.
func TestARunByHandOfAKeptAliveTaskIsItsRun(t *testing.T) {
	fastKeepAlive(t)
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "waiter"}})
	ctx := context.Background()

	h, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, h.RunID) == taskapi.RunStatusWaiting })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: h.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return status(t, svc, h.RunID) == taskapi.RunStatusCancelled })
	time.Sleep(60 * time.Millisecond) // past a backoff
	svc.keepAliveSweep(ctx)
	if n := len(runsOf(t, svc, "waiter")); n != 1 {
		t.Fatalf("%d runs; a kept-alive task whose run a person stopped was started again", n)
	}
	if k := keepAliveOf(t, svc, "waiter"); k.State != "stopped" || k.RunID != h.RunID {
		t.Errorf("kept-alive = %+v; want it stopped, its run %s", k, h.RunID)
	}

	// No sweep here: the only run of failer is the one by hand.
	other := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(other, failer{})
	other.useKeepAlive([]taskapi.KeepAliveTask{{Task: "failer"}})
	if _, err := other.Run(ctx, taskapi.TaskRunRequest{TaskName: "failer"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the run by hand counted as an end", func() bool { return keepAliveOf(t, other, "failer").Restarts == 1 })
}

// A kept-alive task's state survives a restart: one a person stopped stays
// stopped, and its count of ends is kept.
func TestAKeptAliveTasksStateSurvivesARestart(t *testing.T) {
	fastKeepAlive(t)
	keepAliveFirstBackoff = time.Hour // backing off across the restart
	ctx := context.Background()
	dir := scratchDir(t)
	declared := []taskapi.KeepAliveTask{{Task: "waiter", Group: "g"}, {Task: "failer", Group: "g"}}
	first := durableService(t, dir)
	useMachine(first, failer{})
	first.useKeepAlive(declared)
	first.keepAliveSweep(ctx)
	r := runsOf(t, first, "waiter")[0]
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	eventually(t, "failer backing off", func() bool { return keepAliveOf(t, first, "failer").State == "backing-off" })
	// The stop comes last: nothing saves the state after it but the stop itself.
	if _, err := first.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusCancelled })

	second := durableService(t, dir)
	useMachine(second, failer{})
	second.useKeepAlive(declared)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	swept := make(chan struct{})
	go second.ServeRestores(sctx, func() { close(swept) })
	<-swept
	for _, x := range runsOf(t, second, "waiter") {
		if x.RunID != r.RunID {
			t.Fatalf("run %s started after the restart; the task a person stopped must stay stopped", x.RunID)
		}
	}
	if k := keepAliveOf(t, second, "waiter"); k.State != "stopped" || k.RunID != r.RunID {
		t.Errorf("kept-alive = %+v; want it still stopped, its run %s", k, r.RunID)
	}
	if k := keepAliveOf(t, second, "failer"); k.State != "backing-off" || k.Restarts != 1 {
		t.Errorf("kept-alive = %+v; want its backoff and count kept", k)
	}
	if n := len(runsOf(t, second, "failer")); n != 1 {
		t.Errorf("%d failer runs; want the one before the restart, none started during its backoff", n)
	}
}
