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

// universePanels is a fake panel server keeping a set of panels: a list for
// an owner answers that owner's, a list for none all of them.
type universePanels struct {
	panelapi.PanelService
	mu     sync.Mutex
	panels []panelapi.Summary
}

func (u *universePanels) List(_ context.Context, in panelapi.ListInput) (panelapi.ListOutput, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []panelapi.Summary
	for _, p := range u.panels {
		if in.Owner == "" || p.Owner == in.Owner {
			out = append(out, p)
		}
	}
	return panelapi.ListOutput{Panels: out}, nil
}

func (u *universePanels) set(ps ...panelapi.Summary) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.panels = ps
}

// ownPanels gives svc the panel server given - its own, so no other test's
// service (a sweep it left running) reaches it -; the events sent are
// answered.
func ownPanels(svc *Service, ps panelapi.PanelService) func() []panel.Event {
	var mu sync.Mutex
	var sent []panel.Event
	svc.panels = &panelSeams{
		resolve: func(context.Context) (panelapi.PanelService, error) { return ps, nil },
		send: func(_ context.Context, events []panel.Event) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, events...)
		},
	}
	return func() []panel.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]panel.Event(nil), sent...)
	}
}

// completedOf indexes the completes sent by panel: their status.
func completedOf(events []panel.Event) map[string]string {
	out := map[string]string{}
	for _, e := range events {
		if e.Kind == panel.KindComplete && e.Complete != nil {
			out[e.Panel] = e.Complete.Status
		}
	}
	return out
}

// A run a person stops completes its chat and its question panels, cancelled,
// at their next sequence - the chat leaves « À faire ».
func TestAStoppedRunCompletesItsChatPanel(t *testing.T) {
	kept := &universePanels{}
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	sent := ownPanels(svc, kept)
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	chat, ask := panel.NewID(), panel.NewID()
	kept.set(
		panelapi.Summary{Panel: chat, Seq: 9, State: panel.StateOpen, Type: panel.TypeChat, Owner: r.RunID},
		panelapi.Summary{Panel: ask, Seq: 3, State: panel.StateAsking, Type: panel.TypeQuestion, Owner: r.RunID, Waiting: true},
	)
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	if got := completedOf(sent()); len(got) != 0 {
		t.Fatalf("panels completed while the run lives: %v", got)
	}
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusCancelled })
	eventually(t, "its panels completed", func() bool { return len(completedOf(sent())) == 2 })
	got := completedOf(sent())
	if got[chat] != "cancelled" || got[ask] != "cancelled" {
		t.Errorf("completed %v, want the chat and the question cancelled", got)
	}
	for _, e := range sent() {
		if e.Panel == chat && e.Seq != 10 {
			t.Errorf("the chat completed at seq %d, want its next (10)", e.Seq)
		}
	}
}

// ownerEcho answers, for any owner asked, one open chat of that owner.
type ownerEcho struct {
	panelapi.PanelService
	chat string
}

func (o ownerEcho) List(_ context.Context, in panelapi.ListInput) (panelapi.ListOutput, error) {
	return panelapi.ListOutput{Panels: []panelapi.Summary{{Panel: o.chat, Seq: 1, State: panel.StateOpen, Type: panel.TypeChat, Owner: in.Owner}}}, nil
}

// A run that fails completes its panels failed.
func TestAFailedRunCompletesItsPanels(t *testing.T) {
	chat := panel.NewID()
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	sent := ownPanels(svc, ownerEcho{chat: chat}) // its id is known once started
	useMachine(svc, failer{})
	log := lifecycleOf(svc)
	f, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "failer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed", func() bool { return log.has(f.RunID, engine.RunFailed) })
	eventually(t, "its chat completed", func() bool { return completedOf(sent())[chat] != "" })
	if got := completedOf(sent())[chat]; got != "failed" {
		t.Errorf("the chat completed %q, want failed", got)
	}
}

