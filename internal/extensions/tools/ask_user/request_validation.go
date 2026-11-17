// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// REQUEST-VALIDATION - a conversation is not a validation: an
// agent asks a conversation question with ask-user and a validation with
// request-validation, so it always knows which it asks. request-validation
// starts a human task: a child run of the `human-task`
// strategy, with its kind, what it is about, what its answer settles, the
// permission code an answerer needs and who may answer. The person answers it
// once, from any channel, with the proof the act requires; the run's result is
// the answer. In a loop that waits as a machine the tool waits as a machine
// on the human task's end: no slot held, a restart survived.

// RequestValidationInput is what an agent asks a person to validate, decide,
// test, choose or provide.
type RequestValidationInput struct {
	Kind         string   `json:"kind" jsonschema:"What is expected of the person: approve (a validation) or decide or test-by-hand or form or provide or choose or permission or unblock or accept,required"`
	Title        string   `json:"title" jsonschema:"The question in one line,required"`
	Question     string   `json:"question,omitempty" jsonschema:"The whole question (markdown) when one line is not enough"`
	About        string   `json:"about,omitempty" jsonschema:"The object the question is about (its id): its answer is recorded on it"`
	Satisfies    []string `json:"satisfies,omitempty" jsonschema:"What its answer settles: gate requirements or verifications or steps"`
	Options      []string `json:"options,omitempty" jsonschema:"The choices (choose)"`
	FormDef      string   `json:"form_def,omitempty" jsonschema:"FormDefinition YAML (form)"`
	Requires     string   `json:"requires,omitempty" jsonschema:"The permission code an answerer must hold (default <about type>:<kind>)"`
	AnswerableBy string   `json:"answerable_by,omitempty" jsonschema:"Who may answer: person (default) or agent or any"`
	Proof        string   `json:"proof,omitempty" jsonschema:"The minimum proof of the act: prompt or totp or passkey or entra"`
}

// RequestValidationOutput is the person's answer.
type RequestValidationOutput struct {
	Status  string `json:"status" jsonschema:"success or fail or blocked - how the person answered"`
	Verdict string `json:"verdict,omitempty" jsonschema:"approved or rejected (a validation)"`
	Answer  string `json:"answer,omitempty" jsonschema:"The decision or choice or data the person gave"`
	Summary string `json:"summary,omitempty" jsonschema:"What the person said in one line"`
	By      string `json:"by,omitempty" jsonschema:"Who answered"`
	Task    string `json:"task,omitempty" jsonschema:"The human task's run"`
}

// RequestValidationTool is the request-validation tool's name.
const RequestValidationTool = "request-validation"

// NewRequestValidationTool creates the request-validation tool.
func NewRequestValidationTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		RequestValidationTool,
		"Ask a person to validate, decide, test by hand, choose or provide something - a validation, never a conversation (ask-user is the conversation). "+
			"Say first what is expected (kind) and what it is about; its answer settles what you name in satisfies. You get the person's answer: status, verdict, answer.",
		executeRequestValidation,
		toolsapi.WithReadOnly(),
		toolsapi.WithInteractive(),
		toolsapi.WithIdle(),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "✅", Label: "Validation", Color: "#C792EA", Template: "{{.title}}"}),
	)
}

