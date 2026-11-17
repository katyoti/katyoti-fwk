// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// TO-DO TOOLS — the worker contract as agent tools, for the internal worker
// (the work-todo task): read its to-do, keep it, report its result. Every move
// is made as the work-todo worker, the one a run's to-do is pulled for.

// TodoTools is the tool group: one tool per verb, named todos-<verb>.
type TodoTools interface {
	Get(ctx context.Context, in api.GetInput) (api.TodoOutput, error)
	Renew(ctx context.Context, in TodoIDArgs) (api.TodoOutput, error)
	Report(ctx context.Context, in ReportArgs) (api.TodoOutput, error)
	Release(ctx context.Context, in ReleaseArgs) (api.TodoOutput, error)
}

// TodoIDArgs names the to-do.
type TodoIDArgs struct {
	ID string `json:"id" jsonschema:"description=The to-do,required"`
}

// ReleaseArgs names the to-do and why it is given back.
type ReleaseArgs struct {
	ID   string `json:"id"   jsonschema:"description=The to-do,required"`
	Note string `json:"note" jsonschema:"description=Why it is given back,required"`
}

// ReportArgs is a report without the worker (an agent gives no verdict).
type ReportArgs struct {
	ID      string         `json:"id"             jsonschema:"description=The to-do,required"`
	Status  string         `json:"status"         jsonschema:"description=success or fail or blocked,required"`
	Summary string         `json:"summary"        jsonschema:"description=One line: what was done and how it was proven - or what failed or blocks it and what would lift it,required"`
	Details string         `json:"details,omitempty" jsonschema:"description=The detailed result when one line is not enough (markdown): what was changed and where - the tests run - what was decided and why"`
	More    map[string]any `json:"more,omitempty" jsonschema:"description=For planification: add (steps the work needs after it) and follow_ups (other work found on the way) and choice (options a person decides on a decision planification opens)"`
}

type todoTools struct{}

func todos(ctx context.Context) (api.TodoService, error) {
	return application.UseService[api.TodoService](ctx)
}

func (todoTools) Get(ctx context.Context, in api.GetInput) (api.TodoOutput, error) {
	svc, err := todos(ctx)
	if err != nil {
		return api.TodoOutput{}, err
	}
	return svc.Get(ctx, in)
}

func (todoTools) Renew(ctx context.Context, in TodoIDArgs) (api.TodoOutput, error) {
	svc, err := todos(ctx)
	if err != nil {
		return api.TodoOutput{}, err
	}
	return svc.Renew(ctx, api.RenewInput{ID: in.ID, LeaseSeconds: api.RunLeaseSeconds, By: api.Worker})
}

func (todoTools) Report(ctx context.Context, in ReportArgs) (api.TodoOutput, error) {
	svc, err := todos(ctx)
	if err != nil {
		return api.TodoOutput{}, err
	}
	return svc.Report(ctx, api.ReportInput{ID: in.ID, Result: api.Result{Status: in.Status, Summary: in.Summary, Details: in.Details, More: in.More, By: api.Worker}})
}

func (todoTools) Release(ctx context.Context, in ReleaseArgs) (api.TodoOutput, error) {
	svc, err := todos(ctx)
	if err != nil {
		return api.TodoOutput{}, err
	}
	return svc.Release(ctx, api.ReleaseInput{ID: in.ID, Note: in.Note, By: api.Worker})
}

// TodoToolGroup is the to-do tools: todos-get, todos-renew, todos-report,
// todos-release.
//
// They are ALLOWED without asking: the work-todo task runs headless, and these
// tools move nothing but the to-do its run holds (the scheduler refuses any
// other); a person undoes any move (work retry, a new plan).
func TodoToolGroup() []toolsapi.Tool {
	return toolsapi.NewToolGroup[TodoTools]("todos",
		"Your to-do: read it (get), keep it (renew), report its result (report: success or fail or blocked), or give it back (release).",
		TodoTools(todoTools{}), toolsapi.WithMutating(), toolsapi.WithPermissionCheck(toolsapi.AllowFunc()))
}

const todosSystemMessage = `To-do tools are the worker contract: your to-do is already pulled for you.
Read it with todos-get (its target object, skill, and work: instructions, params); when
done, report it with todos-report - planification decides what follows.`

// todosPluginStrategy registers the `todos` plugin: bind it with
// `plugins: [todos]` in a TASK.md (the work-todo task does).
var todosPluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"todos", "To-do tools: work one to-do (the worker contract)",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "todos",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("todos", todosSystemMessage, TodoToolGroup())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `todos` plugin — only in a binary that
// runs the agent loop (it owns the agent PluginFactory point).
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scheduler-tools", "", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, todosPluginStrategy)
		})
	}
}
