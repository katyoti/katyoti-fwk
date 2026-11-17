// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// The panels the fake server keeps for the run asked.
var (
	askingPanel = panel.NewID()
	donePanel   = panel.NewID()
	otherPanel  = panel.NewID()
)

// keptPanels is a fake PanelService: for the run it keeps panels of, the
// panels its server would keep for it - one left asking, one complete, one
// of another run (which a server filtering badly would answer); for any
// other run (one another test left, ending meanwhile), none.
type keptPanels struct {
	panelapi.PanelService
	mu     sync.Mutex
	run    string
	owners []string
}

func (k *keptPanels) List(_ context.Context, in panelapi.ListInput) (panelapi.ListOutput, error) {
	k.mu.Lock()
	k.owners = append(k.owners, in.Owner)
	run := k.run
	k.mu.Unlock()
	if in.Owner != run {
		return panelapi.ListOutput{}, nil
	}
	return panelapi.ListOutput{Panels: []panelapi.Summary{
		{Panel: askingPanel, Seq: 4, State: panel.StateAsking, Owner: in.Owner, Waiting: true},
		{Panel: donePanel, Seq: 7, State: panel.StateComplete, Owner: in.Owner},
		{Panel: otherPanel, Seq: 2, State: panel.StateOpen, Owner: "run-other"},
	}}, nil
}

// A run a restart interrupts completes the panels it left open - one left
// asking would wait for ever - with the status interrupted, at their next
// sequence; a panel already complete, or another run's, is left alone
func TestAnInterruptedRunCompletesItsOpenPanels(t *testing.T) {
	kept := &keptPanels{}
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
	kept.mu.Lock()
	kept.run = r.RunID
	kept.mu.Unlock()
	eventually(t, "running", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusRunning })
	began(t, first.runs, r.RunID)
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	sent := ownPanels(second, kept)
	if rep := second.RestoreRuns(ctx); len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v, want it interrupted", rep)
	}
	kept.mu.Lock()
	owners := append([]string(nil), kept.owners...)
	kept.mu.Unlock()
	listed := 0
	for _, o := range owners {
		if o == r.RunID {
			listed++
		}
	}
	if listed != 1 {
		t.Errorf("panels listed for %v, want the interrupted run's (%s) once", owners, r.RunID)
	}
	got := sent()
	if len(got) != 1 {
		t.Fatalf("panel events sent = %+v, want one complete of the panel left asking", got)
	}
	e := got[0]
	if e.Panel != askingPanel || e.Kind != panel.KindComplete || e.Seq != 5 || e.Complete == nil || e.Complete.Status != PanelStatusInterrupted {
		t.Errorf("sent %+v (complete %+v), want the asking panel completed interrupted at seq 5", e, e.Complete)
	}
}

// No panel server reached: the run's end is told all the same, nothing sent.
func TestAnInterruptedRunWithNoPanelServerStillEnds(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	sent := ownPanels(svc, nil)
	svc.endPanels("run-x", engine.RunInterrupted)
	if got := sent(); len(got) != 0 {
		t.Errorf("sent %+v with no panel server, want nothing", got)
	}
}

// A run's panels complete with how it ended: a run a person had stopped,
// cancelled; one a restart ended, interrupted; completed, failed or killed
// as it was.
func TestARunsPanelStatus(t *testing.T) {
	for tr, want := range map[engine.RunTransition]string{
		engine.RunCancelled:   "cancelled",
		engine.RunInterrupted: PanelStatusInterrupted,
		engine.RunCompleted:   "completed",
		engine.RunFailed:      "failed",
		engine.RunKilled:      "killed",
	} {
		if got := endedPanelStatus(tr); got != want {
			t.Errorf("endedPanelStatus(%s) = %s, want %s", tr, got, want)
		}
	}
}
