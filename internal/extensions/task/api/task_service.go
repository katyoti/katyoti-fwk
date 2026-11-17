// SPDX-License-Identifier: Apache-2.0

// Package api defines the TaskService interface — the single public interface
// for running all tasks. Tasks = machines. Every task type (agent-loop, workflow,
// program, template, human-gate) is a Machine in MachineDomain.
//
// TaskService is an L4 published service (singleton). Internally it uses the
// Task Engine (L5) — MachineDomain, MachineFactory, Executor, StateStore.
package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// TaskServiceDef describes the TaskService for KTSP publishing.
//
// Naming: plural `tasks` to match `skills`. Service slot path is `tasks/*`,
// auto-generated CLI is `ktw-agents tasks <verb>`. Three output modes
// (rich/raw/json) come for free via the KTSP container's auto-generated
// `--out` flag. Rich rendering uses the templates declared on each method.
var TaskServiceDef = meta.ServiceDef{
	Name:  "tasks",
	Short: "Task definitions and execution engine",
	Long: `Lists task definitions, executes them as state machines via MachineDomain,
and tracks live runs (start / status / stop / result). Single public
interface for all task execution.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List": {
			Short:          "List available task definitions",
			RenderTemplate: tasksListTemplate,
		},
		"Get": {
			Short:          "Get task details",
			RenderTemplate: taskGetTemplate,
		},
		"Run": meta.MethodLong(
			"Start a task, returns run ID",
			`Spawns a task asynchronously. Returns the run ID immediately;
events stream via the KTSP terminal protocol. Use 'tasks status <runID>'
or 'tasks result <runID>' to track progress.`,
		),
		"Status": {
			Short:          "Query run state",
			RenderTemplate: tasksStatusTemplate,
		},
		"Stop": meta.Method("Stop a run: cancel it, kill it if it is still alive after the grace period; a queued run is removed"),
		"Groups": {
			Short:          "The task groups: per group, runs running, waiting and queued, and its limits",
			RenderTemplate: tasksGroupsTemplate,
		},
		"SetGroup":   meta.Method("Change a task group's limits live, or pause / resume it"),
		"ClearGroup": meta.Method("Remove the queued runs of a task group (all, or one origin's)"),
		"Result":     meta.Method("Get final result (blocks if still running)"),
		"Repeat":     meta.Method("Start an ended run again: a new run of the same request (an interrupted, failed or stopped one taken up by a person)"),
		"RunStages": {
			Short:    "A run's steps as a progress list's stages: one per task its child runs ran, in order",
			Long:     "Read-only: the run's direct child runs - a workflow's delegate steps, each parallel branch or foreach item - grouped by task, in order of first start, each {id, label, state (running, done, failed), done, total, message, ms}: done/total over that task's runs, failed with the first error, running while one is active, its time from first start to last end; done once the run ended. What display's progress-list polls to follow a workflow run live. A call step starts no child run and shows no stage.",
			ReadOnly: true,
		},
		"Runs": {
			Short:          "List the runs, newest first (filter by session, group, status; status active = queued, running or waiting)",
			RenderTemplate: tasksRunsTemplate,
		},
	},
}

// TaskService is the published interface for all task execution.
// Scope: Singleton. Manages ALL active runs across all sessions.
type TaskService interface {
	Run(ctx context.Context, in TaskRunRequest) (TaskRunResponse, error)
	Status(ctx context.Context, in StatusRequest) (StatusResponse, error)
	Stop(ctx context.Context, in StopRequest) (StopResponse, error)
	Result(ctx context.Context, in ResultRequest) (ResultResponse, error)
	// Repeat starts an ended run again, from the request it was started with:
	// a new top-level run under a new id.
	Repeat(ctx context.Context, in RepeatRequest) (TaskRunResponse, error)
	Runs(ctx context.Context, in ListRunsRequest) (ListRunsResponse, error)
	// RunStages is a run's steps as a progress list's stages.
	RunStages(ctx context.Context, in RunStagesRequest) (RunStagesResponse, error)
	// The task groups (TaskGroups.yaml): their load, their limits live, and
	// clearing their queues.
	Groups(ctx context.Context, in GroupsRequest) (GroupsResponse, error)
	SetGroup(ctx context.Context, in SetGroupRequest) (GroupLoad, error)
	ClearGroup(ctx context.Context, in ClearGroupRequest) (ClearGroupResponse, error)
	List(ctx context.Context, in ListTasksRequest) (ListTasksResponse, error)
	Get(ctx context.Context, in GetTaskRequest) (TaskInfo, error)
	BackgroundManager() BackgroundTaskManager
	SuspensionManager() SuspensionManager
}

// ── Run ─────────────────────────────────────────────────────────────────

// TaskRunRequest defines what to run and with what input.
// 100% JSON-serializable. No type-specific fields — the task definition
// (loaded by name/file or provided inline) carries all type-specific config.
type TaskRunRequest struct {
	// What to run (union — exactly one):
	TaskName string         `json:"taskName,omitempty" desc:"Task name from registry"`
	TaskFile string         `json:"taskFile,omitempty" desc:"Path to task YAML/MD file"`
	TaskDef  map[string]any `json:"taskDef,omitempty" desc:"Inline task definition (must have 'type' field)"`

	// Runtime input (passed to the task factory):
	Input map[string]any `json:"input,omitempty" desc:"Task input arguments"`

	// Execution context:
	WorkspaceRoot string `json:"workspaceRoot,omitempty" desc:"Workspace directory"`
	ParentRunID   string `json:"parentRunId,omitempty" desc:"Parent run ID (empty = root)"`
	SessionID     string `json:"sessionId,omitempty" desc:"Session ID"`

	// Group is the task group the run is processed in (TaskGroups.yaml): it
	// limits how many runs of the group run at once. Empty: the parent run's
	// group, else the default group (user).
	Group string `json:"group,omitempty" desc:"Task group (TaskGroups.yaml); empty = the parent's, else the default group"`
	// Priority orders the run in its group's queue: higher first, then submit
	// order.
	Priority int `json:"priority,omitempty" desc:"Order in the group's queue: higher first"`
	// Origin is who started the run; empty = an agent when ParentRunID is set,
	// else a person.
	Origin *RunOrigin `json:"origin,omitempty" desc:"Who starts the run: a person, the planner, an agent"`
	// Target is the object the run works on, when it has one.
	Target string `json:"target,omitempty" desc:"The object the run works on"`

	// ConfigOverride merges caller-supplied config fields on top of the task's
	// static config: block from TASK.md. Use cases: per-spawn permissions,
	// delegation mode overrides, model overrides at invocation time.
	// The caller is responsible for using field names the strategy understands.
	ConfigOverride map[string]any `json:"configOverride,omitempty" desc:"Per-invocation config overrides merged onto task config"`

	// PermissionProfile names the kernel permission profile this run uses when
	// no explicit profile is already in context. The delegate tool propagates
	// the PARENT's active profile here so a parent that authors files outside
	// the workspace subdir (e.g. the compose wizard under `compose-edit`) keeps
	// that profile for the workers it spawns — children run on the bg manager's
	// app context, not the parent's, so the profile cannot ride ctx into them.
	// Empty → the task-declared profile (TASK.md `permissionProfile:`), else
	// the configured default.
	PermissionProfile string `json:"permissionProfile,omitempty" desc:"Kernel permission profile for this run (delegated children inherit the parent's)"`

	// BackgroundID is, for a run started as a background task (a delegate in
	// the background), that task's id — the id its parent knows it by. Saved
	// with the run: after a restart its parent still learns how it ended.
	BackgroundID string `json:"backgroundId,omitempty" desc:"The background task id a parent knows this run by (set by the background task manager)"`

	// MaxDurationS is the wall-clock budget for this run in seconds.
	// 0 = unlimited. Enforced by BackgroundTaskManager via context.WithTimeout
	// when the run is launched as a background task. Foreground runs inherit
	// the caller's context, so duration limits apply via the caller's context.
	MaxDurationS int `json:"maxDurationS,omitempty" desc:"Wall-clock budget in seconds (0=unlimited)"`

	// FireAndForget signals that the parent does not want a completion
	// notification (no entry in the parent's notification queue). Used by
	// fire_and_forget scheduling mode. Only meaningful when launched as background.
	FireAndForget bool `json:"fireAndForget,omitempty" desc:"Skip parent notification on completion"`

	// DryRunMode replaces every spawned LLM-based child (agent, task, human-gate,
	// program) with a synthetic simulator that returns a deterministic placeholder
	// output. Used to validate workflow topology + event flow without burning LLM
	// tokens or triggering side-effects. Inherited by nested workflows.
	DryRunMode bool `json:"dryRunMode,omitempty" desc:"Replace LLM/program spawns with simulator (zero-cost validation)"`

	// CurrentNode is the CWM context node the task is operating on, when the
	// task's Kind is `node`. Opaque map[string]any — no compile-time CWM
	// dependency. The strategy + task body read it however they like (e.g.
	// agent template variables `{{.node.id}}`). Workflow children inherit
	// this from their parent unless explicitly overridden. Ignored for
	// `global` tasks (the default).
	CurrentNode map[string]any `json:"currentNode,omitempty" desc:"CWM context node for kind:node tasks (opaque map)"`
}

// TaskRunResponse is the output of TaskService.Run — mirrors the
// shape of StatusResponse minus the live-only Duration field. Carries
// the full run identity so callers can immediately wire bus
// subscriptions, journal lookups, and UI panels without a follow-up
// Status round-trip.
//
// SessionID is the journal session id (`<dateT>_<taskName>` form,
// matches the on-disk session directory under `<DataDir>/sessions/`).
// If the request didn't supply one, Run() generates it the same way
// RunID is generated and returns it here so the caller can lock onto
// the same session for subscriptions / journal reads / output views.
type TaskRunResponse struct {
	RunID       string    `json:"runId"       desc:"Unique run identifier"`
	TaskName    string    `json:"taskName"    desc:"Task that was started"`
	MachineType string    `json:"machineType" desc:"Strategy (agent / workflow / program / …)"`
	State       string    `json:"state,omitempty" desc:"Initial machine state name"`
	Status      RunStatus `json:"status"      desc:"Run lifecycle status (will be 'running' on success)"`
	ParentRunID string    `json:"parentRunId,omitempty" desc:"Parent run id (empty for root runs)"`
	SessionID   string    `json:"sessionId,omitempty" desc:"Journal session id — caller-supplied or auto-generated (<dateT>_<taskName>)"`
	StartedAt   time.Time `json:"startedAt"   desc:"Wall-clock start time"`
}

// ── Status ──────────────────────────────────────────────────────────────

type StatusRequest struct {
	RunID string `json:"runId" desc:"Run ID to query"`
}

type StatusResponse struct {
	RunID       string    `json:"runId"`
	TaskName    string    `json:"taskName"`
	MachineType string    `json:"machineType"`
	State       string    `json:"state" desc:"Current machine state name"`
	Status      RunStatus `json:"status"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	Duration    string    `json:"duration,omitempty"`
}

