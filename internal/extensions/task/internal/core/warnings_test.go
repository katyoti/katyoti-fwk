// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

func warned(t *testing.T, svc *Service, id, want string) bool {
	t.Helper()
	for _, w := range runInfo(t, svc, id).Warnings {
		if strings.Contains(w, want) {
			return true
		}
	}
	return false
}

// A run that goes on although something went wrong around it says so — in
// Runs and as a warning transition (journalled): a group it names that is not
// declared, a journal that could not be opened. (A log line only.)
func TestARunGoesOnWithItsWarningsShown(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	log := lifecycleOf(svc)
	file := filepath.Join(scratchDir(t), "not-a-folder")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.sessionsDir = file // its journal cannot be opened under a file
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", Group: "nope"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	if !warned(t, svc, r.RunID, `"nope"`) || !warned(t, svc, r.RunID, "journal could not be opened") {
		t.Errorf("warnings = %q; want the undeclared group and the journal", runInfo(t, svc, r.RunID).Warnings)
	}
	if !log.has(r.RunID, engine.RunWarning) {
		t.Error("no warning transition told")
	}
}

type undeletableStore struct{ engine.StateStore }

func (undeletableStore) Delete(string) error { return errors.New("access denied") }

// A saved state that could not be removed is a warning on its run.
func TestASavedStateNotRemovedIsAWarning(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	_ = tellingStore{StateStore: undeletableStore{}, s: svc}.Delete(r.RunID)
	if !warned(t, svc, r.RunID, "access denied") {
		t.Errorf("warnings = %q; want the saved state not removed", runInfo(t, svc, r.RunID).Warnings)
	}
	wake(t, svc, r.RunID)
}

// A run manifest the task service cannot read is set aside (runs/bad) and
// reported once, in Groups — it was skipped silently at every sweep.
func TestABadManifestIsSetAsideAndReportedOnce(t *testing.T) {
	dir := scratchDir(t)
	svc := durableService(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "broken.run.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc.RestoreRuns(context.Background())
	svc.RestoreRuns(context.Background())
	gs, err := svc.Groups(context.Background(), taskapi.GroupsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, w := range gs.Warnings {
		if strings.Contains(w, "broken.run.json") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("service warnings = %q; want the bad manifest reported once", gs.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad", "broken.run.json")); err != nil {
		t.Errorf("not set aside in runs/bad: %v", err)
	}
}

// A service warning is kept once, however often it recurs (a manifest that
// cannot even be moved aside is met at every sweep).
func TestAServiceWarningIsKeptOnce(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.serviceWarn("the runs folder cannot be read")
	svc.serviceWarn("the runs folder cannot be read")
	if ws := svc.serviceWarnings(); len(ws) != 1 {
		t.Errorf("warnings = %q; want it once", ws)
	}
}

// warner warns on its run from its step, then ends.
type warner struct{}

func (warner) Name() string                           { return "warner" }
func (warner) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (warner) Step(ctx context.Context, _ *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	return func(ctx context.Context) (*engine.StepResult, error) {
		engine.WarnRun(ctx, "a hook it uses failed")
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

// A machine warns on its run from its step's context — an agent's hooks,
// its permission gate — and the warning is the run's.
func TestAMachinesWarningIsItsRuns(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, warner{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "warner"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "warned", func() bool { return warned(t, svc, r.RunID, "a hook it uses failed") })
}
