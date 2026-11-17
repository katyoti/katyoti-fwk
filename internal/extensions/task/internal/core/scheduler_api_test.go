// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// THE SCHEDULER API — what a pane reads (Runs, Groups) and what it does
// (SetGroup, ClearGroup, Stop).

// stubbornMachine's one action ignores its context until released: a run that
// does not end when it is cancelled.
type stubbornMachine struct{ release chan struct{} }

func (stubbornMachine) Name() string                           { return "stubborn" }
func (stubbornMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }

// stubbornActions counts the stubborn actions started: a run's slot is
// granted a moment before its action runs.
var stubbornActions atomic.Int64

func (m stubbornMachine) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		stubbornActions.Add(1)
		<-m.release
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

// stubbornRelease is the current test's release: the machine type registry
// keeps the first factory contributed, so the factory reads it at create time.
var stubbornRelease chan struct{}

type stubbornFactory struct{}

func (stubbornFactory) Name() string { return "stubborn" }
func (stubbornFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return stubbornMachine{release: stubbornRelease}, nil
}

func runsByID(rs []taskapi.RunInfo) map[string]taskapi.RunInfo {
	out := map[string]taskapi.RunInfo{}
	for _, r := range rs {
		out[r.RunID] = r
	}
	return out
}

func groupLoad(t *testing.T, svc *Service, name string) taskapi.GroupLoad {
	t.Helper()
	gs, err := svc.Groups(context.Background(), taskapi.GroupsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range gs.Groups {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no group %q in %+v", name, gs.Groups)
	return taskapi.GroupLoad{}
}

func wake(t *testing.T, svc *Service, ids ...string) {
	t.Helper()
	for _, id := range ids {
		_ = svc.channel.Bus().Publish("wake."+id, map[string]any{})
		_, _ = svc.Result(context.Background(), taskapi.ResultRequest{RunID: id})
	}
}

// Runs says who started a run and for what, filters by group and by "active",
// and lists the newest first; an ended run carries its end time.
func TestRunsFilterAndSayWhoStartedThem(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(8)}})
	ctx := context.Background()
	a, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g",
		Origin: &taskapi.RunOrigin{Kind: taskapi.OriginPlanner, ID: "req-1"}, Target: "mod.obj"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a waiting", func() bool { return status(t, svc, a.RunID) == taskapi.RunStatusWaiting })
	time.Sleep(2 * time.Millisecond)
	b, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "uc-130-task", ParentRunID: a.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Result(ctx, taskapi.ResultRequest{RunID: b.RunID}); err != nil {
		t.Fatal(err)
	}

	all, _ := svc.Runs(ctx, taskapi.ListRunsRequest{})
	if len(all.Runs) < 2 || all.Runs[0].RunID != b.RunID {
		t.Fatalf("runs not newest first: %+v", all.Runs)
	}
	byID := runsByID(all.Runs)
	if o := byID[a.RunID]; o.Origin.Kind != taskapi.OriginPlanner || o.Origin.ID != "req-1" || o.Target != "mod.obj" {
		t.Errorf("a: origin %+v target %q", o.Origin, o.Target)
	}
	if o := byID[b.RunID]; o.Origin.Kind != taskapi.OriginAgent || o.Origin.ID != a.RunID {
		t.Errorf("b (delegated by a): origin %+v", o.Origin)
	}
	if byID[b.RunID].EndedAt == nil || byID[a.RunID].EndedAt != nil {
		t.Errorf("EndedAt: b %v (want set), a %v (want nil)", byID[b.RunID].EndedAt, byID[a.RunID].EndedAt)
	}

	active, _ := svc.Runs(ctx, taskapi.ListRunsRequest{Status: "active"})
	if ids := runsByID(active.Runs); len(ids) != 1 || ids[a.RunID].RunID == "" {
		t.Errorf("active = %+v, want only a", active.Runs)
	}
	inG, _ := svc.Runs(ctx, taskapi.ListRunsRequest{Group: "g"})
	if ids := runsByID(inG.Runs); ids[a.RunID].RunID == "" || ids[b.RunID].RunID != "" && ids[b.RunID].Group != "g" {
		t.Errorf("group g = %+v, want a in it", inG.Runs)
	}
	wake(t, svc, a.RunID)
}

// Groups counts the waiting runs apart from the running ones; a paused group
// starts nothing until it is resumed; SetGroup changes a limit live.
func TestGroupsLoadAndALiveLimit(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	a, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	eventually(t, "a waiting", func() bool { return status(t, svc, a.RunID) == taskapi.RunStatusWaiting })
	if l := groupLoad(t, svc, "g"); l.Waiting != 1 || l.Running != 0 || l.MaxRunning != 1 || !l.On {
		t.Fatalf("load with a waiting: %+v", l)
	}
	groupLoad(t, svc, taskapi.DefaultGroup) // the default group is listed

	off, on := false, true
	if _, err := svc.SetGroup(ctx, taskapi.SetGroupRequest{Group: "g", On: &off}); err != nil {
		t.Fatal(err)
	}
	b, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	eventually(t, "b queued in the paused group", func() bool { return status(t, svc, b.RunID) == taskapi.RunStatusQueued })
	time.Sleep(20 * time.Millisecond)
	if st := status(t, svc, b.RunID); st != taskapi.RunStatusQueued {
		t.Fatalf("b in a paused group = %s, want queued", st)
	}
	if _, err := svc.SetGroup(ctx, taskapi.SetGroupRequest{Group: "g", On: &on}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "b started once resumed", func() bool { return status(t, svc, b.RunID) == taskapi.RunStatusWaiting })

	if _, err := svc.SetGroup(ctx, taskapi.SetGroupRequest{}); err == nil {
		t.Error("SetGroup with no group: want an error")
	}
	two := 2
	if l, _ := svc.SetGroup(ctx, taskapi.SetGroupRequest{Group: "g", MaxRunning: &two}); l.MaxRunning != 2 {
		t.Errorf("max after set = %d, want 2", l.MaxRunning)
	}
	wake(t, svc, a.RunID, b.RunID)
}

