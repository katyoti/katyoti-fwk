// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN COMES BACK EXACTLY AS IT WAS — never twice, never from scratch, never
// after a stop; a panic fails one run, never the process.

// eoDone: restartable; one step that completes (counted).
var eoDoneSteps atomic.Int64

type eoDone struct{}

func (eoDone) Name() string                           { return "eodone" }
func (eoDone) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (eoDone) RestartsAfterCrash() bool               { return true }
func (eoDone) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		eoDoneSteps.Add(1)
		return &engine.StepResult{NextState: "done", Output: map[string]any{"ok": true}}, nil
	}, nil
}

// eoFails: restartable; one step that fails (counted).
var eoFailSteps atomic.Int64

type eoFails struct{}

func (eoFails) Name() string                           { return "eofails" }
func (eoFails) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (eoFails) RestartsAfterCrash() bool               { return true }
func (eoFails) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		eoFailSteps.Add(1)
		return nil, errors.New("it fails")
	}, nil
}

// eoWaker waits on eo.wake.<name>, then does its side effect (counted, with
// the event that woke it) in the step after the wake, until released.
type eoWaker struct {
	name        string
	restartable bool
	effects     *atomic.Int64
	woken       *atomic.Value // the payload of the event the last effect saw
	replayed    *atomic.Bool  // whether that event was marked replayed
	release     <-chan struct{}
}

func (m eoWaker) Name() string                           { return m.name }
func (m eoWaker) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (m eoWaker) RestartsAfterCrash() bool               { return m.restartable }
func (m eoWaker) Step(_ context.Context, st *engine.State, ev engine.Event) (engine.ActionFunc, error) {
	name := st.Name
	return func(ctx context.Context) (*engine.StepResult, error) {
		if name == "init" {
			return &engine.StepResult{NextState: "after", Wait: engine.ResumeOnTopic("eo.wake." + m.name)}, nil
		}
		m.effects.Add(1)
		m.woken.Store(ev.Payload)
		if m.replayed != nil {
			m.replayed.Store(ev.Replayed)
		}
		select {
		case <-m.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

// eoStuck: restartable; its action ignores its context until released (a
// tool blocked in a call) — a Stop has to let it go.
type eoStuck struct {
	name    string
	actions *atomic.Int64
	release <-chan struct{}
}

func (m eoStuck) Name() string                         { return m.name }
func (eoStuck) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (eoStuck) RestartsAfterCrash() bool               { return true }
func (m eoStuck) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		m.actions.Add(1)
		<-m.release
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

// eoPanics: its action panics.
type eoPanics struct{}

func (eoPanics) Name() string                           { return "eopanics" }
func (eoPanics) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (eoPanics) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		var m map[string]int
		m["boom"] = 1
		return nil, nil
	}, nil
}

// panickingFactory panics when it builds a machine (a request shape a new
// build does not expect).
type panickingFactory struct{ name string }

func (f panickingFactory) Name() string { return f.name }
func (panickingFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	panic("a request this build does not expect")
}

// A run that ended — completed or failed — while its background children
// still ran is never run again after a crash: its end is on disk before its
// checkpoint goes.
func TestAnEndedRunIsNeverRunAgain(t *testing.T) {
	cases := []struct {
		m     engine.Machine
		steps *atomic.Int64
		want  taskapi.RunStatus
	}{
		{eoDone{}, &eoDoneSteps, taskapi.RunStatusCompleted},
		{eoFails{}, &eoFailSteps, taskapi.RunStatusFailed},
	}
	for _, c := range cases {
		t.Run(c.m.Name(), func(t *testing.T) {
			dir := scratchDir(t)
			ctx := context.Background()
			first := durableService(t, dir)
			useMachine(first, c.m)
			before := c.steps.Load()
			var told sync.Map // runs whose end is told (published once both of its marks are on disk)
			unsub := first.channel.Bus().Subscribe(engine.EventTaskOutput, func(_ string, p any) {
				if e, ok := p.(*engine.TaskOutputEvent); ok {
					told.Store(e.RunID, true)
				}
			})
			defer unsub()
			r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: c.m.Name(), Group: "g"})
			if err != nil {
				t.Fatal(err)
			}
			bg := first.bgMgr.(*bgManager) // a background child still running: its parent's manifest stays
			bg.mu.Lock()
			bg.entries["bg-child-"+r.RunID] = &bgEntry{record: taskapi.TaskRecord{ID: "bg-child-" + r.RunID, ParentRunID: r.RunID}, done: make(chan struct{})}
			bg.mu.Unlock()
			eventually(t, "ended", func() bool { return status(t, first, r.RunID) == c.want })
			eventually(t, "its end told", func() bool { _, ok := told.Load(r.RunID); return ok })
			if m, err := first.runs.readManifest(r.RunID); err != nil || m.Ended != c.want {
				t.Fatalf("manifest %+v (%v); want it marked %s", m, err, c.want)
			}
			orphan(t, first.runs, r.RunID)

			second := durableService(t, dir)
			useMachine(second, c.m)
			rep := second.RestoreRuns(ctx)
			time.Sleep(50 * time.Millisecond)
			if n := c.steps.Load() - before; n != 1 || len(rep.Restarted)+len(rep.Resumed)+len(rep.Interrupted) > 0 {
				t.Fatalf("restore %+v, steps %d; an ended run was taken up again", rep, n)
			}
			if exists(second.runs.manifestPath(r.RunID)) {
				t.Error("the ended run's manifest is still on disk after the restore")
			}
			listed := false
			for _, x := range runsOf(t, second, c.m.Name()) {
				if x.RunID == r.RunID && x.Status == c.want {
					listed = true
				}
			}
			if !listed {
				t.Errorf("runs after the restart %+v; want it listed %s", runsOf(t, second, c.m.Name()), c.want)
			}
		})
	}
}

