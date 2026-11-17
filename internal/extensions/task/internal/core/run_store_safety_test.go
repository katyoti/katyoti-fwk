// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentlocal "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// THE RUN STORE ACROSS RESTARTS — what runs/ keeps is exactly what a restart
// needs: a run is resumed at its CURRENT wait only.

// twoStepMachine waits for wake.<run>, then works (a step that does not wait)
// until twoStepRelease closes, then ends.
type twoStepMachine struct{ release chan struct{} }

var twoStepRelease chan struct{}

func (twoStepMachine) Name() string                           { return "twostep" }
func (twoStepMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (m twoStepMachine) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	switch st.Name {
	case "init":
		return func(context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{NextState: "waiting", Wait: &engine.WaitSpec{
				Conditions: []engine.WaitCondition{{Topic: "wake." + st.RunID}}, Mode: engine.WaitAny}}, nil
		}, nil
	case "waiting":
		return func(context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{NextState: "working"}, nil
		}, nil
	default:
		return func(context.Context) (*engine.StepResult, error) {
			<-m.release
			return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
		}, nil
	}
}

type twoStepFactory struct{}

func (twoStepFactory) Name() string { return "twostep" }
func (twoStepFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return twoStepMachine{release: twoStepRelease}, nil
}

func useTwoStep(svc *Service) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "twostep"}, Impl: twoStepFactory{}})
	svc.tasks["twostep"] = &taskapi.Task{Name: "twostep", Provider: "test", Type: "twostep"}
}

// A run resumed after a restart, then woken and working, keeps no checkpoint:
// a second restart while it works reports it interrupted — it is never resumed
// again at the wait it already passed (an agent would redo every turn since).
func TestAResumedRunThatWorksIsNotResumedAgain(t *testing.T) {
	twoStepRelease = make(chan struct{})
	t.Cleanup(func() { close(twoStepRelease) })
	dir := scratchDir(t)
	ctx := context.Background()

	first := durableService(t, dir)
	useTwoStep(first)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "twostep", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	useTwoStep(second)
	if rep := second.RestoreRuns(ctx); len(rep.Resumed) != 1 {
		t.Fatalf("first restart = %+v, want it resumed", rep)
	}
	_ = second.channel.Bus().Publish("wake."+r.RunID, map[string]any{})
	eventually(t, "working, its checkpoint gone", func() bool {
		return status(t, second, r.RunID) == taskapi.RunStatusRunning && !exists(second.runs.checkpointPath(r.RunID))
	})
	orphan(t, second.runs, r.RunID) // the second process dies while the run works

	third := durableService(t, dir)
	useTwoStep(third)
	rep := third.RestoreRuns(ctx)
	if len(rep.Resumed) != 0 || len(rep.Interrupted) != 1 {
		t.Errorf("second restart = %+v; want it interrupted, not resumed at the wait it passed", rep)
	}
	time.Sleep(10 * time.Millisecond)
}

// The permission profile a run was started under — even one given on the
// context only (the CLI's --permission-profile) — is kept with the run: a
// restored run runs under the same profile.
func TestARunKeepsItsPermissionProfileForARestart(t *testing.T) {
	dir := scratchDir(t)
	svc := durableService(t, dir)
	ctx := agentlocal.WithPermissionProfile(context.Background(), "compose-edit")
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	ms, err := svc.runs.manifests()
	if err != nil || len(ms) != 1 {
		t.Fatalf("manifests = %v, %v", ms, err)
	}
	if got := ms[0].Request.PermissionProfile; got != "compose-edit" {
		t.Errorf("kept profile = %q, want the one it ran under", got)
	}
	wake(t, svc, r.RunID)
}

// A wait checkpoint that cannot be written leaves no older one behind: a
// restart then reports the run interrupted instead of resuming it at an
// earlier wait.
func TestAFailedCheckpointLeavesNoOlderOne(t *testing.T) {
	st, err := newRunStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close) // its owner lock open would keep the folder
	wait := &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: "x"}}}
	if err := st.Save("r", engine.Checkpoint{State: engine.State{Name: "first-wait"}, Wait: wait}); err != nil {
		t.Fatal(err)
	}
	unwritable := engine.Checkpoint{State: engine.State{Name: "second-wait", Data: make(chan int)}, Wait: wait}
	if err := st.Save("r", unwritable); err == nil {
		t.Fatal("saving a state JSON cannot hold: want an error")
	}
	if exists(st.checkpointPath("r")) {
		cp, _ := st.Load("r")
		t.Errorf("the older checkpoint (%s) is still there: a restart would resume the run at a wait it passed", cp.State.Name)
	}
}

// A waiting run whose task changed while no process ran it (a new build, an
// edited TASK.md) is not resumed on the new definition — its saved state
// belongs to the old one: it is reported interrupted, and says why.
func TestARunWhoseTaskChangedIsNotResumed(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	first.tasks["waiter"].Body = "version 1"
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	second.tasks["waiter"].Body = "version 2"
	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 0 || len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v; want it interrupted, its task changed", rep)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if res, _ := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID}); !strings.Contains(res.Error, "changed") {
		t.Errorf("error = %q, want it to say the task changed", res.Error)
	}
	wake(t, first, r.RunID)
}

// A run whose process is gone is restored even when the OS gave that
// process's PID to another live process: its owner is judged by its lock, not
// by a PID. A run whose owner lives is left alone — and taken when that owner
// goes; the gone owner's file is then removed.
func TestARunIsRestoredWhenItsOwnerIsGoneEvenIfItsPIDLives(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })

	// While the first process lives, its run is its own.
	second := durableService(t, dir)
	if rep := second.RestoreRuns(ctx); len(rep.Resumed)+len(rep.Interrupted) != 0 {
		t.Fatalf("a live owner's run was taken: %+v", rep)
	}

	// The first process dies; its file names the PID of another live process.
	first.runs.close()
	file := first.runs.ownerPath(first.runs.owner)
	if err := os.WriteFile(file, []byte(fmt.Sprintf("pid %d", os.Getppid())), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute) // past the grace a new owner's file has
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 1 || rep.Resumed[0] != r.RunID {
		t.Fatalf("restore = %+v; want the run of the gone owner resumed", rep)
	}
	if exists(first.runs.ownerPath(first.runs.owner)) {
		t.Error("the gone owner's lease is kept")
	}
	wake(t, second, r.RunID)
	wake(t, first, r.RunID)
}
