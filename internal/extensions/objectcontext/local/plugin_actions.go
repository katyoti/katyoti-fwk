// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemacore "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// taskToolName converts a task identifier into a
// LLM-tool-name-compliant string. OpenAI / Anthropic enforce
// `^[a-zA-Z0-9_-]+$` (no colons, no slashes, no dots). Source
// identifiers come in four forms here:
//   - bare task name from a node-local `actions: [<name>]`:
//     `idea-curator-promote` → `idea_curator_promote`
//   - fully-qualified task URI: `<provider>:tasks/<name>`
//     `ktw-example:tasks/refine-scope`
//     → `ktw_example_tasks_refine_scope`
//   - hierarchical task URI:
//     `ktw-example:tasks/case/refine-scope`
//     → `ktw_example_tasks_case_refine_scope`
//   - slot-method URI (KTSP method call):
//     `ktw-example:example/case-new`
//     → `ktw_example_example_case_new`
//
// We collapse `-`, `/`, `:` to `_` so the resulting name only uses
// `[a-zA-Z0-9_]` — strictly OpenAI-compliant. The `taskFor` map on
// the plugin remembers the original identifier so dispatch routes
// to TaskService.Run with the canonical form.
func taskToolName(taskName string) string {
	s := strings.ReplaceAll(taskName, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// taskRunName is the name TaskService looks a task up by: the task URI
// `<provider>:<name>` (Task.URI) for an action URI `<provider>:tasks/<name>`,
// a bare name as is. TaskService.Get / Run do not read the action form — given
// `ktw-methods:tasks/generate-goal-features` the catalog looked for a task
// named `tasks/generate-goal-features` and never found it.
func taskRunName(ref string) string {
	if i := strings.Index(ref, ":"); i >= 0 && strings.HasPrefix(ref[i+1:], "tasks/") {
		return ref[:i] + ":" + ref[i+1+len("tasks/"):]
	}
	return ref
}

// fallbackInputSchema is the input schema attached to a task-as-tool
// when the underlying TASK.md doesn't declare an explicit
// `input-schema:`. An open `object` permits any input — the task
// itself will validate at run time.
var fallbackInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)

// nodeActionsPlugin exposes each entry from
// `NodeType.Actions` (merged across the Extends chain) as a
// typed agent tool. The tool's `Name` is the snake-case task name;
// its `InputSchema` is the task's declared `input-schema:`; its
// `Description` is the task's `description:`. `HandleToolCall`
// routes to `TaskService.Run` + `TaskService.Result` (synchronous —
// agent blocks until the task finishes, mirrors `delegate(mode:worker)`).
//
// A task the catalog does not know YET (its sidecar — ktw-methods — has not
// registered when the run starts) is not dropped: it stays pending and is
// looked up again when the tools are next read (each turn, at most every
// actionRetryEvery), and joins the tools once its provider is there.
type nodeActionsPlugin struct {
	ctx context.Context
	svc taskapi.TaskService
	now func() time.Time

	mu        sync.Mutex
	defs      []agentapi.ToolDef
	taskFor   map[string]string // tool name → original TASK.md name
	pending   []string          // task names the catalog has not answered yet
	lastTry   time.Time
	announced map[string]bool // pending names already logged
}

// actionRetryEvery bounds how often a pending task is looked up again.
const actionRetryEvery = 5 * time.Second

// newNodeActionsPlugin resolves each task name against the task catalog;
// what does not resolve yet stays pending (nodeActionsPlugin). It returns nil
// only when there is nothing to expose: no task named, or no TaskService.
func newNodeActionsPlugin(ctx context.Context, taskNames []string) agentapi.AgentPlugin {
	if len(taskNames) == 0 {
		return nil
	}
	svc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || svc == nil {
		return nil
	}
	return newNodeActions(ctx, svc, taskNames, time.Now)
}

func newNodeActions(ctx context.Context, svc taskapi.TaskService, taskNames []string, now func() time.Time) *nodeActionsPlugin {
	p := &nodeActionsPlugin{
		ctx: ctx, svc: svc, now: now,
		taskFor:   make(map[string]string, len(taskNames)),
		pending:   append([]string(nil), taskNames...),
		announced: map[string]bool{},
	}
	p.mu.Lock()
	p.resolvePendingLocked()
	p.mu.Unlock()
	return p
}

// resolvePendingLocked looks the pending task names up; what the catalog
// answers becomes a tool, the rest stays pending.
func (p *nodeActionsPlugin) resolvePendingLocked() {
	p.lastTry = p.now()
	var still []string
	for _, name := range p.pending {
		info, err := p.svc.Get(p.ctx, taskapi.GetTaskRequest{Name: taskRunName(name)})
		if err != nil || info.Name == "" {
			if !p.announced[name] {
				p.announced[name] = true
				log.Printf("[object-context actions] %q not in the task catalog yet (%v) — looked up again on the next turns", name, err)
			}
			still = append(still, name)
			continue
		}
		p.addLocked(name, info)
	}
	p.pending = still
}

func (p *nodeActionsPlugin) addLocked(name string, info taskapi.TaskInfo) {
	toolName := taskToolName(name)
	// The task declares its input in the DSL; the model speaks JSON
	// Schema. Compile at this edge rather than forwarding the authored
	// bytes — a tool whose `parameters` carry a DSL document tells the
	// model nothing it can act on, and nothing downstream would report it.
	schema := compileInput(p.ctx, info.InputSchema)
	if len(schema) == 0 {
		schema = fallbackInputSchema
	}
	description := info.Description
	if description == "" {
		description = fmt.Sprintf("Run the %q task (injected by the current node's type).", name)
	}
	p.defs = append(p.defs, agentapi.ToolDef{
		Name:        toolName,
		Description: description,
		InputSchema: schema,
	})
	p.taskFor[toolName] = name
}

func (p *nodeActionsPlugin) Name() string                { return "object-actions" }
func (p *nodeActionsPlugin) SystemMessage() string       { return "" }
func (p *nodeActionsPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }

// Stable once every task resolved: until then the tools may still grow.
func (p *nodeActionsPlugin) Stable() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pending) == 0
}

