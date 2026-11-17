// SPDX-License-Identifier: Apache-2.0

package presets

// Mode is a named delegation mode — a pre-packaged SpawnConfig.
// See agent-orchestration-architecture.md for the full dimension mapping of each mode.
type Mode string

const (
	ModeWorker       Mode = "worker"
	ModeBackground   Mode = "background"
	ModeFork         Mode = "fork"
	ModeIsolated     Mode = "isolated"
	ModeForkIsolated Mode = "fork-isolated"
	ModeCoordinator  Mode = "coordinator"
	ModeTeamLeader   Mode = "team-leader"
	ModeTeammate     Mode = "teammate"
	ModeRemote       Mode = "remote"
	ModeChat         Mode = "chat"
)

// ── Preset Definitions ───────────────────────────────────────────────
//
// Each preset is the full SpawnConfig for a named mode. The dimension values
// come from the agent-orchestration-architecture.md ADR (Part 2).

// presetWorker: simple sync delegation. The default mode.
var presetWorker = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingForeground},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: false, Messaging: false},
	// ms-cwm-v2-providers US-16 (2026-06-08): worker spawns are
	// headless by design — route ask_user calls to the target
	// node's Inbox (suspend on resolution) instead of silently
	// rejecting them.
	UserComms: UserCommsConfig{Display: DisplaySilent, Input: InputInbox},
	Isolation: IsolationConfig{Mode: IsolationInProcess},
}

// DefaultBackgroundDurationS is the wall-clock budget applied to every
// background-flavored mode (background / fork / isolated / fork-isolated)
// unless the caller overrides via TaskRunRequest.MaxDurationS. 1 hour is
// generous for the kind of async work these modes typically host while
// still preventing the "unbounded async spawn" resource-exhaustion risk
// flagged by the audit (R6 — security.md high-finding-2).
const DefaultBackgroundDurationS = 3600

// presetBackground: async worker — parent continues immediately.
// UserComms Input → Inbox per US-16 (parent doesn't get the question
// directly; it lands in the target node's Inbox where the user picks
// it up later).
var presetBackground = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground, Limits: ResourceLimits{MaxDurationS: DefaultBackgroundDurationS}},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true},
	UserComms:   UserCommsConfig{Display: DisplaySilent, Input: InputInbox},
	Isolation:   IsolationConfig{Mode: IsolationInProcess},
}

// presetFork: inherits parent's full conversation history.
// UserComms Input → Inbox per US-16. Its permissions are a background
// child's: bubbling a tool's approval to the parent is not built, and the
// agent factory refuses a child that declares it (OD-030).
var presetFork = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground, Limits: ResourceLimits{MaxDurationS: DefaultBackgroundDurationS}},
	Context:     ContextConfig{History: HistoryFullFork, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true},
	UserComms:   UserCommsConfig{Display: DisplaySilent, Input: InputInbox},
	Isolation:   IsolationConfig{Mode: IsolationInProcess},
}

// presetIsolated: dedicated git worktree for file isolation.
// UserComms Input → Inbox per US-16.
var presetIsolated = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground, Limits: ResourceLimits{MaxDurationS: DefaultBackgroundDurationS}},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceDedicated, Strategy: "git_worktree", ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true},
	UserComms:   UserCommsConfig{Display: DisplaySilent, Input: InputInbox},
	Isolation:   IsolationConfig{Mode: IsolationInProcess},
}

// presetForkIsolated: fork context + git worktree.
// UserComms Input → Inbox per US-16. Permissions as presetFork's.
var presetForkIsolated = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground, Limits: ResourceLimits{MaxDurationS: DefaultBackgroundDurationS}},
	Context:     ContextConfig{History: HistoryFullFork, Workspace: WorkspaceConfig{Mode: WorkspaceDedicated, Strategy: "git_worktree", ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true},
	UserComms:   UserCommsConfig{Display: DisplaySilent, Input: InputInbox},
	Isolation:   IsolationConfig{Mode: IsolationInProcess},
}

// presetCoordinator: orchestrator. Full tools for self, restricted for spawned workers.
// Carries the CoordinatorRestrictsChildren spawn policy — children get
// SafeToolSubset and the policy is sticky (UC-053 + UC-054).
var presetCoordinator = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingForeground},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true, Messaging: true},
	// UserComms Input → Inbox per US-16: humans-in-the-loop for
	// coordinator / team-leader talk through artifacts + inbox
	// questions, not direct HumanGate prompts. Override to
	// InputDirect explicitly when the operator wants the orchestrator
	// to ask the user interactively (rare; usually coordinator runs
	// detached).
	UserComms:     UserCommsConfig{Display: DisplayStreaming, Input: InputInbox},
	Isolation:     IsolationConfig{Mode: IsolationInProcess},
	SpawnPolicies: []SpawnPolicy{CoordinatorRestrictsChildrenPolicy{}},
}

