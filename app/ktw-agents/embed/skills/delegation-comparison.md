---
name: delegation-comparison
description: >-
  Full 6-dimension comparison matrix for all 9 delegation modes in
  ktw-agents. Breaks down Scheduling, Context (history + workspace),
  Permissions, AgentComms, UserComms, and Isolation values for worker,
  background, fork, isolated, fork-isolated, coordinator, team-leader,
  teammate, and remote. Includes cost/weight comparison and override
  compatibility. Load this skill alongside `delegation-modes` when you need
  the exhaustive dimension-level view of a preset.
iconName: table
icon: "📊"
hashtags: [delegation, modes, comparison, matrix, dimensions]
category: orchestration
advertised: false
---

# Mode Comparison Matrix

Full 6-dimension breakdown of all 9 modes. Source of truth:
`internal/extensions/task/api/engine/presets/modes.go`.

## At a glance

| Mode | Scheduling | History | Workspace | Permissions | AgentComms | UserComms | Isolation |
|------|-----------|---------|-----------|-------------|------------|-----------|-----------|
| `worker` | foreground | none | shared (`*`) | inherits, default | direct | parent stream | in-process |
| `background` | background | none | shared (`*`) | inherits, default | direct + notify | parent stream | in-process |
| `fork` | background | **full_fork** | shared (`*`) | **bubble** | direct | parent stream | in-process |
| `isolated` | background | none | **dedicated (worktree)** | inherits, default | direct | parent stream | in-process |
| `fork-isolated` | background | **full_fork** | **dedicated (worktree)** | **bubble** | direct | parent stream | in-process |
| `coordinator` | foreground | none | shared (`*`) | self: inherits / **workers: safe-subset auto** | mailbox + delegate | parent stream | in-process |
| `team-leader` | foreground | none | shared (`*`) | inherits | mailbox + spawn-teammate + group | parent stream | in-process |
| `teammate` | background | none | shared (`*`) | inherits | mailbox + group (auto-subscribed) | parent stream | **context_isolated** (independent abort) |
| `remote` | background | none | (TBD) | (planned) | KTSP | (TBD) | **subprocess / cross-host** |

## Dimension definitions

### 1. Scheduling
- `foreground` — parent blocks until child exits
- `background` — child runs in goroutine; parent receives task ID immediately

### 2. Context — History
- `none` — child starts fresh (just system prompt + initial input)
- `full_fork` — parent's full conversation copied as InitialMessages
- `prompt_only` — only the system prompt, no conversation
- `summary` — summarized history (NOT IMPLEMENTED yet)

### 2b. Context — Workspace
- `shared` — child uses parent's workspace dir
- `dedicated` — child gets its own workspace; `Strategy: "git_worktree"` creates a new branch
- `ReadPaths` / `WritePaths` — path-level access (default `*` = full); enforced by WorkspacePathHook

### 3. Permissions
- `inherits` — child uses parent's permission profile
- `bubble` — child's permission requests bubble back to parent for approval
- `restricted` — child's `Tools` allow-list is enforced (defaults to `SafeToolSubset` for coordinator-spawned workers)
- `default` — uses the user-curated permission profile gate as-is

### 4. AgentComms
- `direct` — agents talk via `to.<name>.message` topics through the mailbox
- `mailbox + group` — additionally subscribed to `group.<team>.message` broadcasts
- `delegate` — has the `delegate` tool to spawn workers
- `spawn-teammate` — has the `spawn-teammate` tool (team-leader only)
- `notify` — completion event pushed to parent's notification queue (background mode)

### 5. UserComms
All current modes stream to the parent's terminal (no separate user-facing UI per child yet).

### 6. Isolation
- `in_process` — runs as a goroutine in the parent's process; cancel cascades from parent
- `context_isolated` — goroutine in same process, but child's abort signal is independent of parent (used by teammate so team-leader cancel doesn't immediately kill teammates)
- `subprocess` — separate OS process (planned for remote mode)

## Cost / weight comparison

| Mode | Cost | Weight | Common use case |
|------|------|--------|------------------|
| `worker` | low | low | simple sub-task, parent waits |
| `background` | low | low | parallel fan-out |
| `fork` | **high** (full history copy) | medium | "second opinion" with same context |
| `isolated` | low | medium (worktree creation) | safe refactor / experiment |
| `fork-isolated` | **highest** | high | full A/B exploration |
| `coordinator` | medium (long-running) | medium | orchestrate + synthesize |
| `team-leader` | medium (long-running) | medium | coordination with messaging |
| `teammate` | low | low | team member |
| `remote` | (TBD) | (highest, network) | cross-host work |

## Override compatibility

The `delegate(...)` tool accepts dimension-level overrides. The override
**merges** non-zero fields onto the preset:

| Override field | Effect |
|----------------|--------|
| `max_turns: N` | tightens Scheduling.MaxTurns |
| `max_budget_usd: X` | tightens Scheduling.MaxBudgetUSD |
| `permission_mode: "restricted"` | switches to restricted Permissions |
| `allowed_tools: [...]` | sets Permissions.Tools allow-list |
| `disallowed_tools: [...]` | sets Permissions.Tools deny-list |
| `workspace_paths: { read: [...], write: [...] }` | tightens path access |
| `history_mode: "..."` | overrides Context.History |
| `isolation_mode: "..."` | overrides Isolation |

For major behavior changes (e.g., adding history to a worker), pick the
right base mode (`fork`) instead of overriding — the prompt template differs
across modes and overrides don't switch the prompt.