// A run woken from its wait does not wait again after a crash in the step
// after the wake: a kind not restarted is reported interrupted (its side
// effect is not done twice); a restartable one restarts with the event that
// woke it.
func TestAPassedWaitIsNotWaitedAgain(t *testing.T) {
	for _, restartable := range []bool{false, true} {
		name := map[bool]string{false: "eowaitonce", true: "eowaitrestart"}[restartable]
		t.Run(name, func(t *testing.T) {
			dir := scratchDir(t)
			ctx := context.Background()
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			var effects atomic.Int64
			var woken atomic.Value
			var replayed atomic.Bool
			m := eoWaker{name: name, restartable: restartable, effects: &effects, woken: &woken, replayed: &replayed, release: release}
			first := durableService(t, dir)
			useMachine(first, m)
			r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: name, Group: "g"})
			if err != nil {
				t.Fatal(err)
			}
			eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
			_ = first.channel.Bus().Publish("eo.wake."+name, map[string]any{"answer": "yes"})
			eventually(t, "its side effect in flight", func() bool { return effects.Load() == 1 })
			if replayed.Load() {
				t.Error("a live wake is marked replayed")
			}
			orphan(t, first.runs, r.RunID) // the process dies in the step after the wake
			woken.Store(map[string]any{})  // what the restarted step sees is its own

			second := durableService(t, dir)
			useMachine(second, m)
			rep := second.RestoreRuns(ctx)
			if !restartable {
				if len(rep.Interrupted) != 1 || len(rep.Resumed) > 0 {
					t.Fatalf("restore %+v; a run past its wait, working at the crash, is reported interrupted", rep)
				}
				_ = second.channel.Bus().Publish("eo.wake."+name, map[string]any{})
				time.Sleep(50 * time.Millisecond)
				if n := effects.Load(); n != 1 {
					t.Fatalf("its side effect ran %d times; once", n)
				}
				return
			}
			if len(rep.Restarted) != 1 {
				t.Fatalf("restore %+v; a restartable run past its wait restarts", rep)
			}
			eventually(t, "stepped again", func() bool { return effects.Load() == 2 })
			if p, _ := woken.Load().(map[string]any); p["answer"] != "yes" {
				t.Errorf("restarted with %v; want the event that woke it", woken.Load())
			}
			if !replayed.Load() {
				t.Error("restarted with the event that woke it, not marked replayed: the step cannot tell it may have run part of its work")
			}
		})
	}
}

