// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	taskcore "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
)

// EVERY CHILD IS A RUN — a workflow run through the task service starts its
// children as runs of the service: they take their group's slots, and a
// step's children are its own.

// serviceForWorkflows is a task service that runs workflows and simulators.
func serviceForWorkflows(t *testing.T) taskapi.TaskService {
	t.Helper()
	svc, _ := serviceAndBus(t)
	return svc
}

// serviceAndBus is serviceForWorkflows with the event bus it publishes on.
func serviceAndBus(t *testing.T) (taskapi.TaskService, engine.EventBus) {
	t.Helper()
	registerSimulator(t)
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "workflow"}, Impl: &workflowFactory{}})
	app := application.New(application.Config())
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	bus := engine.NewMemoryEventBus()
	application.PutService[engine.EventBus](app, "default", bus)
	ctx := application.WithApplication(context.Background(), app)
	ts, err := taskcore.NewTaskService(ctx, &taskcore.TaskServiceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := ts.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
	return ts, bus
}

func sim(name string, ms int, output map[string]any) map[string]any {
	return map[string]any{"name": name, "task": map[string]any{"type": "simulator", "duration_ms": ms, "output": output}}
}

func runWorkflowThrough(t *testing.T, svc taskapi.TaskService, group string, input map[string]any, steps ...any) taskapi.ResultResponse {
	t.Helper()
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{
		TaskDef: map[string]any{"name": "wf", "type": "workflow", "config": map[string]any{"steps": steps}},
		Group:   group, Input: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil {
		t.Fatalf("the workflow did not end: %v", err)
	}
	return res
}

// A parallel's children take their group's slots: with a group of one, they
// run one at a time (the workflow, waiting, holds none) — and all complete.
func TestAParallelsChildrenTakeTheirGroupsSlots(t *testing.T) {
	svc := serviceForWorkflows(t)
	one := 1
	if _, err := svc.SetGroup(context.Background(), taskapi.SetGroupRequest{Group: taskapi.DefaultGroup, MaxRunning: &one}); err != nil {
		t.Fatal(err)
	}
	var most atomic.Int64
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
			}
			gs, _ := svc.Groups(context.Background(), taskapi.GroupsRequest{})
			for _, g := range gs.Groups {
				if g.Name == taskapi.DefaultGroup && int64(g.Running) > most.Load() {
					most.Store(int64(g.Running))
				}
			}
		}
	}()
	began := time.Now()
	res := runWorkflowThrough(t, svc, "", nil, map[string]any{"name": "fan", "kind": "parallel", "branches": []any{
		sim("a", 150, map[string]any{"v": "a"}), sim("b", 150, map[string]any{"v": "b"}), sim("c", 150, map[string]any{"v": "c"}),
	}}, map[string]any{"kind": "return", "value": "${results}"})
	close(stop)
	if res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("workflow %+v; want completed", res)
	}
	if n := most.Load(); n > 1 {
		t.Errorf("%d ran at once in a group of one", n)
	}
	if d := time.Since(began); d < 400*time.Millisecond {
		t.Errorf("three children of 150 ms ended in %s in a group of one: they did not take its slot", d)
	}
	out, _ := res.Output.(map[string]any)
	for _, b := range []string{"a", "b", "c"} {
		if _, ok := out[b]; !ok {
			t.Errorf("results %v; want %s", out, b)
		}
	}
}

// Two workflows at once, each with a child named "a", each get their own
// child's result — a child is its run, never a name the other shares.
func TestTwoWorkflowsKeepTheirOwnChildren(t *testing.T) {
	svc := serviceForWorkflows(t)
	ret := map[string]any{"kind": "return", "value": "${results.a.who}"}
	got := make(chan string, 2)
	for i, who := range []string{"first", "second"} {
		step := sim("a", 30+40*i, map[string]any{"who": who})
		go func(who string) {
			got <- who + "=" + endedOutput(svc, step, ret) // never t.Fatal off the test's goroutine: it would wait for ever
		}(who)
	}
	for i := 0; i < 2; i++ {
		if g := <-got; g != "first=first" && g != "second=second" {
			t.Errorf("a run got %s; want its own child's result", g)
		}
	}
}