// presetTeamLeader: creates a named team, spawns teammates, mailbox messaging.
// Carries the TeamLeaderTagsChildren spawn policy — children get messaging
// enabled at spawn time (NOT sticky).
var presetTeamLeader = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingForeground},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true, Messaging: true},
	// UserComms Input → Inbox per US-16: humans-in-the-loop for
	// coordinator / team-leader talk through artifacts + inbox
	// questions, not direct HumanGate prompts. Override to
	// InputDirect explicitly when the operator wants the orchestrator
	// to ask the user interactively (rare; usually coordinator runs
	// detached).
	UserComms:     UserCommsConfig{Display: DisplayStreaming, Input: InputInbox},
	Isolation:     IsolationConfig{Mode: IsolationInProcess},
	SpawnPolicies: []SpawnPolicy{TeamLeaderTagsChildrenPolicy{}},
}

// presetTeammate: member of a team. Spawned by a team-leader.
// Scheduling carries the standard background duration limit like every
// other background preset — its absence silently broke EVERY spawn-teammate
// once the bg manager started rejecting unbounded background spawns.
var presetTeammate = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground, Limits: ResourceLimits{MaxDurationS: DefaultBackgroundDurationS}},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true, Messaging: true},
	UserComms:   UserCommsConfig{Display: DisplaySilent, Input: InputViaLeader},
	Isolation:   IsolationConfig{Mode: IsolationContextIsolated},
}

// presetChat: interactive chat session — the user drives the loop.
// Foreground (the session lives exactly as long as the chat tab),
// UserComms `direct` (HumanGate prompts render inline in the chat),
// full tools, shared workspace, notifications on (the user wants to
// see child completions surface in the conversation).
//
// The chat loop behaviors all derive from Finish=FinishDriven (NOT from the
// mode name): the `complete` tool is filtered from the catalog, the keepalive
// OnDecision nudge is auto-attached, and Idle-declared tools (ask-user) are
// exempt from the loop-safety repetition guard. Every OTHER preset leaves
// Finish empty (== FinishSelf): they expose `complete` and end themselves.
var presetChat = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingForeground},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceShared, ReadPaths: []string{"*"}, WritePaths: []string{"*"}}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true, Messaging: false},
	UserComms:   UserCommsConfig{Display: DisplayStreaming, Input: InputDirect},
	Isolation:   IsolationConfig{Mode: IsolationInProcess},
	Finish:      FinishDriven,
}

// presetRemote: stub for cloud execution. Not yet implemented.
var presetRemote = SpawnConfig{
	Scheduling:  SchedulingConfig{Mode: SchedulingBackground},
	Context:     ContextConfig{History: HistoryNone, Workspace: WorkspaceConfig{Mode: WorkspaceRemote}},
	Permissions: PermissionsConfig{Tools: []string{"*"}, PermissionMode: PermissionDefault},
	AgentComms:  AgentCommsConfig{Notifications: true},
	UserComms:   UserCommsConfig{Display: DisplayOnCompletion, Input: InputNone},
	Isolation:   IsolationConfig{Mode: IsolationRemote},
}

// presetsByMode maps mode name → preset SpawnConfig.
var presetsByMode = map[Mode]SpawnConfig{
	ModeWorker:       presetWorker,
	ModeBackground:   presetBackground,
	ModeFork:         presetFork,
	ModeIsolated:     presetIsolated,
	ModeForkIsolated: presetForkIsolated,
	ModeCoordinator:  presetCoordinator,
	ModeTeamLeader:   presetTeamLeader,
	ModeTeammate:     presetTeammate,
	ModeRemote:       presetRemote,
	ModeChat:         presetChat,
}

// KnownModes returns all registered mode names.
func KnownModes() []Mode {
	modes := make([]Mode, 0, len(presetsByMode))
	for m := range presetsByMode {
		modes = append(modes, m)
	}
	return modes
}

// IsKnownMode returns true if the given mode has a registered preset.
func IsKnownMode(mode Mode) bool {
	_, ok := presetsByMode[mode]
	return ok
}