// ── Stop ────────────────────────────────────────────────────────────────

// RepeatRequest names the ended run to start again, and who asks for it.
type RepeatRequest struct {
	RunID  string     `json:"runId" desc:"The ended run to start again"`
	Origin *RunOrigin `json:"origin,omitempty" desc:"Who asks for it (a person by default)"`
}

type StopRequest struct {
	RunID string `json:"runId" desc:"Run ID to cancel"`
}

type StopResponse struct {
	Stopped bool `json:"stopped" desc:"Whether the run was stopped"`
}

// ── Result ──────────────────────────────────────────────────────────────

type ResultRequest struct {
	RunID string `json:"runId" desc:"Run ID to get result for"`
	// WaitS bounds the wait for the run's end: past it, the answer is the
	// run's status now (a queued run with why it has no slot). 0 = until it ends.
	WaitS int `json:"waitS,omitempty" desc:"Seconds to wait for the run's end; past them its status now is answered (0 = until it ends)"`
}

type ResultResponse struct {
	RunID    string `json:"runId"`
	Status   string `json:"status" desc:"Outcome: completed, error, cancelled"`
	Output   any    `json:"output,omitempty" desc:"Task output (type-specific)"`
	Duration string `json:"duration,omitempty" desc:"Execution duration"`
	Error    string `json:"error,omitempty" desc:"Error message if failed"`
	// QueuedBecause: a run answered still queued (WaitS passed) says why it
	// has no slot.
	QueuedBecause string `json:"queuedBecause,omitempty" desc:"Why a run still queued has no slot (its group off or full or the universe's ceiling)"`
}

