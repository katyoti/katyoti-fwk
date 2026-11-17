// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// REQUEST-VALIDATION: a validation an agent asks starts a human
// task - a child run of the human-task strategy - and its answer is the
// person's, read from the run's result.

// humanTasks is a task service that runs human tasks: Run records the
// request, Status and Result answer what the test set.
type humanTasks struct {
	taskapi.TaskService
	mu      sync.Mutex
	started []taskapi.TaskRunRequest
	status  taskapi.RunStatus
	result  taskapi.ResultResponse
}

func (h *humanTasks) Run(_ context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = append(h.started, in)
	return taskapi.TaskRunResponse{RunID: "ht-run-1"}, nil
}

func (h *humanTasks) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return taskapi.StatusResponse{RunID: in.RunID, Status: h.status}, nil
}

func (h *humanTasks) Result(_ context.Context, in taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.result
	r.RunID = in.RunID
	return r, nil
}

func validationCtx(t *testing.T, tasks *humanTasks, machine bool) context.Context {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	application.PutService[taskapi.TaskService](app, "default", tasks)
	ctx := agentapi.WithRunID(application.WithApplication(context.Background(), app), "agent-run-7")
	if machine {
		ctx = agentapi.WithMachineWaits(ctx)
	}
	return ctx
}

func answered(verdict string) taskapi.ResultResponse {
	return taskapi.ResultResponse{Status: "completed", Output: inboxapi.HumanAnswer{
		Task: "m.TD-1", Status: "success", Verdict: verdict, Summary: "looks right",
		By: cwmapi.Actor{ID: "jdoe", Kind: "person"},
	}}
}

func TestRequestValidationStartsAHumanTaskAndAwaitsItsEnd(t *testing.T) {
	tasks := &humanTasks{status: taskapi.RunStatusRunning}
	ctx := validationCtx(t, tasks, true)
	in := RequestValidationInput{Kind: "approve", Title: "Approve the release notes", About: "m.RL-033",
		Satisfies: []string{"ready-to-release/change-log-confirmed"}, Proof: "passkey"}

	_, err := executeRequestValidation(ctx, in)
	var aw *agentapi.ErrAwait
	if !errors.As(err, &aw) {
		t.Fatalf("first call: %v; want a machine wait on the human task", err)
	}
	if aw.Topic != engine.RunEndedTopic("ht-run-1") || aw.Resume[humanTaskKey] != "ht-run-1" {
		t.Errorf("await %+v; want the human task's end", aw)
	}
	if len(tasks.started) != 1 {
		t.Fatalf("%d runs started; want one human task", len(tasks.started))
	}
	run := tasks.started[0]
	if run.TaskDef["type"] != inboxapi.HumanTaskStrategy || run.ParentRunID != "agent-run-7" {
		t.Errorf("run %+v; want a human-task child of the agent's run", run)
	}
	if run.Input["kind"] != "approve" || run.Input["title"] != "Approve the release notes" || run.Input["about"] != "m.RL-033" ||
		run.Input["proof"] != "passkey" || run.Input["asked_by"] != "agent-run-7" {
		t.Errorf("what it asks %v; want the kind, title, about, proof and the asker", run.Input)
	}
	if s, _ := run.Input["satisfies"].([]any); len(s) != 1 {
		t.Errorf("satisfies %v; want what the answer settles", run.Input["satisfies"])
	}

	// Called again while the human task waits: it waits again, starts nothing.
	resumed := agentapi.WithAwaitResume(ctx, aw.Resume)
	if _, err := executeRequestValidation(resumed, in); !errors.As(err, &aw) {
		t.Fatalf("called again while unanswered: %v; want another wait", err)
	}
	// Answered: the person's answer, read from the human task's result.
	tasks.status, tasks.result = taskapi.RunStatusCompleted, answered("approved")
	out, err := executeRequestValidation(resumed, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != "approved" || out.Status != "success" || out.By != "jdoe" || out.Task != "ht-run-1" {
		t.Errorf("answer %+v; want approved by jdoe", out)
	}
	if len(tasks.started) != 1 {
		t.Errorf("%d runs started; the question is asked once", len(tasks.started))
	}
}

func TestRequestValidationOutsideAMachineLoopWaitsForTheResult(t *testing.T) {
	tasks := &humanTasks{status: taskapi.RunStatusCompleted, result: answered("rejected")}
	out, err := executeRequestValidation(validationCtx(t, tasks, false), RequestValidationInput{Kind: "test-by-hand", Title: "Open the export in Word", Options: []string{"ok", "broken"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != "rejected" {
		t.Errorf("answer %+v; want the person's rejection", out)
	}
	// The choices are the human task's own field, never text appended to its
	// question.
	var asked inboxapi.HumanTask
	raw, _ := json.Marshal(tasks.started[0].Input)
	if err := json.Unmarshal(raw, &asked); err != nil || len(asked.Options) != 2 || asked.Options[1] != "broken" || asked.Question != "" {
		t.Errorf("asked %+v (%v); want the two options as the human task's options and no question added", asked, err)
	}
}

// TestRequestValidationAsksAFormWithItsDefinition: a form
// asked through request-validation reaches its human task with its
// FormDefinition, for its card to draw it.
func TestRequestValidationAsksAFormWithItsDefinition(t *testing.T) {
	tasks := &humanTasks{status: taskapi.RunStatusCompleted, result: answered("approved")}
	const form = "fields:\n  - name: reason\n    type: text\n"
	if _, err := executeRequestValidation(validationCtx(t, tasks, false), RequestValidationInput{Kind: "form", Title: "Describe the export", FormDef: form}); err != nil {
		t.Fatal(err)
	}
	var asked inboxapi.HumanTask
	raw, _ := json.Marshal(tasks.started[0].Input)
	if err := json.Unmarshal(raw, &asked); err != nil || asked.FormDef != form || asked.Kind != "form" {
		t.Errorf("asked %+v (%v); want the form's definition", asked, err)
	}
}

func TestRequestValidationSaysWhatIsExpectedOrIsNotAsked(t *testing.T) {
	tasks := &humanTasks{}
	ctx := validationCtx(t, tasks, true)
	for _, in := range []RequestValidationInput{{Title: "Is it right?"}, {Kind: "approve"}} {
		if _, err := executeRequestValidation(ctx, in); err == nil {
			t.Errorf("%+v asked; want refused (a kind and a question)", in)
		}
	}
	if len(tasks.started) != 0 {
		t.Errorf("%d human tasks started; want none", len(tasks.started))
	}
}

func TestRequestValidationReportsAHumanTaskThatEndedWithoutAnswer(t *testing.T) {
	tasks := &humanTasks{status: taskapi.RunStatusFailed, result: taskapi.ResultResponse{Status: "error", Error: "its to-do is gone"}}
	ctx := agentapi.WithAwaitResume(validationCtx(t, tasks, true), map[string]any{humanTaskKey: "ht-run-1"})
	if _, err := executeRequestValidation(ctx, RequestValidationInput{Kind: "approve", Title: "x"}); err == nil {
		t.Error("an ended human task without an answer answered")
	}
}

func TestRequestValidationIsItsOwnToolNamedWithoutPromptWords(t *testing.T) {
	tool := NewRequestValidationTool()
	if tool.ToolDef().Name != "request-validation" || tool.ToolDef().Name == NewAskUserTool().ToolDef().Name {
		t.Errorf("tool %q; want request-validation, apart from ask-user", tool.ToolDef().Name)
	}
}
