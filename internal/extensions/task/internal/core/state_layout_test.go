// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// versionedWaiter is a waiter whose saved-state layout is versionedLayout.
type versionedWaiter struct{ waiterMachine }

var versionedLayout = "versioned/1"

func (versionedWaiter) StateLayout() string { return versionedLayout }

type versionedFactory struct{}

func (versionedFactory) Name() string { return "versioned" }
func (versionedFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return versionedWaiter{}, nil
}

// A waiting run saved by a machine whose state layout has changed since (a new
// build) is not resumed on the new layout — it would be read half: it is
// reported interrupted, and says why.
func TestARunSavedUnderAnotherStateLayoutIsNotResumed(t *testing.T) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "versioned"}, Impl: versionedFactory{}})
	versionedLayout = "versioned/1"
	t.Cleanup(func() { versionedLayout = "versioned/1" })
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	first.tasks["versioned"] = &taskapi.Task{Name: "versioned", Provider: "test", Type: "versioned"}
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "versioned", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)

	versionedLayout = "versioned/2" // a new build changed the state's shape
	second := durableService(t, dir)
	second.tasks["versioned"] = first.tasks["versioned"]
	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 0 || len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v; want it interrupted, its state's layout changed", rep)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if res, _ := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID}); !strings.Contains(res.Error, "layout") {
		t.Errorf("error = %q, want it to say the saved state's layout changed", res.Error)
	}
	wake(t, first, r.RunID)
}
