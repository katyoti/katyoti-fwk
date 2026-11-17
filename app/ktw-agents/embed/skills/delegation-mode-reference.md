---
name: delegation-mode-reference
description: >-
  Per-mode reference for all 9 ktw-agents delegation presets. For each mode
  (worker, background, fork, isolated, fork-isolated, coordinator,
  team-leader, teammate, remote): the 6-dimension breakdown (Scheduling,
  Context, Permissions, AgentComms, UserComms, Isolation), when to use it,
  what to avoid it for, and a minimal `delegate(...)` call example. Load this
  skill alongside `delegation-modes` when you need the exhaustive
  per-mode definitions.
iconName: book
icon: "📒"
hashtags: [delegation, modes, reference, presets, orchestration]
category: orchestration
advertised: false
---

# Per-Mode Reference

For each mode: 6-dimension breakdown, when to use, minimal `delegate(...)`
call. Source: `internal/extensions/task/api/engine/presets/modes.go`,
`prompts.go`.

The 6 dimensions are: **Scheduling**, **Context** (history + workspace),
**Permissions**, **AgentComms**, **UserComms**, **Isolation**.

---

## worker

**Default mode**. Synchronous specialist that completes a self-contained task.

| Dimension | Value |
|-----------|-------|
| Scheduling | foreground (parent blocks) |
| Context | history: none, workspace: shared, paths: `*` (full) |
| Permissions | inherits parent profile |
| AgentComms | direct only (mailbox) |
| UserComms | streams to parent's terminal |
| Isolation | in-process (parent ctx cascades) |

**When**: short, well-defined sub-task (parse a file, write a sentence,
analyze code). Parent waits for the result and continues.

**Avoid for**: long-running work (use `background`), work needing parent's
conversation context (use `fork`), or anything needing tool restrictions
(coordinator workers get this auto).

```jsonc
{ "mode": "worker", "task_name": "ad-hoc", "prompt": "Name 3 colors. Reply with one comma-separated line." }
```

---

## background

Same as worker but runs in a goroutine; parent doesn't block. Parent receives
a notification when done.

| Dimension | Value |
|-----------|-------|
| Scheduling | background (returns task ID immediately) |
| Context | history: none, workspace: shared, paths: `*` |
| Permissions | inherits |
| AgentComms | direct + completion-notification pushed to parent |
| UserComms | streams to parent's terminal |
| Isolation | in-process |

**When**: parallel fan-out (3 workers in parallel), fire-and-forget
sub-tasks where parent will check later via `task-status`.

```jsonc
{ "mode": "background", "task_name": "ad-hoc", "prompt": "Run analysis...", "name": "analyzer-1" }
```

---

## fork

Background mode + child inherits the parent's full conversation history. The
child can "continue thinking" from where the parent was.