// ToolDefs is read each turn: a pending task is looked up again first.
func (p *nodeActionsPlugin) ToolDefs() []agentapi.ToolDef {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.pending) > 0 && p.now().Sub(p.lastTry) >= actionRetryEvery {
		p.resolvePendingLocked()
	}
	return append([]agentapi.ToolDef(nil), p.defs...)
}

func (p *nodeActionsPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	p.mu.Lock()
	taskName, ok := p.taskFor[name]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("nodeActionsPlugin: unknown tool %q", name)
	}
	svc, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || svc == nil {
		return nil, fmt.Errorf("nodeActionsPlugin: TaskService unavailable: %w", err)
	}
	// Decode the input as a generic map — the schema-driven validation
	// happens inside the task strategy. An empty/malformed body
	// degrades to an empty map (the task's input-schema decides
	// what's required).
	var args map[string]any
	if len(input) > 0 {
		_ = json.Unmarshal(input, &args)
	}
	resp, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: taskRunName(taskName), Input: args})
	if err != nil {
		return nil, fmt.Errorf("run %q: %w", taskName, err)
	}
	// Synchronous wait — TaskService.Result blocks if the run is
	// still in flight (per its godoc).
	res, err := svc.Result(ctx, taskapi.ResultRequest{RunID: resp.RunID})
	if err != nil {
		return nil, fmt.Errorf("result for %q (runID=%s): %w", taskName, resp.RunID, err)
	}
	return &agentapi.ToolCallResult{Data: res}, nil
}

// Compile-time interface check.
var _ agentapi.AgentPlugin = (*nodeActionsPlugin)(nil)

// compileInput turns a task's input DSL into the JSON Schema a model reads,
// through the SchemaService: the parser is the service's, never this
// package's (schema-parser-boundary). nil when the task declares none, the
// service is not there, or the DSL does not compile - the caller falls back.
func compileInput(ctx context.Context, dsl []byte) json.RawMessage {
	if len(dsl) == 0 {
		return nil
	}
	svc, err := application.UseService[schemasvc.SchemaService](ctx)
	if err != nil || svc == nil {
		return nil
	}
	out, err := svc.Parse(ctx, schemacore.ParseInput{Body: dsl})
	if err != nil || out.Schema == nil {
		return nil
	}
	js, err := out.Schema.ToJSONSchema()
	if err != nil {
		return nil
	}
	return js
}