// A chat its conversation's first agent mounted stays open while the run
// above it lives - a workflow goes on -; that agent's question panel ends
// with it. Once the run above ends, the sweep completes the chat.
func TestAChatEndsWithItsConversationNotItsFirstAgent(t *testing.T) {
	kept := &universePanels{}
	svc := durableService(t, scratchDir(t))
	sent := ownPanels(svc, kept)
	ctx := context.Background()
	parent, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	child := engine.ChildRunID(parent.RunID, "c")
	if _, _, err := svc.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter", ParentRunID: parent.RunID}, child, nil); err != nil {
		t.Fatal(err)
	}
	chat, ask := panel.NewID(), panel.NewID()
	kept.set(
		panelapi.Summary{Panel: chat, Seq: 5, State: panel.StateOpen, Type: panel.TypeChat, Owner: child},
		panelapi.Summary{Panel: ask, Seq: 2, State: panel.StateAsking, Type: panel.TypeQuestion, Owner: child},
	)
	eventually(t, "both waiting", func() bool {
		return status(t, svc, parent.RunID) == taskapi.RunStatusWaiting && status(t, svc, child) == taskapi.RunStatusWaiting
	})
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: child}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child's question completed", func() bool { return completedOf(sent())[ask] == "cancelled" })
	svc.sweepPanels(ctx)
	if got, ok := completedOf(sent())[chat]; ok {
		t.Fatalf("the chat completed %q while the run above its owner lives", got)
	}
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: parent.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the parent cancelled", func() bool { return status(t, svc, parent.RunID) == taskapi.RunStatusCancelled })
	kept.set(
		panelapi.Summary{Panel: chat, Seq: 5, State: panel.StateOpen, Type: panel.TypeChat, Owner: child},
		panelapi.Summary{Panel: ask, Seq: 3, State: panel.StateComplete, Type: panel.TypeQuestion, Owner: child},
	)
	svc.sweepPanels(ctx)
	if got := completedOf(sent())[chat]; got != "cancelled" {
		t.Errorf("the sweep completed the chat %q, want cancelled (how its owner ended)", got)
	}
}

// The serving process's sweep completes a panel whose owner run no record
// knows (as the measured run-1b3bc9d89f99931f) or that ended; a live run's
// panel, a run another process runs (its manifest not ended), a panel no run
// owns and a closed one are left.
func TestTheSweepCompletesThePanelsNoRunDrives(t *testing.T) {
	kept := &universePanels{}
	svc := durableService(t, scratchDir(t))
	sent := ownPanels(svc, kept)
	ctx := context.Background()
	live, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, live.RunID) == taskapi.RunStatusWaiting })
	// Another process's run: its manifest on disk, not ended.
	other := runManifest{RunID: "run-0e0e0e0e0e0e0e0e", Request: taskapi.TaskRunRequest{TaskName: "waiter"}, Owner: "elsewhere"}
	if err := writeSealedJSON(svc.runs.manifestPath(other.RunID), other); err != nil {
		t.Fatal(err)
	}
	// A run that ended: its record kept.
	svc.runs.saveEnded(taskapi.RunInfo{RunID: "run-c0c0c0c0c0c0c0c0", TaskName: "waiter", Status: taskapi.RunStatusCompleted})
	gone, ended, mine, theirs, screen, done := panel.NewID(), panel.NewID(), panel.NewID(), panel.NewID(), panel.NewID(), panel.NewID()
	kept.set(
		panelapi.Summary{Panel: gone, Seq: 4, State: panel.StateOpen, Type: panel.TypeChat, Owner: "run-1b3bc9d89f99931f"},
		panelapi.Summary{Panel: ended, Seq: 6, State: panel.StateAsking, Type: panel.TypeChat, Owner: "run-c0c0c0c0c0c0c0c0"},
		panelapi.Summary{Panel: mine, Seq: 2, State: panel.StateOpen, Type: panel.TypeChat, Owner: live.RunID},
		panelapi.Summary{Panel: theirs, Seq: 2, State: panel.StateOpen, Type: panel.TypeChat, Owner: other.RunID},
		panelapi.Summary{Panel: screen, Seq: 1, State: panel.StateOpen, Type: panel.TypeProgress, Owner: "ktw-gui"},
		panelapi.Summary{Panel: done, Seq: 8, State: panel.StateClosed, Type: panel.TypeChat, Owner: "run-1b3bc9d89f99931f"},
	)
	svc.sweepPanels(ctx)
	got := completedOf(sent())
	if got[gone] != PanelStatusRunGone || got[ended] != "completed" {
		t.Errorf("swept %v, want the unknown run's chat %q and the ended run's completed", got, PanelStatusRunGone)
	}
	for name, id := range map[string]string{"a live run's": mine, "another process's run's": theirs, "a screen's own": screen, "a closed": done} {
		if st, ok := got[id]; ok {
			t.Errorf("%s panel completed %q, want it left", name, st)
		}
	}
}
