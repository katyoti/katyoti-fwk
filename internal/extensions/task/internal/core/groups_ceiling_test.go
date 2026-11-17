// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// STATUSES TELL THE TRUTH, THE UNIVERSE HAS A CEILING, NO RUN STARVES.

// orderRec records the order in which its runs got a slot (their first step),
// then waits for ever (giving its slot back).
type orderRec struct{}

var started struct {
	mu  sync.Mutex
	ids []string
}

func (orderRec) Name() string                           { return "orderrec" }
func (orderRec) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (orderRec) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		if st.Name == "init" {
			started.mu.Lock()
			started.ids = append(started.ids, st.RunID)
			started.mu.Unlock()
		}
		return &engine.StepResult{NextState: "waiting", Wait: &engine.WaitSpec{Mode: engine.WaitAny,
			Conditions: []engine.WaitCondition{{Topic: "never." + st.RunID}}}}, nil
	}, nil
}

type orderRecFactory struct{}

func (orderRecFactory) Name() string { return "orderrec" }
func (orderRecFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return orderRec{}, nil
}

func useOrderRec(svc *Service) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "orderrec"}, Impl: orderRecFactory{}})
	svc.tasks["orderrec"] = &taskapi.Task{Name: "orderrec", Provider: "test", Type: "orderrec"}
	started.mu.Lock()
	started.ids = nil
	started.mu.Unlock()
}

func startedOrder() []string {
	started.mu.Lock()
	defer started.mu.Unlock()
	return append([]string(nil), started.ids...)
}

// holder runs a stubborn run in group: it holds a slot until release closes.
func holder(t *testing.T, svc *Service, group string) {
	t.Helper()
	if _, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "stubborn", Group: group}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the slot held", func() bool { return groupLoad(t, svc, group).Running == 1 })
}

// A new run is "queued" until it has a slot; a stopped run is "stopping"
// until it has really ended, then "cancelled".
func TestStatusesSayWhereARunIs(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	useStubborn(svc, release)
	holder(t, svc, "g")
	ctx := context.Background()
	q, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Status != taskapi.RunStatusQueued {
		t.Errorf("a run with no slot yet: %s, want queued", q.Status)
	}
	runs, _ := svc.Runs(ctx, taskapi.ListRunsRequest{Group: "g"})
	var holderID string
	for _, r := range runs.Runs {
		if r.TaskName == "stubborn" {
			holderID = r.RunID
		}
	}
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: holderID})
	if st := status(t, svc, holderID); st != taskapi.RunStatusStopping {
		t.Errorf("stopped, still ending: %s, want stopping", st)
	}
	once.Do(func() { close(release) })
	eventually(t, "ended: cancelled", func() bool { return status(t, svc, holderID) == taskapi.RunStatusCancelled })
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: q.RunID})
}

// The universe's ceiling holds across groups: a group with free slots still
// waits when the universe runs its maximum.
func TestTheUniverseCeilingHoldsAcrossGroups(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{MaxRunning: 1, Groups: map[string]taskapi.TaskGroup{"a": one(4), "b": one(4)}})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	useStubborn(svc, release)
	useOrderRec(svc)
	holder(t, svc, "a")
	ctx := context.Background()
	b, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "orderrec", Group: "b"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if l := groupLoad(t, svc, "b"); l.Running != 0 || l.Queued != 1 {
		t.Fatalf("b at the ceiling = %+v; want it queued", l)
	}
	once.Do(func() { close(release) })
	eventually(t, "b ran once a slot of the universe freed", func() bool { return len(startedOrder()) == 1 })
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: b.RunID})
}

// At the ceiling, a person's runs (the default group) start before the
// others, whatever the groups' priorities.
func TestAtTheCeilingAPersonsRunsGoFirst(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{MaxRunning: 1, Groups: map[string]taskapi.TaskGroup{
		"x":                  {MaxRunning: 4, Priority: 50},
		taskapi.DefaultGroup: {MaxRunning: 4},
	}})
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	useStubborn(svc, release)
	useOrderRec(svc)
	holder(t, svc, "x")
	ctx := context.Background()
	x2, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "orderrec", Group: "x"})
	time.Sleep(10 * time.Millisecond)
	u1, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "orderrec"})
	eventually(t, "both queued", func() bool {
		return groupLoad(t, svc, "x").Queued == 1 && groupLoad(t, svc, taskapi.DefaultGroup).Queued == 1
	})
	once.Do(func() { close(release) })
	eventually(t, "both ran", func() bool { return len(startedOrder()) == 2 })
	if o := startedOrder(); o[0] != u1.RunID {
		t.Errorf("order = %v; want the person's run %s first", o, u1.RunID)
	}
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: x2.RunID})
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: u1.RunID})
}

// A run naming a group TaskGroups.yaml does not declare runs in the default
// group — a typo or an unknown name does not create a new group of slots.
func TestAnUndeclaredGroupRunsInTheDefaultOne(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"a": one(2)}})
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "no-such-group"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	runs, _ := svc.Runs(ctx, taskapi.ListRunsRequest{})
	if got := runsByID(runs.Runs)[r.RunID].Group; got != taskapi.DefaultGroup {
		t.Errorf("run's group = %q, want the default group", got)
	}
	gs, _ := svc.Groups(ctx, taskapi.GroupsRequest{})
	for _, g := range gs.Groups {
		if g.Name == "no-such-group" {
			t.Error("an undeclared group was created")
		}
	}
	wake(t, svc, r.RunID)
}

// A queued run's priority rises while it waits: an old low-priority run gets
// through before a fresh higher-priority one.
func TestPriorityAgingLetsAnOldRunThrough(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	svc.groups.aging = 10 * time.Millisecond
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	useStubborn(svc, release)
	useOrderRec(svc)
	holder(t, svc, "g")
	ctx := context.Background()
	low, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "orderrec", Group: "g", Priority: 0})
	time.Sleep(80 * time.Millisecond)
	high, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "orderrec", Group: "g", Priority: 3})
	eventually(t, "both queued", func() bool { return groupLoad(t, svc, "g").Queued == 2 })
	once.Do(func() { close(release) })
	eventually(t, "both ran", func() bool { return len(startedOrder()) == 2 })
	if o := startedOrder(); o[0] != low.RunID {
		t.Errorf("order = %v; want the old run %s first", o, low.RunID)
	}
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: low.RunID})
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: high.RunID})
}
