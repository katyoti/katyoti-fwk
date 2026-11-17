// SPDX-License-Identifier: Apache-2.0

// Package presets defines the 6-dimension spawn configuration and the named
// delegation mode presets that compile down to SpawnConfig values.
//
// See modules/agent/orchestration/concepts.md and the delegation-modes skill
// for the full dimension/mode specification.
package presets

// SpawnConfig is the complete spawn configuration across all 6 orchestration
// dimensions. It is produced by ResolvePreset(mode, overrides) and consumed
// by the delegate tool, workflow spawn nodes, and the Spawner.
//
// Note: this is the type definition. Runtime behavior (git worktree creation,
// pane spawning, remote execution, UI streaming, etc.) is implemented by
// separate subsystems — see the spawn-behavior-* todos in ms08.
type SpawnConfig struct {
	Scheduling  SchedulingConfig  `json:"scheduling"`
	Context     ContextConfig     `json:"context"`
	Permissions PermissionsConfig `json:"permissions"`
	AgentComms  AgentCommsConfig  `json:"agent_communication"`
	UserComms   UserCommsConfig   `json:"user_communication"`
	Isolation   IsolationConfig   `json:"isolation"`

	// Finish declares HOW the run terminates — the loop-lifecycle dimension,
	// orthogonal to where ask_user goes (UserComms) and to scheduling. Empty ==
	// FinishSelf. See FinishMode.
	Finish FinishMode `json:"finish,omitempty"`

	// SpawnPolicies are rules applied AT spawn time to children of agents
	// configured with this SpawnConfig. Sticky policies append themselves
	// here on Apply so they propagate transitively to grandchildren (UC-054).
	// Not JSON-serialized — implementations are typed Go values.
	SpawnPolicies []SpawnPolicy `json:"-"`
}

// FinishMode is the loop-lifecycle dimension: HOW an agent run ends.
//
//   - FinishSelf (default): the run finishes ITSELF. The `complete` tool is
//     exposed and the loop ends when the agent calls it OR produces a final
//     tool-less turn. No keepalive. Task/worker children that PRODUCE a result
//     — including an agent_as_human child that's really a task (it may still
//     bubble questions to its parent, but it completes on its own).
//   - FinishDriven: the run NEVER self-completes. `complete` is hidden and a
//     keepalive nudge keeps it going; it ends only when an EXTERNAL driver stops
//     it — a parent's `stop-child` (hard kill) or a human quitting. Chat
//     children + top-level interactive chat.
//
// This replaces the old `mode == chat` fusion: keepalive + complete-hiding are
// now driven by Finish, so a chat-shaped wizard can be DRIVEN TO COMPLETION as a
// task (finish=self) when the launcher wants a product, not a conversation.
type FinishMode string

const (
	FinishSelf   FinishMode = "self"
	FinishDriven FinishMode = "driven"
)

// ── Dimension 1: Scheduling & Execution ───────────────────────────────

type SchedulingConfig struct {
	// Mode: foreground (sync) | background (async) | fire_and_forget
	Mode SchedulingMode `json:"mode"`

	// AutoBackgroundMs: if > 0, a foreground spawn auto-transitions to background
	// after this many milliseconds. 0 = never auto-background.
	AutoBackgroundMs int `json:"auto_background_ms,omitempty"`

	// FanOutError: strategy when a child in a fan-out fails.
	// continue | fail_fast | retry(N) | escalate
	FanOutError string `json:"fan_out_error,omitempty"`

	// Discovery: how to find the child to spawn.
	// by_name (default) | by_capability
	Discovery string `json:"discovery,omitempty"`

	// Pool: named resource pool for shared budgets. Empty = per-child limits.
	Pool string `json:"pool,omitempty"`

	// Limits: per-child resource budgets.
	Limits ResourceLimits `json:"limits,omitempty"`
}

type SchedulingMode string

const (
	SchedulingForeground    SchedulingMode = "foreground"
	SchedulingBackground    SchedulingMode = "background"
	SchedulingFireAndForget SchedulingMode = "fire_and_forget"
)

type ResourceLimits struct {
	MaxTokens    int     `json:"max_tokens,omitempty"`     // 0 = inherit parent's remaining
	MaxDurationS int     `json:"max_duration_s,omitempty"` // 0 = unlimited
	MaxToolCalls int     `json:"max_tool_calls,omitempty"` // 0 = unlimited
	MaxBudgetUSD float64 `json:"max_budget_usd,omitempty"` // 0 = unlimited
}

// ── Dimension 2: Context & State Sharing ──────────────────────────────

type ContextConfig struct {
	// History: how parent conversation flows to child.
	// none | prompt_only | full_fork | summary
	History HistoryMode `json:"history"`

	// Workspace: filesystem access model.
	Workspace WorkspaceConfig `json:"workspace"`

	// Memory: persistent memory scope.
	// none | inherit | user | project | local | team
	Memory string `json:"memory,omitempty"`

	// Model: LLM model override. empty = inherit parent's model.
	Model string `json:"model,omitempty"`

	// SystemPrompt: override the default system prompt.
	// default | inherit | <explicit string>
	SystemPrompt string `json:"system_prompt,omitempty"`
}

type HistoryMode string

const (
	HistoryNone       HistoryMode = "none"
	HistoryPromptOnly HistoryMode = "prompt_only"
	HistoryFullFork   HistoryMode = "full_fork"
	HistorySummary    HistoryMode = "summary"
)

