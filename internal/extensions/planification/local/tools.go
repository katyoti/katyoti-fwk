// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// INTAKE TOOLS — what the intake agent does to a request, as agent tools over
// the request and plan services: read it and its module's requests, record its
// assessment, split it (one part per module), order it after what it waits on,
// ask its requester one question — and finish it (a duplicate, dropped) or
// block it. Its steps are its recipe's to-dos. Each move is made as the intake
// agent.

// intakeActor is who the tools move a request as.
var intakeActor = cwmapi.Actor{ID: api.RoleIntake, Kind: "agent"}

// IntakeTools is the tool group: one tool per method, named requests_<verb>.
type IntakeTools interface {
	Get(ctx context.Context, in api.GetInput) (api.GetOutput, error)
	List(ctx context.Context, in api.ListInput) (api.ListOutput, error)
	Assess(ctx context.Context, in AssessArgs) (api.RequestOutput, error)
	Split(ctx context.Context, in SplitArgs) (api.SplitOutput, error)
	Depend(ctx context.Context, in api.DependInput) (api.RequestOutput, error)
	Ask(ctx context.Context, in AskArgs) (api.RequestOutput, error)
	Finish(ctx context.Context, in FinishArgs) (api.PlannedOutput, error)
	Block(ctx context.Context, in BlockArgs) (api.PlannedOutput, error)
	File(ctx context.Context, in FileArgs) (api.FileOutput, error)
	Answer(ctx context.Context, in AnswerArgs) (api.RequestOutput, error)
}

// FileArgs files a request into a module — work asked of it, or a question.
type FileArgs struct {
	Module       string            `json:"module"                 jsonschema:"The module asked; required"`
	Title        string            `json:"title"                  jsonschema:"The ask in one line; required"`
	Details      string            `json:"details,omitempty"      jsonschema:"What the one line leaves out"`
	About        string            `json:"about,omitempty"        jsonschema:"The object it concerns (an id)"`
	Kind         string            `json:"kind,omitempty"         jsonschema:"build or fix or analyse or review or decide or maintain or question or discover"`
	Priority     string            `json:"priority,omitempty"     jsonschema:"P0 or P1 or P2 (default) or P3"`
	Deliverables []api.Deliverable `json:"deliverables,omitempty" jsonschema:"What the work must produce: a title and how to check it"`
	From         string            `json:"from,omitempty"         jsonschema:"The module asking, when a module asks another; it is the requester"`
}

// AnswerArgs answers a question.
type AnswerArgs struct {
	ID   string `json:"id"   jsonschema:"The question (a request of kind question); required"`
	Text string `json:"text" jsonschema:"The answer; required"`
}

// AssessArgs is Assess without the actor.
type AssessArgs struct {
	ID       string  `json:"id"                 jsonschema:"The request; required"`
	Kind     string  `json:"kind,omitempty"     jsonschema:"build or fix or analyse or review or decide or maintain or discover"`
	Priority string  `json:"priority,omitempty" jsonschema:"P0 (now) or P1 (next) or P2 (later) or P3 (someday)"`
	Estimate float64 `json:"estimate,omitempty" jsonschema:"Its weight: agent-hours or cost"`
	Serves   string  `json:"serves,omitempty"   jsonschema:"The want it serves: the id of a goal or a feature or a use case (build fix and maintain work serves one)"`
}

// SplitArgs is Split without the actor.
type SplitArgs struct {
	ID    string          `json:"id"             jsonschema:"The demand to split (a request or a feature or a goal); required"`
	Parts []api.SplitPart `json:"parts"          jsonschema:"Its parts: one per module (module) or per unmet goal (on); the request stays and ends with them"`
	Note  string          `json:"note,omitempty" jsonschema:"Why it is split"`
}

// AskArgs is Ask without the actor.
type AskArgs struct {
	ID       string `json:"id"                jsonschema:"The request; required"`
	Question string `json:"question"          jsonschema:"ONE well-framed question; the answer may take hours; required"`
	Context  string `json:"context,omitempty" jsonschema:"What the requester needs to answer it"`
}

// FinishArgs ends a plan without the actor.
type FinishArgs struct {
	ID      string `json:"id"                jsonschema:"The object; required"`
	Outcome string `json:"outcome,omitempty" jsonschema:"done (default) or dropped (a duplicate, or not to be done)"`
	Note    string `json:"note,omitempty"    jsonschema:"Why"`
}

