// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// slotSuspensions records the waits run without the run's slot.
type slotSuspensions struct {
	taskapi.SuspensionManager
	waiting bool
	runIDs  []string
}

func (s *slotSuspensions) WhileWaiting(_ context.Context, taskID string, wait func()) error {
	s.runIDs = append(s.runIDs, taskID)
	s.waiting = true
	wait()
	s.waiting = false
	return nil
}

type slotTasks struct {
	taskapi.TaskService
	sm *slotSuspensions
}

func (s slotTasks) Repeat(context.Context, taskapi.RepeatRequest) (taskapi.TaskRunResponse, error) {
	return taskapi.TaskRunResponse{}, nil
}
func (s slotTasks) SuspensionManager() taskapi.SuspensionManager { return s.sm }

// promptingGate answers, noting whether the run's slot was given back then.
type promptingGate struct {
	sm        *slotSuspensions
	slotFreed bool
	prompted  bool
}

func (g *promptingGate) Prompt(context.Context, hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	g.prompted, g.slotFreed = true, g.sm.waiting
	return hgapi.PromptResponse{Answer: "yes"}, nil
}
func (g *promptingGate) PromptPermission(context.Context, hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	return hgapi.PromptPermissionResponse{}, nil
}
func (g *promptingGate) Capabilities() []string { return []string{"text"} }

// A direct ask — a person answering at a live UI — waits without its run's
// group slot: the person may take minutes, others' runs go on meanwhile.
func TestADirectAskWaitsWithoutItsSlot(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	sm := &slotSuspensions{}
	application.PutService[taskapi.TaskService](app, "default", slotTasks{sm: sm})
	gate := &promptingGate{sm: sm}
	ctx := agentapi.WithRunID(application.WithApplication(context.Background(), app), "run-7")
	ctx = hgapi.WithHumanGate(agentapi.WithUserCommsInput(ctx, "direct"), gate)

	out, err := executeAskUser(ctx, AskInput{Question: "ok?"})
	if err != nil || out.Answer != "yes" {
		t.Fatalf("ask = %+v, %v", out, err)
	}
	if !gate.prompted || !gate.slotFreed {
		t.Errorf("prompted %v, slot given back while the person answered %v; want both", gate.prompted, gate.slotFreed)
	}
	if len(sm.runIDs) != 1 || sm.runIDs[0] != "run-7" {
		t.Errorf("waits = %v, want one for the asking run", sm.runIDs)
	}
}
