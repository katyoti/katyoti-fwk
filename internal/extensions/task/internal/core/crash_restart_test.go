// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A CRASHED RUN RESTARTS — a run that was working, or queued, when its process
// died starts again after the restart: from its last saved step for a kind
// whose steps may be done again; reported interrupted for any other kind.

// stepper: init → work1 → work2 (blocks until stepperRelease closes) → done.
// It counts the steps it takes, by state.
type stepper struct{}

var stepperRuns struct {
	mu      sync.Mutex
	steps   map[string]int
	release chan struct{}
}

func stepped(name string) int {
	stepperRuns.mu.Lock()
	defer stepperRuns.mu.Unlock()
	return stepperRuns.steps[name]
}

func (stepper) Name() string                           { return "stepper" }
func (stepper) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (stepper) RestartsAfterCrash() bool               { return true }
func (stepper) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	name := st.Name
	release := stepperRuns.release
	return func(ctx context.Context) (*engine.StepResult, error) {
		stepperRuns.mu.Lock()
		stepperRuns.steps[name]++
		stepperRuns.mu.Unlock()
		switch name {
		case "init":
			return &engine.StepResult{NextState: "work1"}, nil
		case "work1":
			return &engine.StepResult{NextState: "work2"}, nil
		default:
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &engine.StepResult{NextState: "done", Output: map[string]any{"ok": true}}, nil
		}
	}, nil
}

type stepperFactory struct{}

func (stepperFactory) Name() string { return "stepper" }
func (stepperFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return stepper{}, nil
}

func useStepper(t *testing.T, svcs ...*Service) {
	t.Helper()
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "stepper"}, Impl: stepperFactory{}})
	for _, s := range svcs {
		s.tasks["stepper"] = &taskapi.Task{Name: "stepper", Provider: "test", Type: "stepper"}
	}
}

// resetStepper starts a test with no step counted; the returned func lets
// the stepper's last step end (once — also at cleanup).
func resetStepper(t *testing.T) func() {
	stepperRuns.mu.Lock()
	stepperRuns.steps = map[string]int{}
	stepperRuns.release = make(chan struct{})
	rel := stepperRuns.release
	stepperRuns.mu.Unlock()
	var once sync.Once
	release := func() { once.Do(func() { close(rel) }) }
	t.Cleanup(release)
	return release
}

// A run working when its process died restarts from its last saved step: the
// steps it had finished are not done again; the one in flight is.
func TestARunWorkingAtACrashRestartsFromItsLastStep(t *testing.T) {
	release := resetStepper(t)
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useStepper(t, first)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working in its last step", func() bool { return stepped("work2") == 1 })
	orphan(t, first.runs, r.RunID) // the process dies while it works

	second := durableService(t, dir)
	useStepper(t, second)
	rep := second.RestoreRuns(ctx)
	if len(rep.Restarted) != 1 || rep.Restarted[0] != r.RunID || len(rep.Interrupted) != 0 {
		t.Fatalf("restore = %+v; want the working run restarted", rep)
	}
	eventually(t, "the step in flight done again", func() bool { return stepped("work2") == 2 })
	if stepped("init") != 1 || stepped("work1") != 1 {
		t.Errorf("steps = init %d, work1 %d; the finished steps were done again", stepped("init"), stepped("work1"))
	}
	release()
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if res, err := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID}); err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Errorf("result = %+v, %v; want completed", res, err)
	}
}

// A run still queued when its process died — it had not started — starts
// again, from its beginning.
func TestARunQueuedAtACrashStartsAgain(t *testing.T) {
	resetStepper(t)
	stubbornRelease = make(chan struct{})
	t.Cleanup(func() { close(stubbornRelease) })
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useStepper(t, first)
	useStubborn(first, stubbornRelease)
	holder(t, first, "g")
	q, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "queued", func() bool { return groupLoad(t, first, "g").Queued == 1 })
	orphan(t, first.runs, q.RunID)

	second := durableService(t, dir)
	useStepper(t, second)
	rep := second.RestoreRuns(ctx)
	if len(rep.Restarted) != 1 || rep.Restarted[0] != q.RunID {
		t.Fatalf("restore = %+v; want the queued run started again", rep)
	}
	eventually(t, "started from its beginning", func() bool { return stepped("init") == 1 && stepped("work2") == 1 })
	_, _ = first.Stop(ctx, taskapi.StopRequest{RunID: q.RunID})
}

// A run of a kind whose steps may not be done again (a script) is reported
// interrupted, and says why.
func TestAKindNotRestartableIsReportedInterrupted(t *testing.T) {
	dir := scratchDir(t)
	first := durableService(t, dir)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(first, release)
	ctx := context.Background()
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "running", func() bool { return groupLoad(t, first, "g").Running == 1 })
	began(t, first.runs, r.RunID)
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	useStubborn(second, release)
	rep := second.RestoreRuns(ctx)
	if len(rep.Interrupted) != 1 || len(rep.Restarted) != 0 {
		t.Fatalf("restore = %+v; want it interrupted", rep)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if res, _ := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID}); !strings.Contains(res.Error, "not restarted") {
		t.Errorf("error = %q, want it to say this kind is not restarted after a crash", res.Error)
	}
}

// A run that crashes again and again without making progress is not
// restarted for ever: after three restarts it is failed, and says why.
func TestARunThatKeepsCrashingIsFailed(t *testing.T) {
	resetStepper(t)
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useStepper(t, first)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working", func() bool { return stepped("work2") == 1 })
	orphan(t, first.runs, r.RunID)

	for crash := 1; crash <= 3; crash++ {
		s := durableService(t, dir)
		useStepper(t, s)
		if rep := s.RestoreRuns(ctx); len(rep.Restarted) != 1 {
			t.Fatalf("restart %d = %+v; want restarted", crash, rep)
		}
		eventually(t, "working again", func() bool { return stepped("work2") == crash+1 })
		orphan(t, s.runs, r.RunID) // it dies again, at the same step
	}
	last := durableService(t, dir)
	useStepper(t, last)
	rep := last.RestoreRuns(ctx)
	if len(rep.Interrupted) != 1 || len(rep.Restarted) != 0 {
		t.Fatalf("fourth crash = %+v; want it failed, restarted too many times", rep)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if res, _ := last.Result(rctx, taskapi.ResultRequest{RunID: r.RunID}); !strings.Contains(res.Error, "too many times") {
		t.Errorf("error = %q, want it to say it restarted too many times", res.Error)
	}
}