// ── Runs ────────────────────────────────────────────────────────────────

type ListRunsRequest struct {
	SessionID string `json:"sessionId,omitempty" desc:"Filter by session ID"`
	Group     string `json:"group,omitempty"     desc:"Only the runs of this task group"`
	// Status filters by status; "active" = queued, running or waiting.
	Status string `json:"status,omitempty" desc:"Only runs in this status (active = queued, running or waiting)"`
}

// RunStagesRequest names the run whose steps are followed.
type RunStagesRequest struct {
	Run string `json:"run" desc:"The run - a workflow - whose child runs are its stages" required:"true"`
}

// RunStagesResponse is what a progress list polls: the stages, done once the
// run ended.
type RunStagesResponse struct {
	Done   bool       `json:"done"`
	Stages []RunStage `json:"stages"`
}

// RunStage is one task the run's child runs ran.
type RunStage struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	State   string `json:"state" desc:"running, done or failed"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty" desc:"The first error, when one failed"`
	Ms      int64  `json:"ms,omitempty" desc:"From its first start to its last end, once ended"`
}

type ListRunsResponse struct {
	Runs []RunInfo `json:"runs"`
}

type RunInfo struct {
	RunID       string    `json:"runId"`
	TaskName    string    `json:"taskName"`
	MachineType string    `json:"machineType"`
	Status      RunStatus `json:"status"`
	State       string    `json:"state,omitempty"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	SessionID   string    `json:"sessionId,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	// Group is the task group the run is processed in.
	Group    string    `json:"group,omitempty"`
	Priority int       `json:"priority,omitempty"`
	Origin   RunOrigin `json:"origin"`
	Target   string    `json:"target,omitempty"`
	// EndedAt is when the run ended; nil while it has not.
	EndedAt *time.Time `json:"endedAt,omitempty"`
	// Error is why a failed or killed run ended so — an interrupted run says
	// it was interrupted by a restart, and why.
	Error string `json:"error,omitempty"`
	// QueuedBecause is why a queued run has no slot yet (its group off or
	// full, the universe's ceiling reached); empty once it has one.
	QueuedBecause string `json:"queuedBecause,omitempty"`
	// StuckSince: the run works but has taken no step since then (the
	// stuck-run watchdog); nil while it moves.
	StuckSince *time.Time `json:"stuckSince,omitempty"`
	// Warnings are what went wrong around the run while it went on (its
	// manifest or journal not written, its saved state not removed, …), the
	// last five.
	Warnings []string `json:"warnings,omitempty"`
}

