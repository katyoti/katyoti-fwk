// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/worktree"
)

// StrategyDef is the typed strategy definition for the agent.plugin domain.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"task", "Task completion + delegation tools",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "task",
			CreateFunc:  taskPluginFactory,
		}, nil
	},
)

func taskPluginFactory(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	taskSvc, _ := application.UseService[taskapi.TaskService](ctx)
	var wtMgr *worktree.Manager
	cwd, _ := os.Getwd()
	if cwd != "" {
		wtMgr = worktree.NewManager(cwd, worktree.Config{})
	}
	p := newTaskPlugin(taskSvc, loop, wtMgr)
	p.completeSchema = compileOutputSchema(ctx)
	return []agentapi.AgentPlugin{p}, nil
}

// compileOutputSchema turns the task's output contract forwarded by the agent
// factory under `_output_schema` into the JSON Schema the `complete` tool
// advertises. Without it the LLM only sees the generic {summary, files} schema
// and never produces the task's declared output fields.
//
// The contract arrives in the DSL — the one language TASK.md schemas are
// authored in — and is compiled here because the LLM speaks JSON Schema.
//
// This used to SNIFF: check whether the bytes looked like JSON Schema and pass
// them through if so, else parse them as the DSL. That branch existed because
// `output-schema:` was authored in both languages, and it made the wrong one
// work well enough that nobody fixed the split. With one language there is one
// path, and a task whose schema does not parse now fails loudly at the census
// rather than quietly reaching the model in a language it was never given.
//
// Best-effort at THIS edge: a failure falls back to the generic schema rather
// than failing the run, because the check that the schema parses has already
// happened where the author could act on it.
func compileOutputSchema(ctx context.Context) json.RawMessage {
	pc := agentapi.PluginContextFrom(ctx)
	if pc == nil {
		return nil
	}
	raw, _ := pc["_output_schema"].(string)
	return schemaparser.JSONSchemaOrNil(ctx, []byte(raw))
}

type taskPlugin struct {
	// warn warns on the run (its delegate call's run warner): what goes wrong
	// around its children while it goes on.
	warn    func(reason string)
	taskSvc taskapi.TaskService
	// loop is the NARROW introspection view — task-side code reads only
	// RunID/Mode/Messages/InheritedPlugins, never raw loop meta keys.
	loop  agentapi.LoopIntrospector
	wtMgr *worktree.Manager

	// completeSchema, when set, is the task's compiled output-schema —
	// advertised as the `complete` tool's input schema.
	completeSchema json.RawMessage
}

func newTaskPlugin(taskSvc taskapi.TaskService, loop agentapi.LoopIntrospector, wtMgr *worktree.Manager) *taskPlugin {
	return &taskPlugin{taskSvc: taskSvc, loop: loop, wtMgr: wtMgr}
}

func (p *taskPlugin) Name() string { return "task" }
func (p *taskPlugin) Stable() bool { return true }

// SystemMessage assembles only the guidance whose tools are actually present.
// The `complete` tool is filtered out of the catalog in chat mode, so its
// `## Completion` block must be suppressed there — otherwise it contradicts the
// chat protocol ("there is no completion"). The `## Delegation` block is emitted
// only when the delegate tool exists (ToolDefs adds it only with a TaskService).
func (p *taskPlugin) SystemMessage() string {
	var b strings.Builder
	if p.mode() != "chat" {
		// A task with a declared output-schema completes with THAT shape — the
		// complete tool's input schema IS the output contract. The generic
		// "call complete with a summary" wording steers the model to dump a
		// freeform summary instead of filling the schema fields, leaving
		// task.output empty. Emit the structured wording when a schema is set.
		if len(p.completeSchema) > 0 {
			b.WriteString(structuredCompletionBlock)
		} else {
			b.WriteString(completionBlock)
		}
	}
	if p.taskSvc != nil {
		b.WriteString(delegationBlock)
	}
	return b.String()
}

// mode returns this agent's loop mode ("" if not set / root). Same source as
// parentMode — the loop introspector — read here to gate prompt fragments.
func (p *taskPlugin) mode() string {
	if p.loop == nil {
		return ""
	}
	return p.loop.Mode()
}

// Hooks installs an OnTurnStart hook that drains pending TaskNotifications for
// this agent (as parent) and injects them into the conversation as system
// messages. This is the push delivery layer on top of bgManager.Notifications.
func (p *taskPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		OnTurnStart: p.drainNotifications,
	}
}

// drainNotifications is called at the start of each agent turn. It pulls
// pending background task notifications and injects them as system messages
// so the agent sees child completions without explicitly polling.
func (p *taskPlugin) drainNotifications(ctx agentapi.LoopContext, _ int) error {
	if p.taskSvc == nil {
		return nil
	}
	bgMgr := p.taskSvc.BackgroundManager()
	if bgMgr == nil {
		return nil
	}
	parentID := p.parentRunID()
	if parentID == "" {
		return nil
	}
	notifs := bgMgr.Notifications(parentID)
	if len(notifs) == 0 {
		return nil
	}
	ctx.PushMessage(agentapi.NewTextMessage(agentapi.RoleSystem, formatNotifications(notifs)))
	return nil
}

// formatNotifications renders task notifications as a concise system message.
func formatNotifications(notifs []taskapi.TaskNotification) string {
	out := "Background task updates:\n"
	for _, n := range notifs {
		line := fmt.Sprintf("- [%s] %s", n.Status, n.TaskName)
		if n.TaskID != "" {
			line += fmt.Sprintf(" (id: %s)", n.TaskID)
		}
		if n.Summary != "" {
			line += " — " + n.Summary
		}
		if n.Error != "" {
			line += " — error: " + n.Error
		}
		out += line + "\n"
	}
	return out
}