type WorkspaceConfig struct {
	// Mode: shared (default) | dedicated | remote
	Mode WorkspaceMode `json:"mode"`

	// Strategy: for dedicated mode. directory | git_worktree
	Strategy string `json:"strategy,omitempty"`

	// BaseDir: for dedicated mode. e.g., "work/research/"
	BaseDir string `json:"base_dir,omitempty"`

	// OutputDir: where the child puts deliverables (coordinator reads from here).
	OutputDir string `json:"output_dir,omitempty"`

	// Branch: for git_worktree strategy. e.g., "feature/module-1"
	Branch string `json:"branch,omitempty"`

	// ReadPaths: directories/files the child can read. ["*"] = everything.
	ReadPaths []string `json:"read_paths,omitempty"`

	// WritePaths: directories/files the child can write. ["*"] = everything.
	WritePaths []string `json:"write_paths,omitempty"`
}

type WorkspaceMode string

const (
	WorkspaceShared    WorkspaceMode = "shared"
	WorkspaceDedicated WorkspaceMode = "dedicated"
	WorkspaceRemote    WorkspaceMode = "remote"
)

// ── Dimension 3: Permissions & Tool Access ────────────────────────────

type PermissionsConfig struct {
	// Tools: allow list. ["*"] = all (minus DisallowedTools).
	Tools []string `json:"tools,omitempty"`

	// DisallowedTools: explicit deny list.
	DisallowedTools []string `json:"disallowed_tools,omitempty"`

	// PermissionMode: how tool approval flows to parent.
	// default | bubble | restricted
	PermissionMode PermissionMode `json:"permission_mode,omitempty"`

	// PlanMode: if true, child must get plan approval before executing.
	PlanMode bool `json:"plan_mode,omitempty"`
}

type PermissionMode string

const (
	PermissionDefault    PermissionMode = "default"    // child uses own permissions
	PermissionBubble     PermissionMode = "bubble"     // bubble to parent for restricted tools
	PermissionRestricted PermissionMode = "restricted" // safe subset only
)

// ── Dimension 4: Agent-to-Agent Communication ────────────────────────

type AgentCommsConfig struct {
	// Notifications: deliver completion/failure notifications to parent.
	Notifications bool `json:"notifications"`

	// Messaging: enable mailbox messaging (send-message / check-messages).
	Messaging bool `json:"messaging"`

	// Progress: emit periodic progress events (child.<name>.progress).
	Progress bool `json:"progress"`
}

// ── Dimension 5: Agent-to-User Communication ──────────────────────────

type UserCommsConfig struct {
	// Display: how the child's output is shown to the user.
	// silent | on_completion | streaming | own_pane | progress_only
	Display DisplayMode `json:"display"`

	// Input: how the child can request user input. Five values:
	//   - none             — no user channel; ask_user fails fast
	//   - direct           — synchronous HumanGate (user-attended chat)
	//   - inbox            — async: write approval-request item to the
	//                        target node's Inbox; suspend the task on
	//                        the item-resolved event. Replaces the
	//                        previous "bubble to a non-existent
	//                        channel" behaviour for headless spawns.
	//   - bubble_to_parent — route the prompt through the parent task's
	//                        own UserComms channel
	//   - via_leader       — route through the team leader's channel
	//                        (teammate mode)
	Input InputMode `json:"input"`
}

type DisplayMode string

const (
	DisplaySilent       DisplayMode = "silent"
	DisplayOnCompletion DisplayMode = "on_completion"
	DisplayStreaming    DisplayMode = "streaming"
	DisplayOwnPane      DisplayMode = "own_pane"
	DisplayProgressOnly DisplayMode = "progress_only"
)

type InputMode string

const (
	InputNone           InputMode = "none"
	InputBubbleToParent InputMode = "bubble_to_parent"
	InputDirect         InputMode = "direct"
	InputViaLeader      InputMode = "via_leader"
	// InputInbox routes user-input requests to the target node's
	// Inbox (an `approval-request` item) and suspends the task on the
	// item-resolved event. Default for every system-spawned preset
	// (worker / background /
	// fork / isolated / fork-isolated / coordinator / team-leader) —
	// those runs have no user-attached chat by design.
	InputInbox InputMode = "inbox"
)

// EffectiveUserCommsInput returns the channel a task should use for
// `ask_user` calls, given its resolved spawn mode's preset default.
// Encodes the auto-detection rule:
//
//   - a non-empty `explicitOverride` (an operator who set
//     `user_comms_input:` in TASK.md frontmatter) wins over everything;
//   - otherwise the resolved preset default applies: `chat` → InputDirect
//     (user is sitting in the chat tab right now; HumanGate prompts
//     work), system-spawned modes → InputInbox, teammate →
//     InputViaLeader, remote → InputNone.
func EffectiveUserCommsInput(presetDefault InputMode, explicitOverride InputMode) InputMode {
	if explicitOverride != "" {
		return explicitOverride
	}
	return presetDefault
}

// EffectiveFinish resolves the loop-termination mode: an explicit launch-time
// override (delegate `finish:` / TASK.md) wins; else the resolved preset default
// (chat → driven, everything else → self); empty defaults to FinishSelf.
func EffectiveFinish(presetDefault FinishMode, explicitOverride FinishMode) FinishMode {
	if explicitOverride != "" {
		return explicitOverride
	}
	if presetDefault != "" {
		return presetDefault
	}
	return FinishSelf
}

// ── Dimension 6: Isolation ────────────────────────────────────────────

type IsolationConfig struct {
	// Mode: process/network boundary for the child.
	// in_process | context_isolated | pane | remote
	Mode IsolationMode `json:"mode"`
}

type IsolationMode string

const (
	IsolationInProcess       IsolationMode = "in_process"
	IsolationContextIsolated IsolationMode = "context_isolated"
	IsolationPane            IsolationMode = "pane"
	IsolationRemote          IsolationMode = "remote"
)
