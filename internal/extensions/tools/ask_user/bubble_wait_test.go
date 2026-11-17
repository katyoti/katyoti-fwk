// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	commslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comms/local"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// childQuestions runs a child's questions as human tasks: Run puts the
// task's to-do on the parent's list, a report of it ends the run with the
// answer (as the inbox's collector does).
type childQuestions struct {
	taskapi.TaskService
	mu    sync.Mutex
	todos map[string]*schedapi.Todo
	ended map[string]inboxapi.HumanAnswer
}

func (c *childQuestions) Run(_ context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	run := "ht-" + strconv.Itoa(len(c.todos)+1)
	group, _ := in.Input["candidates"].(string)
	c.todos[run] = &schedapi.Todo{ID: "sys-planification.TD-" + run, Key: inboxapi.HumanTaskStrategy + ":" + run, Owner: inboxapi.Owner, Group: group, State: schedapi.StateReady}
	return taskapi.TaskRunResponse{RunID: run}, nil
}

func (c *childQuestions) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.ended[in.RunID]; ok {
		return taskapi.StatusResponse{RunID: in.RunID, Status: taskapi.RunStatusCompleted}, nil
	}
	return taskapi.StatusResponse{RunID: in.RunID, Status: taskapi.RunStatusRunning}, nil
}

func (c *childQuestions) Result(_ context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return taskapi.ResultResponse{RunID: in.RunID, Status: "completed", Output: c.ended[in.RunID]}, nil
}

type childQuestionTodos struct {
	schedapi.TodoService
	c *childQuestions
}

func (t childQuestionTodos) List(context.Context, schedapi.ListInput) (schedapi.TodosOutput, error) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	var out schedapi.TodosOutput
	for _, td := range t.c.todos {
		out.Todos = append(out.Todos, *td)
	}
	return out, nil
}

func (t childQuestionTodos) Pull(_ context.Context, in schedapi.PullInput) (schedapi.PullOutput, error) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	for _, td := range t.c.todos {
		if td.ID == in.ID {
			td.Claim = &schedapi.Claim{By: in.By}
			got := *td
			return schedapi.PullOutput{Todo: &got}, nil
		}
	}
	return schedapi.PullOutput{Nothing: "none"}, nil
}

func (t childQuestionTodos) Report(_ context.Context, in schedapi.ReportInput) (schedapi.TodoOutput, error) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	for run, td := range t.c.todos {
		if td.ID == in.ID {
			r := in.Result
			td.Result = &r
			t.c.ended[run] = inboxapi.HumanAnswer{Task: td.ID, Status: r.Status, Answer: r.Answer, By: r.By}
			return schedapi.TodoOutput{Todo: *td}, nil
		}
	}
	return schedapi.TodoOutput{}, errors.New("no such to-do")
}

// A child agent's question to its parent (bubble_to_parent), in a loop that
// waits as a machine, is a human task on its parent's holding list
// : it awaits the task's end holding nothing and, called again
// once its parent answered through the task's input, gets the answer.
func TestABubbledAskAwaitsAsAMachine(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), commslocal.Extension)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	questions := &childQuestions{todos: map[string]*schedapi.Todo{}, ended: map[string]inboxapi.HumanAnswer{}}
	application.PutService[taskapi.TaskService](app, "default", questions)
	application.PutService[schedapi.TodoService](app, "default", childQuestionTodos{c: questions})
	base := application.WithApplication(context.Background(), app)
	child := agentapi.WithRunID(base, "child-1")
	child = agentapi.WithPluginContext(child, map[string]any{"_parent_run_id": "parent-1", "_child_name": "painter"})
	child = agentapi.WithMachineWaits(agentapi.WithUserCommsInput(child, "bubble_to_parent"))

	_, err := executeAskUser(child, AskInput{Question: "which colour?"})
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("ask = %v; want it to await as a machine", err)
	}

	comms, err := application.UseService[commsapi.Service](base)
	if err != nil {
		t.Fatal(err)
	}
	qs := comms.PendingQuestions(base, "parent-1")
	if len(qs) != 1 {
		t.Fatalf("parent's questions = %+v", qs)
	}
	if res, err := comms.Answer(base, commsapi.AnswerRequest{QuestionID: qs[0].ID, Answer: "blue"}); err != nil || !res.Delivered {
		t.Fatalf("answer = %+v, %v", res, err)
	}

	out, err := executeAskUser(agentapi.WithAwaitResume(child, aw.Resume), AskInput{Question: "which colour?"})
	if err != nil || out.Answer != "blue" {
		t.Fatalf("called again = %+v, %v; want the parent's answer", out, err)
	}
}