// RunOrigin is who started a run.
type RunOrigin struct {
	// Kind: person · planner · agent (a parent run) · system.
	Kind string `json:"kind"`
	// ID: the person, the planner's job, the parent run.
	ID string `json:"id,omitempty"`
}

// Run origin kinds.
const (
	OriginPerson  = "person"
	OriginPlanner = "planner"
	OriginAgent   = "agent"
	OriginSystem  = "system"
	// OriginKeepAlive: started by the universe, which keeps the task running
	// (TaskGroups.yaml keep_alive); ID is the task.
	OriginKeepAlive = "keep-alive"
	// OriginWorkflow: started by a workflow, as one of its steps; ID is the
	// workflow's run.
	OriginWorkflow = "workflow"
)

// ── List / Get ──────────────────────────────────────────────────────────

type ListTasksRequest struct{}

type ListTasksResponse struct {
	Tasks []TaskSummary `json:"tasks"`
}

type GetTaskRequest struct {
	Name string `json:"name" desc:"Task name"`
}

// TaskSummary is a compact task descriptor returned by ListTasks — enough
// to pick a task without downloading its full definition. For the full
// definition (config + body), call GetTask.
//
// Capability is a derived single-word label extracted from the task's
// frontmatter config, commonly used by task pickers as a sorting key.
// It is surfaced here (rather than hidden inside Config) because it is
// the only piece of metadata a picker needs besides name/type/description.
type TaskSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"       desc:"Human-friendly display label (falls back to name when empty)"`
	Provider    string   `json:"provider,omitempty"    desc:"Provider (binary/sidecar that hosts this task)"`
	URI         string   `json:"uri,omitempty"         desc:"Canonical provider:name identifier"`
	Type        string   `json:"type"                  desc:"Machine type (strategy or named type)"`
	Description string   `json:"description,omitempty"`
	Advertised  *bool    `json:"advertised,omitempty"  desc:"List/picker visibility (default true; false hides from gallery but stays callable by name)"`
	Capability  string   `json:"capability,omitempty"  desc:"Task capability label from frontmatter config"`
	Icon        string   `json:"icon,omitempty"        desc:"Emoji icon for terminal display"`
	IconName    string   `json:"iconName,omitempty"    desc:"Tabler icon name for GUI display"`
	Hashtags    []string `json:"hashtags,omitempty"    desc:"Categorization tags"`
	Category    string   `json:"category,omitempty"    desc:"Primary grouping category"`
	Tags        []string `json:"tags,omitempty"        desc:"Behavioral markers (e.g., quick-action)"`
	Kind        string   `json:"kind,omitempty"        desc:"Targeting kind: 'global' (default), 'object' (one CWM object) or 'collection' (all objects of a type)"`
	Types       []string `json:"types,omitempty" desc:"CWM object types this task targets (kind object or collection); * = any"`
	Audience    string   `json:"audience,omitempty"    desc:"Whom the task is for: end-user | technical | demo; empty: undeclared (not end-user)"`
	Filter      string   `json:"filter,omitempty"      desc:"Optional el expression over the object refining applicability beyond Types"`
	SessionMode string   `json:"sessionMode,omitempty" desc:"How a task-declared action surfaces: none (sync inline) | session (async journaled) | chat (interactive tab)"`
}

