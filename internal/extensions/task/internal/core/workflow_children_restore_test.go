// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A WORKFLOW'S CHILDREN COME BACK WITH IT — what a workflow step does with a
// child, without the workflow package (it imports this one): start it at a
// site through the service's spawner, wait on its run's end, settle from how
// it ended.

// wfParent starts one child (childKind) at site "step@1" and waits for it.
type wfParent struct {
	childKind string
	sp        engine.Spawner
	gate      <-chan struct{} // when set, its settling step waits for it (a crash lands there)
}

func (wfParent) Name() string                           { return "wfparent" }
func (wfParent) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (wfParent) RestartsAfterCrash() bool               { return true }
func (m wfParent) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	name, runID := st.Name, st.RunID
	woken := ev.Type != engine.TickEvent && ev.Type != engine.RestartedEvent // its child's end
	data, _ := st.Data.(map[string]any)
	return func(ctx context.Context) (*engine.StepResult, error) {
		if name == "init" {
			h, err := m.sp.Spawn(ctx, engine.ChildSpec{Kind: m.childKind, Name: "c", ParentID: runID, Site: "step@1"})
			if err != nil {
				return nil, err
			}
			return &engine.StepResult{NextState: "awaiting", Data: map[string]any{"child": h.RunID},
				Wait: &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: engine.RunEndedTopic(h.RunID)}}, RecheckOnResume: true}}, nil
		}
		child, _ := data["child"].(string)
		if m.gate != nil && woken {
			select {
			case <-m.gate:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		o, ended := m.sp.Outcome(child)
		if !ended {
			return &engine.StepResult{NextState: "awaiting", Data: data,
				Wait: &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: engine.RunEndedTopic(child)}}, RecheckOnResume: true}}, nil
		}
		return &engine.StepResult{NextState: "done", Data: data, Output: map[string]any{"child": string(o.Status), "out": o.Output}}, nil
	}, nil
}

type wfParentFactory struct {
	childKind string
	gate      <-chan struct{}
}

func (wfParentFactory) Name() string { return "wfparent" }
func (f wfParentFactory) CreateMachine(ctx context.Context, _ any) (engine.Machine, error) {
	return wfParent{childKind: f.childKind, sp: engine.SpawnerFrom(ctx), gate: f.gate}, nil
}

func useParent(svc *Service, childKind string) {
	useGatedParent(svc, "wfparent-"+childKind, childKind, nil)
}

func useGatedParent(svc *Service, name, childKind string, gate <-chan struct{}) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: name}, Impl: wfParentFactory{childKind: childKind, gate: gate}})
	svc.tasks[name] = &taskapi.Task{Name: name, Provider: "test", Type: name}
}

// A child that ended just before the crash — its parent woken, not yet past
// its end — is not run again: the parent, restored, settles it from its kept
// result.
func TestARestoredWorkflowGetsItsFinishedChildsResult(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	gate := make(chan struct{}) // opened only for the second process (one factory per kind)
	first := durableService(t, dir)
	useGatedParent(first, "wfparent-gated", "waiter", gate)
	p, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "wfparent-gated", Group: "g2"})
	if err != nil {
		t.Fatal(err)
	}
	child := engine.ChildRunID(p.RunID, "step@1")
	eventually(t, "both waiting", func() bool {
		return status(t, first, p.RunID) == taskapi.RunStatusWaiting && status(t, first, child) == taskapi.RunStatusWaiting
	})
	wake(t, first, child) // the child ends; its parent wakes, held at its gate
	eventually(t, "the parent woken", func() bool { return status(t, first, p.RunID) == taskapi.RunStatusRunning })
	e, _ := first.reg.get(p.RunID)
	e.pause() // its process goes down there
	<-e.done
	orphan(t, first.runs, p.RunID)

	second := durableService(t, dir)
	useGatedParent(second, "wfparent-gated", "waiter", gate)
	close(gate)
	rep := second.RestoreRuns(ctx)
	for _, id := range append(rep.Restarted, rep.Resumed...) {
		if id == child {
			t.Fatalf("restore %+v: the child that had ended was taken up again", rep)
		}
	}
	eventually(t, "the parent completed", func() bool { return status(t, second, p.RunID) == taskapi.RunStatusCompleted })
	res, err := second.Result(ctx, taskapi.ResultRequest{RunID: p.RunID})
	if err != nil || res.Output.(map[string]any)["child"] != string(engine.ChildDone) {
		t.Fatalf("parent result %+v (%v); want its child done", res, err)
	}
}

// A workflow's child waiting at the crash comes back with its parent — not
// interrupted — and its end reaches the restored parent.
func TestAWorkflowsChildIsRestoredWithIt(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useParent(first, "waiter")
	p, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "wfparent-waiter", Group: "g2"})
	if err != nil {
		t.Fatal(err)
	}
	child := engine.ChildRunID(p.RunID, "step@1")
	eventually(t, "both waiting", func() bool {
		return status(t, first, p.RunID) == taskapi.RunStatusWaiting && status(t, first, child) == taskapi.RunStatusWaiting
	})
	orphan(t, first.runs, p.RunID)
	orphan(t, first.runs, child)

	second := durableService(t, dir)
	useParent(second, "waiter")
	rep := second.RestoreRuns(ctx)
	resumed := map[string]bool{}
	for _, id := range rep.Resumed {
		resumed[id] = true
	}
	if !resumed[p.RunID] || !resumed[child] || len(rep.Interrupted) > 0 {
		t.Fatalf("restore %+v; want the parent and its child resumed, none interrupted", rep)
	}
	wake(t, second, child)
	eventually(t, "the parent completed", func() bool { return status(t, second, p.RunID) == taskapi.RunStatusCompleted })
	wake(t, first, child, p.RunID)
}

// A workflow's child whose parent is gone (ended, or never restored) is
// reported interrupted.
func TestAWorkflowsChildWithoutItsParentIsInterrupted(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	child := engine.ChildRunID("run-gone0000000000", "step@1")
	if _, _, err := first.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g2", ParentRunID: "run-gone0000000000"}, child,
		&spawnSite{Name: "c", Site: "step@1", By: "workflow"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, child) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, child)

	second := durableService(t, dir)
	rep := second.RestoreRuns(ctx)
	if len(rep.Interrupted) != 1 || rep.Interrupted[0] != child || len(rep.Resumed) > 0 {
		t.Fatalf("restore %+v; want the orphan child interrupted", rep)
	}
	wake(t, first, child)
}
