// SPDX-License-Identifier: Apache-2.0

package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
	"github.com/katyoti/katyoti-fwk/internal/extensions/worktree"
)

// delegate.go — the delegation concern of the task plugin: the
// handleDelegate mode switch, the per-mode delegators (worker / background /
// fork / isolated / fork-isolated / coordinator / team-leader), and the
// spawn-intent audit event. plugin.go keeps tool registration + dispatch.

func (p *taskPlugin) handleDelegate(ctx context.Context, input json.RawMessage) (string, error) {
	if p.taskSvc == nil {
		return "", fmt.Errorf("TaskService not available")
	}
	p.warn = engine.RunWarnerFrom(ctx)
	// Called again after waiting on its child (a machine wait): read the
	// child's outcome — never start a second child.
	if resume, ok := agentapi.AwaitResumeFrom(ctx); ok {
		if child, _ := resume[childKey].(string); child != "" {
			name, _ := resume[childTaskKey].(string)
			return p.childOutcome(ctx, child, name)
		}
	}

	var params struct {
		TaskName string         `json:"task_name"`
		Mode     string         `json:"mode"`
		Input    map[string]any `json:"input"`
		Prompt   string         `json:"prompt"`
		Finish   string         `json:"finish"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	// Default mode
	if params.Mode == "" {
		params.Mode = "worker"
	}

	// Build user_input for the child agent's initial message.
	// When both prompt and structured input exist, combine them so the child
	// sees both the instructions and the data.
	if params.Input == nil {
		params.Input = make(map[string]any)
	}
	if _, ok := params.Input["user_input"]; !ok && params.Prompt != "" {
		// Check if input has structured data beyond user_input
		hasData := false
		for k := range params.Input {
			if k != "user_input" {
				hasData = true
				break
			}
		}
		if hasData {
			// Combine prompt + serialized input data
			data, _ := json.Marshal(params.Input)
			params.Input["user_input"] = params.Prompt + "\n\nInput data:\n" + string(data)
		} else {
			params.Input["user_input"] = params.Prompt
		}
	}

	// I2: emit the structured spawn-intent event for the audit trail. Fires
	// before the per-mode dispatch so the event captures the parent's
	// decision context, not the post-spawn lifecycle. publishSpawnEvent
	// is a no-op if no bus is wired (tests, lightweight harnesses).
	p.publishSpawnEvent(ctx, params.TaskName, params.Mode, params.Input)

	switch params.Mode {
	case "fork", "fork-isolated":
		return p.delegateFork(params.TaskName, params.Mode, params.Input, params.Prompt)
	case "isolated":
		return p.delegateIsolated(params.TaskName, params.Input)
	case "background":
		return p.delegateBackground(params.TaskName, params.Prompt, params.Input)
	case "worker":
		if params.TaskName == "" && params.Prompt != "" {
			// Prompt-only delegation: create an inline task definition
			return p.delegateSyncWithPrompt(ctx, params.Prompt, params.Input)
		}
		return p.delegateSync(ctx, params.TaskName, params.Input)
	case "coordinator", "team-leader":
		// Coordinator/team-leader modes run as background tasks with their own prompt
		return p.delegateBackground(params.TaskName, params.Prompt, params.Input)
	case "agent_as_human", "agent-as-human":
		// The CALLER chooses agent-as-human here (not the child's definition):
		// the child runs concurrently and routes its ask-user UP to this
		// parent, which answers via answer-child. `finish` decides whether the
		// child completes itself (self — a wizard producing a deliverable) or is
		// steered until the parent stop-childs it (driven — an open chat).
		return p.delegateAgentAsHuman(params.TaskName, params.Input, params.Finish)
	case "remote":
		return "", fmt.Errorf("remote delegation mode not available: planned for a future release")
	default:
		return "", fmt.Errorf("unknown delegation mode: %q", params.Mode)
	}
}

// publishSpawnEvent emits a structured ChildSpawnEvent for the audit trail
// (I2). Captures parent + child + mode + override map + sticky-policy
// names without revealing message content. Best-effort: a nil bus or
// missing application context skips emission with no error.
func (p *taskPlugin) publishSpawnEvent(ctx context.Context, taskName, mode string, input map[string]any) {
	bus, _ := application.UseService[engine.EventBus](ctx)
	if bus == nil {
		return
	}
	override := presets.BuildChildOverride(presets.Mode(mode), presets.Mode(p.parentMode()))
	var policyNames []string
	if names, ok := override[presets.OverrideKeySpawnPolicies].([]any); ok {
		for _, n := range names {
			if s, ok := n.(string); ok {
				policyNames = append(policyNames, s)
			}
		}
	}
	evt := &engine.ChildSpawnEvent{
		ParentRunID:   p.parentRunID(),
		ChildName:     taskName,
		Mode:          mode,
		Override:      override,
		SpawnPolicies: policyNames,
		// Input feeds the session task-tree view ("what ran, with which
		// input") — safe to journal because redaction scrubs secrets at
		// the write chokepoint.
		Input: input,
	}
	evt.Correlation = engine.Correlation{ParentRunID: p.parentRunID()}
	bus.Publish(engine.ChildSpawnTopic(), evt)
}

func (p *taskPlugin) delegateFork(taskName, mode string, taskInput map[string]any, prompt string) (string, error) {
	fm := NewForkManager()

	// Recursion guard: block re-fork from within a fork child
	if p.loop != nil && fm.IsInForkChild(p.loop.Messages()) {
		return "", fmt.Errorf("cannot fork from within a fork child — use explicit delegation instead")
	}

	// Build forked messages from parent conversation
	var parentMessages []agentapi.Message
	if p.loop != nil {
		parentMessages = p.loop.Messages()
	}
	childPrompt := prompt
	if childPrompt == "" {
		childPrompt = "Complete the delegated work."
	}
	forkedMessages := fm.BuildForkedMessages(parentMessages, childPrompt)

	// Create inline task definition for the fork agent
	// Inherit parent's plugins for prompt-based delegation
	var parentPlugins []any
	if p.loop != nil {
		parentPlugins = p.loop.InheritedPlugins()
	}
	taskDef := fm.BuildForkTaskDef("", childPrompt, forkedMessages, parentPlugins)

	parentID := p.parentRunID()
	maxDurS, fireForget := presets.SchedulingFieldsFromMode(presets.Mode(mode))
	request := taskapi.TaskRunRequest{
		TaskDef:           taskDef,
		Input:             taskInput,
		ParentRunID:       parentID,
		ConfigOverride:    presets.BuildChildOverride(presets.Mode(mode), presets.Mode(p.parentMode())),
		MaxDurationS:      maxDurS,
		FireAndForget:     fireForget,
		PermissionProfile: p.permissionProfile(),
	}

	// fork-isolated: also create a worktree. Slug is hash-derived; no LLM
	// input flows into it (R8 / audit high-finding-4). The fork label is just
	// for UI/debug — preserved in WorktreeState.DisplayName.
	var wtSlug string
	if mode == "fork-isolated" && p.wtMgr != nil {
		wtState, err := p.wtMgr.CreateWorktree(worktree.KindFork, "fork-context", "")
		if err != nil {
			return "", fmt.Errorf("create worktree for fork: %w", err)
		}
		wtSlug = wtState.Slug
		request.WorkspaceRoot = wtState.Path
	}

	bgMgr := p.taskSvc.BackgroundManager()
	taskID, err := bgMgr.Register(parentID, request)
	if err != nil {
		return "", fmt.Errorf("fork delegate: %w", err)
	}
	// Schedule worktree cleanup once the child terminates. Preserves worktree
	// on uncommitted changes (not forced) so user can review diffs.
	if wtSlug != "" {
		scheduleWorktreeCleanup(bgMgr, p.wtMgr, taskID, wtSlug, p.warn)
	}
	return fmt.Sprintf("Fork started (task_id: %s). The forked agent has your full conversation context. Use task_status to check progress.", taskID), nil
}

func (p *taskPlugin) delegateIsolated(taskName string, taskInput map[string]any) (string, error) {
	if p.wtMgr == nil {
		return "", fmt.Errorf("worktree isolation not available (not in a git repository)")
	}
	// Slug is hash-derived inside CreateWorktree — the LLM-supplied taskName
	// flows ONLY into WorktreeState.DisplayName, never into a filesystem path
	// or git ref (R8 / audit high-finding-4).
	wtState, err := p.wtMgr.CreateWorktree(worktree.KindIsolated, taskName, "")
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	parentID := p.parentRunID()
	maxDurS, fireForget := presets.SchedulingFieldsFromMode(presets.ModeIsolated)
	request := taskapi.TaskRunRequest{
		TaskName:          taskName,
		Input:             taskInput,
		WorkspaceRoot:     wtState.Path,
		ParentRunID:       parentID,
		ConfigOverride:    presets.BuildChildOverride(presets.ModeIsolated, presets.Mode(p.parentMode())),
		MaxDurationS:      maxDurS,
		FireAndForget:     fireForget,
		PermissionProfile: p.permissionProfile(),
	}
	bgMgr := p.taskSvc.BackgroundManager()
	taskID, err := bgMgr.Register(parentID, request)
	if err != nil {
		return "", fmt.Errorf("isolated delegate to %q: %w", taskName, err)
	}
	// Auto-cleanup worktree when the child terminates.
	scheduleWorktreeCleanup(bgMgr, p.wtMgr, taskID, wtState.Slug, p.warn)
	return fmt.Sprintf("Isolated task started: %s (task_id: %s, worktree: %s). Use task_status to check progress.", taskName, taskID, wtState.Path), nil
}

func (p *taskPlugin) delegateBackground(taskName, prompt string, taskInput map[string]any) (string, error) {
	if taskName == "" && prompt == "" {
		return "", fmt.Errorf("background delegate needs a task_name or a prompt")
	}
	parentID := p.parentRunID()
	maxDurS, fireForget := presets.SchedulingFieldsFromMode(presets.ModeBackground)
	request := taskapi.TaskRunRequest{
		Input:             taskInput,
		ParentRunID:       parentID,
		ConfigOverride:    presets.BuildChildOverride(presets.ModeBackground, presets.Mode(p.parentMode())),
		MaxDurationS:      maxDurS,
		FireAndForget:     fireForget,
		PermissionProfile: p.permissionProfile(),
	}
	// Prompt-only delegation: with no registered task_name, run an inline task
	// definition built from the prompt — the same free-form path worker/fork
	// modes support, so `mode: background` (and coordinator/team-leader) can
	// spawn ad-hoc concurrent workers without a pre-registered TASK.md.
	label := taskName
	if taskName == "" && prompt != "" {
		request.TaskDef = p.inlinePromptTaskDef(prompt)
		label = "prompt-worker"
	} else {
		request.TaskName = taskName
	}
	bgMgr := p.taskSvc.BackgroundManager()
	taskID, err := bgMgr.Register(parentID, request)
	if err != nil {
		return "", fmt.Errorf("background delegate to %q: %w", label, err)
	}
	return fmt.Sprintf("Background task started: %s (task_id: %s). Use task_status to check progress.", label, taskID), nil
}

// inlinePromptTaskDef builds an inline `type: agent` task definition from a
// free-form prompt, inheriting the parent's plugins (or `augmented` as a
// fallback). Shared by the synchronous (delegateSyncWithPrompt) and async
// (delegateBackground) prompt-only delegation paths.
func (p *taskPlugin) inlinePromptTaskDef(prompt string) map[string]any {
	var parentPlugins []any
	if p.loop != nil {
		parentPlugins = p.loop.InheritedPlugins()
	}
	if parentPlugins == nil {
		parentPlugins = []any{"augmented"}
	}
	return map[string]any{
		"name":                      "prompt-worker",
		"type":                      "agent",
		"description":               "Prompt-based worker agent",
		presets.ConfigKeyCapability: "smart-best",
		"plugins":                   parentPlugins,
		"body":                      prompt,
	}
}

// delegateAgentAsHuman spawns the child CONCURRENTLY and forces its UserComms
// channel to `via_agent` — its ask-user calls route UP to this parent (which
// answers them via the answer-child tool). The com mode is decided HERE by the
// caller, not in the child's task definition; the child stays a plain task.
func (p *taskPlugin) delegateAgentAsHuman(taskName string, taskInput map[string]any, finish string) (string, error) {
	parentID := p.parentRunID()
	maxDurS, _ := presets.SchedulingFieldsFromMode(presets.ModeBackground)
	override := presets.BuildChildOverride(presets.ModeBackground, presets.Mode(p.parentMode()))
	// Force the agent-as-human channel (the whole point of this mode). Use the
	// CANONICAL InputMode value so the rest of the system (presets, the
	// user-comms adapter) recognizes it; the ask-user dispatcher routes
	// bubble_to_parent to the bubble channel.
	override[presets.OverrideKeyUserCommsInput] = string(presets.InputBubbleToParent)
	// Finish: default SELF — a delegated wizard PRODUCES a deliverable then
	// completes itself (so the parent's await-child gets `completed`). Set
	// `driven` only for an open-ended conversation the parent steers + stops.
	// Setting it explicitly here pins the child's terminator regardless of the
	// child task's own mode (so a `mode: chat` wizard is driven to completion).
	if finish == "" {
		finish = string(presets.FinishSelf)
	}
	override[presets.OverrideKeyFinish] = finish
	request := taskapi.TaskRunRequest{
		TaskName:          taskName,
		Input:             taskInput,
		ParentRunID:       parentID,
		ConfigOverride:    override,
		MaxDurationS:      maxDurS,
		PermissionProfile: p.permissionProfile(),
		// NOT fire-and-forget: the parent needs the child's completion (and
		// must stay alive to answer its questions).
	}
	taskID, err := p.taskSvc.BackgroundManager().Register(parentID, request)
	if err != nil {
		return "", fmt.Errorf("agent_as_human delegate to %q: %w", taskName, err)
	}
	return fmt.Sprintf(
		"Started %q (task_id: %s) in AGENT-AS-HUMAN mode — it runs concurrently "+
			"and routes its ask-user questions to YOU. Each question appears as a "+
			"system line with a question_id; answer it by calling answer-child "+
			"with that question_id. Use task-status(%s) to check progress and keep "+
			"taking turns until it completes. To inspect or SCORE the child's run, "+
			"read its OWN session: task-status(%s) returns the child's `sessionId` "+
			"(NOT this task_id) — pass THAT to session-read.",
		taskName, taskID, taskID, taskID), nil
}

// delegateSyncWithPrompt creates an inline task from a prompt and runs it synchronously.
// Used when the delegate tool receives a prompt but no task_name — the agent
// starts with zero prior context, only the prompt.
func (p *taskPlugin) delegateSyncWithPrompt(ctx context.Context, prompt string, taskInput map[string]any) (string, error) {
	taskDef := p.inlinePromptTaskDef(prompt)

	if taskInput == nil {
		taskInput = make(map[string]any)
	}
	if _, ok := taskInput["user_input"]; !ok {
		taskInput["user_input"] = prompt
	}

	request := taskapi.TaskRunRequest{
		TaskDef:           taskDef,
		Input:             taskInput,
		ParentRunID:       p.parentRunID(),
		ConfigOverride:    presets.BuildChildOverride(presets.ModeWorker, presets.Mode(p.parentMode())),
		PermissionProfile: p.permissionProfile(),
	}
	runResp, err := p.taskSvc.Run(ctx, request)
	if err != nil {
		return "", fmt.Errorf("prompt delegate: %w", err)
	}
	if agentapi.MachineWaitsFrom(ctx) {
		return "", awaitChild(runResp.RunID, "prompt delegate")
	}
	result, err := p.taskSvc.Result(ctx, taskapi.ResultRequest{RunID: runResp.RunID})
	if err != nil {
		return "", fmt.Errorf("wait for prompt delegate: %w", err)
	}
	return delegateOutcome("prompt delegate", result), nil
}

func (p *taskPlugin) delegateSync(ctx context.Context, taskName string, taskInput map[string]any) (string, error) {
	request := taskapi.TaskRunRequest{
		TaskName:          taskName,
		Input:             taskInput,
		ParentRunID:       p.parentRunID(),
		ConfigOverride:    presets.BuildChildOverride(presets.ModeWorker, presets.Mode(p.parentMode())),
		PermissionProfile: p.permissionProfile(),
	}
	runResp, err := p.taskSvc.Run(ctx, request)
	if err != nil {
		return "", fmt.Errorf("delegate to %q: %w", taskName, err)
	}
	// In a loop that waits as a machine, wait for the child as a machine:
	// no slot, no goroutine held, the wait saved.
	if agentapi.MachineWaitsFrom(ctx) {
		return "", awaitChild(runResp.RunID, taskName)
	}
	result, err := p.taskSvc.Result(ctx, taskapi.ResultRequest{RunID: runResp.RunID})
	if err != nil {
		return "", fmt.Errorf("wait for %q: %w", taskName, err)
	}
	return delegateOutcome(taskName, result), nil
}

// Keys of a delegate's await Resume.
const (
	childKey     = "child"
	childTaskKey = "task"
)

// awaitChild is a delegate's machine wait on its child run's end.
func awaitChild(childRunID, taskName string) *agentapi.ErrAwait {
	return &agentapi.ErrAwait{
		Topic:  engine.RunEndedTopic(childRunID),
		Resume: map[string]any{childKey: childRunID, childTaskKey: taskName},
		Reason: fmt.Sprintf("the result of %q (%s)", taskName, childRunID),
	}
}

// childOutcome is a delegate's answer once called again: the child's result
// when it has ended, else another wait on it.
func (p *taskPlugin) childOutcome(ctx context.Context, childRunID, taskName string) (string, error) {
	st, err := p.taskSvc.Status(ctx, taskapi.StatusRequest{RunID: childRunID})
	if err != nil {
		return "", fmt.Errorf("the child run of %q (%s) is gone — it may have ended in a restart: %w", taskName, childRunID, err)
	}
	switch st.Status {
	case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
	default:
		return "", awaitChild(childRunID, taskName)
	}
	result, err := p.taskSvc.Result(ctx, taskapi.ResultRequest{RunID: childRunID})
	if err != nil {
		return "", fmt.Errorf("the result of %q: %w", taskName, err)
	}
	return delegateOutcome(taskName, result), nil
}

// delegateOutcome is what the model reads of a child's result.
func delegateOutcome(taskName string, result taskapi.ResultResponse) string {
	if result.Error != "" {
		return fmt.Sprintf("%q failed: %s", taskName, result.Error)
	}
	if result.Output != nil {
		data, _ := json.Marshal(result.Output)
		return fmt.Sprintf("%q completed: %s", taskName, string(data))
	}
	return fmt.Sprintf("%q completed (status: %s)", taskName, result.Status)
}