// parentRunID returns this agent's own run ID, to be used as the parent_run_id
// for any child task spawned via delegate. Read through the loop introspector
// (LoopIntrospector.RunID). Returns "" for top-level runs (which is fine — the
// child becomes a root in the delegation tree).
func (p *taskPlugin) parentRunID() string {
	if p.loop == nil {
		return ""
	}
	return p.loop.RunID()
}

// permissionProfile returns this agent's active kernel permission profile name
// (empty = the configured default), read through the loop introspector. The
// delegate paths stamp it onto every child request so a worker inherits the
// parent's profile — children run on the bg app context and cannot pick it up
// from ctx the way an in-process call would.
func (p *taskPlugin) permissionProfile() string {
	if p.loop == nil {
		return ""
	}
	return p.loop.PermissionProfile()
}

func (p *taskPlugin) ToolDefs() []agentapi.ToolDef {
	// Use typed tool definitions — schemas auto-generated from Go structs
	defs := []agentapi.ToolDef{CompleteToolDef(p.completeSchema)}

	// Only add delegate + task-status if TaskService is available
	if p.taskSvc != nil {
		defs = append(defs, DelegateToolDef(), TaskStatusToolDef())
	}

	return defs
}

func (p *taskPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	switch name {
	case "complete":
		s, err := p.handleComplete(input)
		if err != nil {
			return nil, err
		}
		return agentapi.StringResult(s), nil
	case "delegate":
		s, err := p.handleDelegate(ctx, input)
		if err != nil {
			return nil, err
		}
		return agentapi.StringResult(s), nil
	case "task-status":
		s, err := p.handleTaskStatus(input)
		if err != nil {
			return nil, err
		}
		return agentapi.StringResult(s), nil
	default:
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
}

func (p *taskPlugin) handleComplete(input json.RawMessage) (string, error) {
	var params struct {
		Summary string   `json:"summary"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	// Tasks with a declared output-schema complete with THAT shape (the complete
	// tool's input schema IS the output contract) — so the full structured input
	// becomes the run's Output map, letting a programmatic consumer read
	// task.output as the declared envelope. Schema-less tasks keep the generic
	// {summary, files} shape and a nil Output.
	var structured map[string]any
	if len(p.completeSchema) > 0 {
		// input decoded into a struct above: an object, or null.
		if err := json.Unmarshal(input, &structured); err != nil || structured == nil {
			return "", fmt.Errorf("complete: this task declares its output — give it as an object of that shape")
		}
	}
	// Keep the run summary meaningful even when the schema carries no summary
	// field (the structured payload lives in Output / the journaled tool call).
	if params.Summary == "" {
		params.Summary = "task completed"
	}
	return "", &agentapi.ErrRunComplete{
		Summary: params.Summary,
		Files:   params.Files,
		Output:  structured,
	}
}

func (p *taskPlugin) handleTaskStatus(input json.RawMessage) (string, error) {
	if p.taskSvc == nil {
		return "", fmt.Errorf("TaskService not available")
	}

	var params struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	bgMgr := p.taskSvc.BackgroundManager()
	record, err := bgMgr.TaskStatus(params.TaskID)
	if err != nil {
		return "", err
	}

	data, _ := json.Marshal(record)
	return string(data), nil
}

// parentMode reads the parent agent's own delegation mode through the loop
// introspector (LoopIntrospector.Mode). Returns "" if not set (root task).
// Used by the BuildChildOverride callers above so the spawn-policy machinery
// can apply parent-derived restrictions (e.g., coordinator-restricts-children)
// to spawned children.
func (p *taskPlugin) parentMode() string {
	if p.loop == nil {
		return ""
	}
	return p.loop.Mode()
}

const completionBlock = "## Completion\n\n" +
	"When ALL deliverables are produced, call the **complete** tool with a summary.\n" +
	"Always call **complete** when you are done — do not just stop talking.\n\n"

// structuredCompletionBlock replaces completionBlock when the task declares an
// output-schema: the complete tool's input schema IS the output contract, so the
// model must FILL those fields (not produce a freeform summary that lands in
// task.output empty).
const structuredCompletionBlock = "## Completion\n\n" +
	"When ALL deliverables are produced, call the **complete** tool. Its input " +
	"schema IS this task's declared output — fill those fields directly from your " +
	"result. Do NOT pass a freeform summary; the structured fields ARE the output a " +
	"consumer reads.\n" +
	"Always call **complete** when you are done — do not just stop talking.\n\n"

const delegationBlock = "## Delegation\n\n" +
	"Call the **delegate** tool to hand work to a specialist.\n" +
	"Use `mode` to select the delegation mode:\n" +
	"- `worker` (default) — synchronous, waits for result\n" +
	"- `background` — asynchronous, returns task_id immediately\n" +
	"- `fork` — inherits your full conversation context (always async)\n" +
	"- `isolated` — runs in an isolated git worktree (always async)\n" +
	"- `fork-isolated` — fork + worktree combined (always async)\n" +
	"- `agent_as_human` — runs concurrently AND routes the child's ask-user " +
	"questions to YOU; you answer them with the **answer-child** tool (you act " +
	"as the child's user). Use when you want to drive/answer a child task.\n" +
	"Use the **task_status** tool to check progress of background tasks.\n\n"