// A run a person stopped never comes back: killed (its goroutine let go), or
// still stopping when the process died.
func TestAStoppedRunNeverComesBack(t *testing.T) {
	for _, killed := range []bool{true, false} {
		t.Run(map[bool]string{true: "killed", false: "stopping"}[killed], func(t *testing.T) {
			old := KillAfter
			KillAfter = map[bool]time.Duration{true: 50 * time.Millisecond, false: time.Hour}[killed]
			t.Cleanup(func() { KillAfter = old })
			dir := scratchDir(t)
			ctx := context.Background()
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			var actions atomic.Int64
			m := eoStuck{name: "eostuck-" + map[bool]string{true: "killed", false: "stopping"}[killed], actions: &actions, release: release}
			first := durableService(t, dir)
			useMachine(first, m)
			r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: m.name, Group: "g"})
			if err != nil {
				t.Fatal(err)
			}
			eventually(t, "working", func() bool { return actions.Load() == 1 })
			if _, err := first.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
				t.Fatal(err)
			}
			if killed {
				eventually(t, "killed", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusKilled })
			}
			orphan(t, first.runs, r.RunID)

			second := durableService(t, dir)
			useMachine(second, m)
			rep := second.RestoreRuns(ctx)
			time.Sleep(50 * time.Millisecond)
			if n := actions.Load(); n != 1 || len(rep.Restarted)+len(rep.Resumed) > 0 {
				t.Fatalf("restore %+v, actions %d; a stopped run came back", rep, n)
			}
			if !killed {
				if len(rep.Cancelled) != 1 {
					t.Fatalf("restore %+v; a run stopping at the crash is reported cancelled", rep)
				}
				if got := status(t, second, r.RunID); got != taskapi.RunStatusCancelled {
					t.Errorf("status %q; cancelled", got)
				}
			}
		})
	}
}

// An unreadable checkpoint is not "none": the run is reported interrupted,
// its file set aside — never restarted from its beginning.
func TestAnUnreadableCheckpointIsNotAFreshStart(t *testing.T) {
	release := resetStepper(t)
	defer release()
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useStepper(t, first)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working in its last step", func() bool { return stepped("work2") == 1 })
	orphan(t, first.runs, r.RunID)
	if err := writeFile(first.runs.checkpointPath(r.RunID), `{"savedAt":"2026-10-04T`); err != nil { // torn
		t.Fatal(err)
	}

	second := durableService(t, dir)
	useStepper(t, second)
	rep := second.RestoreRuns(ctx)
	time.Sleep(50 * time.Millisecond)
	if len(rep.Interrupted) != 1 || len(rep.Restarted) > 0 || stepped("init") != 1 {
		t.Fatalf("restore %+v, init steps %d; an unreadable checkpoint restarted the run", rep, stepped("init"))
	}
	if !exists(filepath.Join(dir, "bad", r.RunID+".wait.json")) {
		t.Error("the unreadable checkpoint is not set aside in runs/bad")
	}
	if info, ok := second.reg.get(r.RunID); !ok || !strings.Contains(info.err.Error(), "cannot be read") {
		t.Errorf("its error does not say its saved state cannot be read")
	}
}

// A run whose restore panics is reported interrupted; the sweep goes on.
func TestAPanickingRestoreDoesNotStopTheSweep(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var effects atomic.Int64
	var woken atomic.Value
	useMachine(first, eoWaker{name: "eopanicky", effects: &effects, woken: &woken, release: release})
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "eopanicky", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	_ = engine.MachineDomain.Types.Contribute(machineTypeDef("eopanicky2", panickingFactory{name: "eopanicky2"}))
	second.tasks["eopanicky"] = &taskapi.Task{Name: "eopanicky", Provider: "test", Type: "eopanicky2"}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	swept := make(chan struct{})
	ended := make(chan any, 1)
	go func() {
		defer func() { ended <- recover() }()
		second.ServeRestores(sctx, func() { close(swept) })
	}()
	select {
	case <-swept:
	case p := <-ended:
		t.Fatalf("the restore loop ended: %v", p)
	case <-time.After(5 * time.Second):
		t.Fatal("the first sweep never finished")
	}
	info, ok := second.reg.get(r.RunID)
	if !ok || info.info.Status != taskapi.RunStatusFailed || !strings.Contains(info.err.Error(), "panicked") {
		t.Fatalf("the run whose restore panicked is not reported interrupted with the panic")
	}
}

// A panic in a run fails that run with the panic; the others go on.
func TestAPanickingRunFailsAlone(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, eoPanics{})
	useMachine(svc, eoDone{})
	ctx := context.Background()
	bad, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eopanics"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed", func() bool { return status(t, svc, bad.RunID) == taskapi.RunStatusFailed })
	if e, _ := svc.reg.get(bad.RunID); !errors.Is(e.err, ErrRunPanicked) {
		t.Errorf("error %v; want ErrRunPanicked", e.err)
	}
	good, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eodone"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "another run completes", func() bool { return status(t, svc, good.RunID) == taskapi.RunStatusCompleted })
}