// A raised limit starts a queued run at once.
func TestRaisingALimitStartsAQueuedRun(t *testing.T) {
	old := KillAfter
	KillAfter = time.Hour
	t.Cleanup(func() { KillAfter = old })
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx := context.Background()
	a, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	eventually(t, "a running", func() bool { return status(t, svc, a.RunID) == taskapi.RunStatusRunning })
	b, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	eventually(t, "b queued", func() bool { return status(t, svc, b.RunID) == taskapi.RunStatusQueued })
	two := 2
	if _, err := svc.SetGroup(ctx, taskapi.SetGroupRequest{Group: "g", MaxRunning: &two}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "b running once the limit is 2", func() bool { return status(t, svc, b.RunID) == taskapi.RunStatusRunning })
}

// ClearGroup stops the queued runs of one origin and leaves the others.
func TestClearGroupStopsOneOriginsQueuedRuns(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	off := false
	_, _ = svc.SetGroup(ctx, taskapi.SetGroupRequest{Group: "g", On: &off})
	run := func(origin string) string {
		r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g",
			Origin: &taskapi.RunOrigin{Kind: taskapi.OriginPlanner, ID: origin}})
		if err != nil {
			t.Fatal(err)
		}
		eventually(t, "queued", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusQueued })
		return r.RunID
	}
	x1, x2, y := run("x"), run("x"), run("y")
	out, err := svc.ClearGroup(ctx, taskapi.ClearGroupRequest{Group: "g", OriginID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Cleared) != 2 {
		t.Fatalf("cleared %v, want x's two", out.Cleared)
	}
	for _, id := range []string{x1, x2} {
		eventually(t, id+" cancelled (it leaves its queue at once)", func() bool { return status(t, svc, id) == taskapi.RunStatusCancelled })
	}
	if st := status(t, svc, y); st != taskapi.RunStatusQueued {
		t.Errorf("y = %s, want still queued", st)
	}
	eventually(t, "x's runs out of the queue", func() bool { return groupLoad(t, svc, "g").Queued == 1 })
}

func useStubborn(svc *Service, release chan struct{}) {
	stubbornRelease = release
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "stubborn"}, Impl: stubbornFactory{}})
	svc.tasks["stubborn"] = &taskapi.Task{Name: "stubborn", Provider: "test", Type: "stubborn"}
}

// Stop cancels; a run that does not end within KillAfter is killed: its slot
// freed, its status killed. Stopping an ended run stops nothing.
func TestStopCancelsThenKills(t *testing.T) {
	old := KillAfter
	KillAfter = 50 * time.Millisecond
	t.Cleanup(func() { KillAfter = old })
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx := context.Background()

	before := stubbornActions.Load()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	// Its action running (stopped before its step, a run ends at once).
	eventually(t, "running", func() bool { return groupLoad(t, svc, "g").Running == 1 && stubbornActions.Load() > before })
	out, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID})
	if err != nil || !out.Stopped {
		t.Fatalf("stop: %+v %v", out, err)
	}
	if st := status(t, svc, r.RunID); st != taskapi.RunStatusStopping {
		t.Errorf("right after stop = %s, want stopping (it has not ended)", st)
	}
	eventually(t, "killed", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusKilled })
	if l := groupLoad(t, svc, "g"); l.Running != 0 {
		t.Errorf("killed run still holds its slot: %+v", l)
	}

	done, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "uc-130-task"})
	_, _ = svc.Result(ctx, taskapi.ResultRequest{RunID: done.RunID})
	if out, _ := svc.Stop(ctx, taskapi.StopRequest{RunID: done.RunID}); out.Stopped {
		t.Error("stopping an ended run: want Stopped false")
	}
}

// A task's declared group places its runs when the caller names none; the
// caller's group wins over it; the catalog carries it across processes.
func TestATaskDeclaredGroupPlacesItsRuns(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"background": one(8), "mine": one(8)}})
	svc.tasks["grouped"] = &taskapi.Task{Name: "grouped", Provider: "test", Type: "waiter", Group: "background"}
	ctx := context.Background()
	a, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "grouped"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "grouped", Group: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	if g := svc.groups.groupOf(a.RunID); g != "background" {
		t.Errorf("no group asked: group = %q, want the task's (background)", g)
	}
	if g := svc.groups.groupOf(b.RunID); g != "mine" {
		t.Errorf("group asked: group = %q, want the caller's (mine)", g)
	}
	if info := taskInfoFromBody("p", mbapi.TaskBody{Name: "x", Group: "background"}); info.Group != "background" {
		t.Errorf("catalog TaskInfo.Group = %q, want background", info.Group)
	}
	for _, id := range []string{a.RunID, b.RunID} {
		eventually(t, "waiting", func() bool { return status(t, svc, id) == taskapi.RunStatusWaiting })
	}
	wake(t, svc, a.RunID, b.RunID)
}
