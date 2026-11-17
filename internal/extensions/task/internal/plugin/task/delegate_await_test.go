// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// childTasks is a TaskService whose one child run ends when told.
type childTasks struct {
	taskapi.TaskService
	runs  int
	ended bool
}

func (c *childTasks) Run(context.Context, taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	c.runs++
	return taskapi.TaskRunResponse{RunID: "child-1"}, nil
}
func (c *childTasks) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	st := taskapi.RunStatusRunning
	if c.ended {
		st = taskapi.RunStatusCompleted
	}
	return taskapi.StatusResponse{RunID: in.RunID, Status: st}, nil
}
func (c *childTasks) Result(context.Context, taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	if !c.ended {
		panic("Result called before the child ended: the delegate blocked")
	}
	return taskapi.ResultResponse{Status: string(taskapi.RunStatusCompleted), Output: map[string]any{"answer": 42}}, nil
}

// In an agent loop that waits as a machine, delegating does not block on the
// child: it starts it and waits — as a machine, no slot, saved — on the
// child's end; called again, it reads the child's result, or waits again. It
// never starts a second child.
func TestADelegateWaitsAsAMachineOnItsChild(t *testing.T) {
	svc := &childTasks{}
	p := newTaskPlugin(svc, stubLoop{}, nil)
	ctx := agentapi.WithMachineWaits(context.Background())
	in := json.RawMessage(`{"task_name":"research"}`)

	_, err := p.handleDelegate(ctx, in)
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("delegate = %v; want it awaiting its child", err)
	}
	if aw.Topic != engine.RunEndedTopic("child-1") || aw.EventURI != "" {
		t.Errorf("await = %+v; want the child's end, in process", aw)
	}

	again := agentapi.WithAwaitResume(ctx, aw.Resume)
	if _, err := p.handleDelegate(again, in); !errors.As(err, &aw) {
		t.Fatalf("child still running: %v; want it awaiting again", err)
	}
	svc.ended = true
	out, err := p.handleDelegate(again, in)
	if err != nil || !strings.Contains(out, "42") {
		t.Fatalf("child ended: %q, %v; want its result", out, err)
	}
	if svc.runs != 1 {
		t.Errorf("children started = %d, want one", svc.runs)
	}
}