// TaskInfo describes a full task definition from the registry — config and
// body included. Returned by GetTask; callers that only need metadata should
// use ListTasks and its lighter TaskSummary instead.
type TaskInfo struct {
	Name              string          `json:"name"`
	Title             string          `json:"title,omitempty"       desc:"Human-friendly display label (falls back to name when empty)"`
	Provider          string          `json:"provider,omitempty"    desc:"Provider (binary/sidecar that hosts this task)"`
	URI               string          `json:"uri,omitempty"         desc:"Canonical provider:name identifier"`
	Type              string          `json:"type"                  desc:"Machine type (strategy or named type)"`
	Description       string          `json:"description,omitempty"`
	Advertised        *bool           `json:"advertised,omitempty"  desc:"List/picker visibility (default true; false hides from gallery but stays callable by name)"`
	Config            map[string]any  `json:"config,omitempty"      desc:"Strategy-specific config from frontmatter"`
	Body              string          `json:"body,omitempty"        desc:"Markdown body (prompt for agent-loop)"`
	InputSchema       json.RawMessage `json:"inputSchema,omitempty" desc:"ktw-schema DSL declaring the task's input"`
	OutputSchema      json.RawMessage `json:"outputSchema,omitempty" desc:"ktw-schema DSL declaring the task's output"`
	PermissionProfile string          `json:"permissionProfile,omitempty" desc:"Kernel permission profile declared by the task (delegated children inherit it)"`
	Group             string          `json:"group,omitempty" desc:"The task group its runs are processed in when the caller names none"`
}

// ── RunStatus ───────────────────────────────────────────────────────────

type RunStatus string

const (
	RunStatusPending RunStatus = "pending"
	// RunStatusQueued: submitted, waiting for a slot of its group.
	RunStatusQueued  RunStatus = "queued"
	RunStatusRunning RunStatus = "running"
	// RunStatusWaiting: its machine waits (an input, an event, a child); it
	// holds no slot of its group.
	RunStatusWaiting   RunStatus = "waiting"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
	// RunStatusKilled: stopped and still alive after the grace period; its
	// slot freed, its result ignored.
	RunStatusKilled RunStatus = "killed"
	// RunStatusStopping: a person stopped the run and it has not ended yet (it
	// still holds its slot); then cancelled — or killed past the grace period.
	RunStatusStopping RunStatus = "stopping"
)