// Two sweeps at once (the periodic one and a backoff's timer) start a
// kept-alive task once.
func TestTwoSweepsAtOnceStartAKeptAliveTaskOnce(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(5)}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var actions atomic.Int64
	_ = engine.MachineDomain.Types.Contribute(machineTypeDef("eoslow", slowFactory{m: eoStuck{name: "eoslow", actions: &actions, release: release}}))
	svc.tasks["eoslow"] = &taskapi.Task{Name: "eoslow", Provider: "test", Type: "eoslow"}
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "eoslow", Group: "g"}})
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.keepAliveSweep(ctx) }()
	}
	wg.Wait()
	if n := len(runsOf(t, svc, "eoslow")); n != 1 {
		t.Fatalf("%d runs; two sweeps at once started the kept-alive task twice", n)
	}
}

// A full group queue is not an end of a kept-alive task: it is tried again,
// never stopped for it.
func TestAFullGroupDoesNotStopAKeptAliveTask(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, QueueLimit: 1}}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var actions atomic.Int64
	useMachine(svc, eoStuck{name: "eostuck-full", actions: &actions, release: release})
	ctx := context.Background()
	a, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-full", Group: "g"}) // running
	b, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-full", Group: "g"}) // queued: the queue is full
	eventually(t, "queue full", func() bool { return groupLoad(t, svc, "g").Queued == 1 })
	_ = a
	_ = b
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "eodone", Group: "g"}})
	useMachine(svc, eoDone{})
	for i := 0; i < keepAliveMaxEnds+1; i++ {
		svc.keepAliveSweep(ctx)
	}
	if k := keepAliveOf(t, svc, "eodone"); k.State == "stopped" || k.Restarts != 0 {
		t.Fatalf("kept-alive = %+v; a full queue stopped it or counted as ends", k)
	}
}

// slowFactory builds its machine after a while (an agent's model and plugins).
type slowFactory struct{ m engine.Machine }

func (f slowFactory) Name() string { return f.m.Name() }
func (f slowFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	time.Sleep(100 * time.Millisecond)
	return f.m, nil
}

func machineTypeDef(name string, f engine.MachineFactory) strategies.TypeDef[engine.MachineFactory] {
	return strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: name}, Impl: f}
}

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o600) }

// eoBgParent starts a background child (eodone, in its own group) and completes.
type eoBgParent struct{ svc *Service }

func (eoBgParent) Name() string                           { return "eobgparent" }
func (eoBgParent) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (m eoBgParent) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	runID := st.RunID
	return func(context.Context) (*engine.StepResult, error) {
		if _, err := m.svc.bgMgr.Register(runID, taskapi.TaskRunRequest{TaskName: "eodone", Group: "g", MaxDurationS: 30}); err != nil {
			return nil, err
		}
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

// A run that completes waits for its background children without holding its
// slot: with a group of one, its child runs (it would never get the slot).
func TestARunWaitingForItsChildrenHoldsNoSlot(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	useMachine(svc, eoBgParent{svc: svc})
	useMachine(svc, eoDone{})
	before := eoDoneSteps.Load()
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "eobgparent", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its background child ran", func() bool { return eoDoneSteps.Load() > before })
	e, _ := svc.reg.get(r.RunID)
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the parent never ended")
	}
}

// A run's result is kept: after a restart (its entry gone with its process)
// Result still answers — how it ended and what it gave.
func TestARunsResultIsReadAfterARestart(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useMachine(first, eoDone{})
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "eodone", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Result(ctx, taskapi.ResultRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	second := durableService(t, dir)
	got, err := second.Result(ctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil {
		t.Fatalf("Result after a restart: %v", err)
	}
	out, _ := got.Output.(map[string]any)
	if got.Status != string(taskapi.RunStatusCompleted) || out["ok"] != true {
		t.Fatalf("result after a restart %+v; want completed with its output", got)
	}
}

// A child's ended record and result are kept while its parent still goes,
// past the ended runs' limit.
func TestAChildsResultIsKeptWhileItsParentGoes(t *testing.T) {
	st, err := newRunStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close)
	st.keepEnded = 1
	if err := st.saveManifest(runManifest{RunID: "run-parent"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run-child", "run-other-1", "run-other-2"} {
		parent := ""
		if id == "run-child" {
			parent = "run-parent"
		}
		st.saveResult(savedResult{ResultResponse: taskapi.ResultResponse{RunID: id, Status: "completed"}, ParentRunID: parent})
		st.saveEnded(taskapi.RunInfo{RunID: id, ParentRunID: parent, Status: taskapi.RunStatusCompleted})
		time.Sleep(20 * time.Millisecond) // the files' times order them
	}
	if _, err := st.loadResult("run-child"); err != nil {
		t.Fatalf("the child of a running parent lost its result: %v", err)
	}
	if _, err := st.loadResult("run-other-1"); err == nil {
		t.Error("past the limit, a run with no running parent kept its result")
	}
}
