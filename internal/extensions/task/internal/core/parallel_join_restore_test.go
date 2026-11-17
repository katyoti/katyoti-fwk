// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	core "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/wkflow"
)

// A WAIT FOR SEVERAL EVENTS SURVIVES A RESTART — the workflow's parallel
// join waits for the end of each of its children; the children it already
// settled are kept in its checkpoint, so a join restored after some of them
// ended resumes on the last one, never waiting again for those, never
// counting them twice.

func joinTask() *taskapi.Task {
	branch := func(name string) any {
		return map[string]any{"name": name, "task": map[string]any{"type": "waiter"}}
	}
	return &taskapi.Task{Name: "join", Provider: "test", Type: "workflow", Config: map[string]any{"steps": []any{
		map[string]any{"name": "fan", "kind": "parallel", "branches": []any{branch("a"), branch("b"), branch("c")}},
		map[string]any{"kind": "return", "value": "${results}"},
	}}}
}

func joinService(t *testing.T, dir string) *core.Service {
	t.Helper()
	_ = engine.MachineDomain.Types.Contribute(wkflow.MachineStrategyDef.Strategy.CreateTypeDef("workflow", workflowFactory(t)))
	svc := core.DurableServiceForTest(t, dir)
	svc.AddTaskForTest(joinTask())
	return svc
}

func workflowFactory(t *testing.T) engine.MachineFactory {
	t.Helper()
	_, create := wkflow.MachineStrategyDef.Strategy.NewTypeConfig()
	f, err := create(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// awaited is the children a run waits for now: name → run id.
func awaited(svc *core.Service, runID string) map[string]string {
	out := map[string]string{}
	for _, c := range svc.AwaitedForTest(runID) {
		if name, ok := strings.CutPrefix(c.Label, "ended:"); ok {
			out[name] = strings.TrimSuffix(strings.TrimPrefix(c.Topic, "run."), ".ended")
		}
	}
	return out
}

func names(m map[string]string) string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func runStatus(t *testing.T, svc *core.Service, id string) taskapi.RunStatus {
	t.Helper()
	st, err := svc.Status(context.Background(), taskapi.StatusRequest{RunID: id})
	if err != nil {
		t.Fatal(err)
	}
	return st.Status
}

func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("20s and still not: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A parallel join of three children, two ended before the restart, is
// restored waiting for the third only; it ends on the third's end with the
// three results.
func TestARestoredParallelJoinResumesOnItsLastChild(t *testing.T) {
	dir, err := os.MkdirTemp("", "ktw-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ctx := context.Background()

	first := joinService(t, dir)
	p, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "join", Group: "g2"})
	if err != nil {
		t.Fatal(err)
	}
	var children map[string]string
	until(t, "the join waiting for its three children", func() bool {
		children = awaited(first, p.RunID)
		if len(children) != 3 {
			return false
		}
		for _, id := range children {
			if runStatus(t, first, id) != taskapi.RunStatusWaiting {
				return false
			}
		}
		return true
	})
	first.WakeForTest(children["a"])
	first.WakeForTest(children["b"])
	until(t, "the join waiting for c only", func() bool {
		return names(awaited(first, p.RunID)) == "c" && runStatus(t, first, p.RunID) == taskapi.RunStatusWaiting
	})
	first.OrphanForTest(t, p.RunID) // its process goes down there
	first.OrphanForTest(t, children["c"])

	second := joinService(t, dir)
	rep := second.RestoreRuns(ctx)
	taken := map[string]bool{}
	for _, id := range append(rep.Restarted, rep.Resumed...) {
		taken[id] = true
	}
	if taken[children["a"]] || taken[children["b"]] || !taken[p.RunID] || !taken[children["c"]] || len(rep.Interrupted) > 0 {
		t.Fatalf("restore %+v; want the join and c taken up, a and b (ended) not, none interrupted", rep)
	}
	until(t, "the restored join waiting", func() bool {
		s := runStatus(t, second, p.RunID)
		return s == taskapi.RunStatusWaiting || s == taskapi.RunStatusCompleted || s == taskapi.RunStatusFailed
	})
	if s := runStatus(t, second, p.RunID); s != taskapi.RunStatusWaiting {
		t.Fatalf("the restored join is %s before its last child ended; want it waiting for c", s)
	}
	if got := names(awaited(second, p.RunID)); got != "c" {
		t.Fatalf("the restored join waits for %q; want c only, never the children that ended", got)
	}

	second.WakeForTest(children["c"])
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	res, err := second.Result(rctx, taskapi.ResultRequest{RunID: p.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("the join after c ended: %+v (%v); want completed", res, err)
	}
	out, _ := res.Output.(map[string]any)
	for _, n := range []string{"a", "b", "c"} {
		if r, _ := out[n].(map[string]any); r == nil || r["woke"] != true {
			t.Errorf("results %v; want %s's output", out, n)
		}
	}
	for _, id := range children {
		first.WakeForTest(id) // nothing left waiting in the first process
	}
}