| Dimension | Value |
|-----------|-------|
| Scheduling | background |
| Context | **history: full_fork** (parent's conversation copied) |
| Permissions | bubble (child requests permissions back to parent) |
| AgentComms | direct |
| UserComms | streams |
| Isolation | in-process |

**When**: deep investigation that needs all the parent's discoveries; "what
if we approached this differently" exploration; second opinion from the
same context.

**Avoid for**: simple parallelism — fork copies the entire conversation,
which costs LLM context tokens.

```jsonc
{ "mode": "fork", "task_name": "ad-hoc", "prompt": "Reconsider the architecture from scratch." }
```

---

## isolated

Background mode + child runs in its own git worktree (workspace dimension =
dedicated). File changes are isolated from the parent.

| Dimension | Value |
|-----------|-------|
| Scheduling | background |
| Context | history: none, **workspace: dedicated (git_worktree)**, paths: `*` |
| Permissions | inherits |
| AgentComms | direct |
| UserComms | streams |
| Isolation | in-process (file isolation only, not process isolation) |

**When**: child will modify files and you don't want those changes to leak
into the parent's worktree until you've reviewed them. Common for refactor
experiments and "can we do this safely?" tries.

**Worktree lifecycle**: auto-cleaned when child completes if no
uncommitted changes; preserved otherwise for user review.

```jsonc
{ "mode": "isolated", "task_name": "ad-hoc", "prompt": "Refactor the auth module — feel free to break things, you have your own worktree." }
```

---

## fork-isolated

Combines `fork` (history inheritance + bubble permissions) with `isolated`
(git worktree). Heaviest mode.

| Dimension | Value |
|-----------|-------|
| Scheduling | background |
| Context | history: full_fork, workspace: dedicated (git_worktree) |
| Permissions | bubble |
| AgentComms | direct |
| UserComms | streams |
| Isolation | in-process |

**When**: you want a "what if I had taken a different turn at step N" exploration
with the full parent context AND file isolation. Rare but valuable for
multi-path A/B comparisons.

```jsonc
{ "mode": "fork-isolated", "task_name": "ad-hoc", "prompt": "Try implementing this with approach B — your own worktree, full context." }
```

---

## coordinator

Long-running orchestrator. Spawns workers via `delegate`, watches for
results via `check-messages`, synthesizes. **Workers spawned under
coordinator get an automatic safe-tool-subset restriction** (file-read /
file-write / file-edit / glob / complete only — no bash, no orchestration
tools).

| Dimension | Value |
|-----------|-------|
| Scheduling | foreground (long-running) |
| Context | history: none, workspace: shared |
| Permissions | inherits for self; **safe-subset auto-applied to spawned workers** |
| AgentComms | mailbox (reads `to.coordinator.message`) + delegate tool |
| UserComms | streams |
| Isolation | in-process |

**When**: orchestrating a fan-out / collect / synthesize pattern. The
coordinator stays alive while N workers run, polls their status, gathers
outputs.

**Prompt template** (auto-injected by orchestration plugin):
> You are a coordinator. Use `delegate(...)` to spawn workers. Workers are
> restricted to safe file tools. Use `check-messages` to read replies. ...

See the **topic-conventions** skill, P1 (coordinator pattern).

```jsonc
{ "mode": "coordinator", "task_name": "ad-hoc", "prompt": "Coordinate research across 3 sub-domains, then synthesize." }
```

---

## team-leader

Like coordinator but with full team semantics: creates a team via
`create-team`, spawns members via `spawn-teammate` (NOT `delegate`),
teammates can talk to each other via `group.<team>.message` broadcasts
or `to.<other-name>.message` directs.

| Dimension | Value |
|-----------|-------|
| Scheduling | foreground (long-running) |
| Context | history: none, workspace: shared |
| Permissions | inherits (no auto-restriction on teammates, unlike coordinator) |
| AgentComms | mailbox + spawn-teammate + group broadcast |
| UserComms | streams |
| Isolation | in-process |

**When**: agents need to talk to each other during work, not just at the
end. Pair review, debate, multi-perspective deliberation.

```jsonc
{ "mode": "team-leader", "task_name": "ad-hoc", "prompt": "Run a debate between 2 teammates with opposing views." }
```

See the **topic-conventions** skill, P2 (team-leader pattern), and
`app/ktw-agents/embed/tasks/demo-delegate-team-messaging/TASK.md`.

---

## teammate

Spawned by a team-leader's `spawn-teammate` call. **Not used directly via
`delegate`** — the team-leader uses the dedicated tool. Auto-subscribes to
its own mailbox topic + the team's group topic on the first turn.

| Dimension | Value |
|-----------|-------|
| Scheduling | background |
| Context | history: none, workspace: shared |
| Permissions | inherits (team-leader controls) |
| AgentComms | mailbox + group (auto-subscribed) |
| UserComms | streams |
| Isolation | **context_isolated** (independent abort signal — team-leader cancel doesn't immediately kill teammate) |

**When**: only via `spawn-teammate` from a team-leader. Don't set as a top-
level mode unless you really know what you're doing (no team to belong to).

The independent abort isolation lets teammates finish gracefully even when
the team-leader's context cancels — useful for write-back patterns.

```jsonc
// From a team-leader:
{ "tool": "spawn-teammate", "team": "color-team", "name": "alice",
  "task_name": "ad-hoc", "prompt": "You're alice. Pick a color, send-message to bob." }
```

---

## remote

**Deferred / not yet functional.** Spawn the child as a separate process
(possibly on a different host) communicating via KTSP.

| Dimension | Value |
|-----------|-------|
| Scheduling | background |
| Context | history: none, workspace: TBD (likely dedicated) |
| Permissions | (planned) |
| AgentComms | KTSP transport |
| UserComms | (TBD) |
| Isolation | **subprocess / cross-host** |

**When**: heavy/risky workloads on a separate machine, multi-tenant
isolation, hardware-specific tasks (GPU, etc.).

**Status**: preset is registered, prompt template exists, but the runtime
isn't implemented. Tracked under `spawn-behavior-isolation-external` in the
ms08 follow-ups. Don't use yet.