// humanTaskSpec is the human task request-validation asks: what the human-task
// strategy reads as its input.
func humanTaskSpec(ctx context.Context, in RequestValidationInput) (map[string]any, error) {
	// The human task refuses a kind no one knows: only its
	// absence is caught here, before a run is started.
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		return nil, fmt.Errorf("%s: no kind - say what is expected (%s, %s, %s, …)", RequestValidationTool, inboxapi.KindApprove, inboxapi.KindDecide, inboxapi.KindTestByHand)
	}
	// The choices and a form's definition are the human task's own fields
	// : its card draws them and its answer is one of them.
	ht := inboxapi.HumanTask{
		Kind: kind, Title: strings.TrimSpace(in.Title), Question: in.Question, Options: in.Options, FormDef: in.FormDef,
		About: in.About, Satisfies: in.Satisfies,
		Proof: in.Proof, AnswerableBy: in.AnswerableBy, Requires: in.Requires, AskedBy: loopIDFromContext(ctx),
	}
	if ht.Title == "" {
		return nil, fmt.Errorf("%s: no question (title)", RequestValidationTool)
	}
	// Asked from a conversation, its card shows inline in the chat
	// , as the screens that answer it.
	ht.Chat = agentapi.RunIDFromContext(ctx)
	raw, err := json.Marshal(ht)
	if err != nil {
		return nil, err
	}
	spec := map[string]any{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// Keys of request-validation's await Resume.
const humanTaskKey = "human_task"

func executeRequestValidation(ctx context.Context, in RequestValidationInput) (RequestValidationOutput, error) {
	ts, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: no task service: %w", RequestValidationTool, err)
	}
	// Called again (its human task ended, or the run was restored): read it.
	if resume, ok := agentapi.AwaitResumeFrom(ctx); ok {
		run, _ := resume[humanTaskKey].(string)
		if run == "" {
			return RequestValidationOutput{}, fmt.Errorf("%s: called again without its human task", RequestValidationTool)
		}
		return humanTaskOutcome(ctx, ts, run)
	}
	spec, err := humanTaskSpec(ctx, in)
	if err != nil {
		return RequestValidationOutput{}, err
	}
	resp, err := ts.Run(ctx, taskapi.TaskRunRequest{
		TaskDef:     map[string]any{"name": RequestValidationTool, "type": inboxapi.HumanTaskStrategy, "description": "A validation an agent asked"},
		Input:       spec,
		ParentRunID: agentapi.RunIDFromContext(ctx),
	})
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: start the human task: %w", RequestValidationTool, err)
	}
	if agentapi.MachineWaitsFrom(ctx) {
		return RequestValidationOutput{}, awaitHumanTask(resp.RunID)
	}
	result, err := ts.Result(ctx, taskapi.ResultRequest{RunID: resp.RunID})
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: wait for the answer: %w", RequestValidationTool, err)
	}
	return outcomeOf(resp.RunID, result)
}

// awaitHumanTask is request-validation's machine wait on its human task's end.
func awaitHumanTask(run string) *agentapi.ErrAwait {
	return &agentapi.ErrAwait{
		Topic:        engine.RunEndedTopic(run),
		Resume:       map[string]any{humanTaskKey: run},
		Reason:       "a person's answer (human task " + run + ")",
		RecheckEvery: inboxRecheckEvery,
	}
}

// humanTaskOutcome is the answer once the human task has ended, else another
// wait on it.
func humanTaskOutcome(ctx context.Context, ts taskapi.TaskService, run string) (RequestValidationOutput, error) {
	st, err := ts.Status(ctx, taskapi.StatusRequest{RunID: run})
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: the human task %s is gone - ask again if you still need the answer: %w", RequestValidationTool, run, err)
	}
	switch st.Status {
	case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
	default:
		return RequestValidationOutput{}, awaitHumanTask(run)
	}
	result, err := ts.Result(ctx, taskapi.ResultRequest{RunID: run})
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: the answer of %s: %w", RequestValidationTool, run, err)
	}
	return outcomeOf(run, result)
}

// outcomeOf reads the human task's result: the person's answer.
func outcomeOf(run string, result taskapi.ResultResponse) (RequestValidationOutput, error) {
	if result.Error != "" {
		return RequestValidationOutput{}, fmt.Errorf("%s: the human task %s ended without an answer: %s", RequestValidationTool, run, result.Error)
	}
	var a inboxapi.HumanAnswer
	raw, err := json.Marshal(result.Output)
	if err == nil {
		err = json.Unmarshal(raw, &a)
	}
	if err != nil {
		return RequestValidationOutput{}, fmt.Errorf("%s: the answer of %s does not read: %w", RequestValidationTool, run, err)
	}
	return RequestValidationOutput{Status: a.Status, Verdict: a.Verdict, Answer: a.Answer, Summary: a.Summary, By: a.By.ID, Task: run}, nil
}

// RequestValidationStrategyDef is the request-validation plugin: opt-in
// (`plugins: [request-validation]`), so no agent asks a validation the
// screens do not list yet.
var RequestValidationStrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	RequestValidationTool, "Ask a person a validation (a human task)",
	func() (agentapi.PluginFactory, error) {
		tool := NewRequestValidationTool()
		return &agentapi.FuncFactory{
			FactoryName: RequestValidationTool,
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(tool)}, nil
			},
		}, nil
	},
)