// A race cancels its own losers — never a child another step started (a
// background delegate goes on).
func TestARaceCancelsOnlyItsOwnBranches(t *testing.T) {
	svc := serviceForWorkflows(t)
	bg := map[string]any{"name": "bg", "mode": "background", "task": map[string]any{"type": "simulator", "duration_ms": 400, "output": map[string]any{"v": "bg"}}}
	race := map[string]any{"name": "r", "kind": "race", "branches": []any{
		sim("fast", 20, map[string]any{"v": "fast"}), sim("slow", 3000, map[string]any{"v": "slow"}),
	}}
	join := map[string]any{"kind": "wait", "for_event": "child.bg.done", "timeout_ms": 5000}
	res := runWorkflowThrough(t, svc, "", nil, bg, race, join, map[string]any{"kind": "return", "value": "${results}"})
	if res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("workflow %+v; want completed — the background child was cancelled by the race?", res)
	}
	runs, _ := svc.Runs(context.Background(), taskapi.ListRunsRequest{})
	for _, r := range runs.Runs {
		if r.ParentRunID != res.RunID {
			continue // another test's
		}
		if r.TaskName == "bg" && r.Status != taskapi.RunStatusCompleted {
			t.Errorf("the background child %s; want completed", r.Status)
		}
		if r.TaskName == "slow" && r.Status == taskapi.RunStatusCompleted {
			t.Error("the race's loser was not cancelled")
		}
	}
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

// A workflow's child reports its start with its delegation mode and the
// mode's dimensions (the journal, the delegation graph read them).
func TestAChildsStartCarriesItsModesDimensions(t *testing.T) {
	svc, bus := serviceAndBus(t)
	got := make(chan *engine.TaskStartedEvent, 8)
	unsub := bus.Subscribe(engine.EventTaskStarted, func(_ string, p any) {
		// The start its parent's journal keeps (stamped with the parent's run).
		if e, ok := p.(*engine.TaskStartedEvent); ok && e.TaskName == "child" && e.Emitter != "" && e.Emitter == e.Correlation.ParentRunID {
			got <- e
		}
	})
	defer unsub()
	step := map[string]any{"name": "child", "mode": "background", "task": map[string]any{"type": "simulator", "duration_ms": 10, "output": map[string]any{"v": 1}}}
	join := map[string]any{"kind": "wait", "for_event": "child.child.done", "timeout_ms": 5000}
	runWorkflowThrough(t, svc, "", nil, step, join, map[string]any{"kind": "return", "value": "ok"})
	select {
	case e := <-got:
		if e.Mode != "background" || len(e.Dimensions) == 0 {
			t.Fatalf("child start %+v; want its mode and its dimensions", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no start event for the child")
	}
}

// endedOutput runs a workflow of steps and says its output — or why it did not
// end; safe off the test's goroutine.
func endedOutput(svc taskapi.TaskService, steps ...any) string {
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskDef: map[string]any{"name": "wf", "type": "workflow", "config": map[string]any{"steps": steps}}})
	if err != nil {
		return "not started: " + err.Error()
	}
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil {
		return "did not end: " + err.Error()
	}
	return toString(res.Output)
}

// A step mode no child can start with is refused when the workflow is
// loaded — an unknown one (it used to panic at run time), and the forks (a
// workflow has no conversation to fork).
func TestAStepModeNoChildCanStartWithIsRefusedAtLoad(t *testing.T) {
	for mode, want := range map[string]string{"no-such-mode": "unknown mode", "fork": "a workflow has none", "fork-isolated": "a workflow has none"} {
		step := sim("a", 10, nil)
		step["mode"] = mode
		_, err := LoadNodes(map[string]any{"steps": []any{step}})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("mode %s: %v; want refused (%s)", mode, err, want)
		}
	}
	step := sim("a", 10, nil)
	step["mode"] = "background"
	if _, err := LoadNodes(map[string]any{"steps": []any{step}}); err != nil {
		t.Errorf("mode background refused: %v", err)
	}
}

// A race retried by its on_error starts its branches again — new runs, not the
// ended ones of its last attempt found again. A workflow that fails is failed.
func TestARetriedStepStartsItsChildrenAgain(t *testing.T) {
	for _, kind := range []string{"race"} {
		t.Run(kind, func(t *testing.T) {
			svc := serviceForWorkflows(t)
			failing := map[string]any{"name": "f", "task": map[string]any{"type": "simulator", "duration_ms": 5, "fail_probability": 1.0, "fail_message": "no"}}
			step := map[string]any{"name": "s", "kind": kind, "branches": []any{failing}, "on_error": map[string]any{"strategy": "retry", "max_retries": 2}}
			res := runWorkflowThrough(t, svc, "", nil, step)
			if res.Status != string(taskapi.RunStatusFailed) {
				t.Errorf("workflow %+v; want failed — its only branch always fails", res)
			}
			runs, _ := svc.Runs(context.Background(), taskapi.ListRunsRequest{})
			n := 0
			for _, r := range runs.Runs {
				if r.ParentRunID == res.RunID && r.TaskName == "f" {
					n++
				}
			}
			if n != 3 {
				t.Errorf("the branch ran %d times; want 3 (the first attempt and 2 retries)", n)
			}
		})
	}
}