// BlockArgs blocks a plan without the actor.
type BlockArgs struct {
	ID   string `json:"id"   jsonschema:"The object; required"`
	Note string `json:"note" jsonschema:"What blocks it, for the person who will look; required"`
}

type intakeTools struct{}

func requests(ctx context.Context) (api.RequestService, error) {
	return application.UseService[api.RequestService](ctx)
}

func (intakeTools) Get(ctx context.Context, in api.GetInput) (api.GetOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.GetOutput{}, err
	}
	return svc.Get(ctx, in)
}

func (intakeTools) List(ctx context.Context, in api.ListInput) (api.ListOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	return svc.List(ctx, in)
}

func (intakeTools) Assess(ctx context.Context, in AssessArgs) (api.RequestOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return svc.Assess(ctx, api.AssessInput{ID: in.ID, Kind: in.Kind, Priority: in.Priority, Estimate: in.Estimate, Serves: in.Serves, By: intakeActor})
}

func (intakeTools) Split(ctx context.Context, in SplitArgs) (api.SplitOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.SplitOutput{}, err
	}
	return svc.Split(ctx, api.SplitInput{ID: in.ID, Parts: in.Parts, Note: in.Note, By: intakeActor})
}

func (intakeTools) Depend(ctx context.Context, in api.DependInput) (api.RequestOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return svc.Depend(ctx, in)
}

func (intakeTools) Ask(ctx context.Context, in AskArgs) (api.RequestOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return svc.Ask(ctx, api.AskInput{ID: in.ID, Question: in.Question, Context: in.Context, By: intakeActor})
}

func plans(ctx context.Context) (api.PlanService, error) {
	return application.UseService[api.PlanService](ctx)
}

func (intakeTools) Finish(ctx context.Context, in FinishArgs) (api.PlannedOutput, error) {
	svc, err := plans(ctx)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	return svc.Finish(ctx, api.FinishInput{ID: in.ID, Outcome: in.Outcome, Note: in.Note, By: intakeActor})
}

func (intakeTools) Block(ctx context.Context, in BlockArgs) (api.PlannedOutput, error) {
	svc, err := plans(ctx)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	return svc.Block(ctx, api.BlockInput{ID: in.ID, Note: in.Note, By: intakeActor})
}

func (intakeTools) File(ctx context.Context, in FileArgs) (api.FileOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.FileOutput{}, err
	}
	by := intakeActor
	if in.From != "" {
		by = cwmapi.Actor{ID: in.From, Kind: api.RequesterModule}
	}
	return svc.File(ctx, api.FileInput{Module: in.Module, Title: in.Title, Details: in.Details, About: in.About,
		Kind: in.Kind, Priority: in.Priority, Deliverables: in.Deliverables, By: by})
}

func (intakeTools) Answer(ctx context.Context, in AnswerArgs) (api.RequestOutput, error) {
	svc, err := requests(ctx)
	if err != nil {
		return api.RequestOutput{}, err
	}
	return svc.Answer(ctx, api.AnswerInput{ID: in.ID, Text: in.Text, By: intakeActor})
}

// IntakeToolGroup is the intake tools: requests-get, requests-list,
// requests-assess, requests-split, requests-depend, requests-ask,
// requests-finish, requests-block — and requests-file (a module
// asks another, or files a question) and requests-answer (answers a question).
//
// They are ALLOWED without asking: intake runs headless (no one to answer a
// permission prompt — the first live run was refused on every move), and
// these tools touch nothing but planification's own objects. Every move is
// recorded as the intake agent's, and a person undoes any of them (plans
// again, finishes, blocks).
func IntakeToolGroup() []toolsapi.Tool {
	return toolsapi.NewToolGroup[IntakeTools]("requests",
		"Work asked for: read requests, record the assessment, split per module, order, ask the requester; plan the next step, finish or block; file a request or a question into a module; answer a question.",
		IntakeTools(intakeTools{}), toolsapi.WithMutating(), toolsapi.WithPermissionCheck(toolsapi.AllowFunc()))
}

const planificationSystemMessage = `Planification tools act on requests — work asked for. Read a request with
requests-get and its module's others with requests-list before deciding; every
move is recorded as the intake agent's.`

// planificationPluginStrategy registers the `planification` plugin: bind it
// with `plugins: [planification]` in a TASK.md (the intake task does).
var planificationPluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"planification", "Planification tools: evaluate requests (intake)",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "planification",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("planification", planificationSystemMessage, IntakeToolGroup())}, nil
			},
		}, nil
	},
)
