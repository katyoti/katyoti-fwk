---
name: delegation-modes
description: >-
  Pick the right delegation mode for a sub-agent in ktw-agents. Covers all 9
  named modes (worker, background, fork, isolated, fork-isolated, coordinator,
  team-leader, teammate, remote) with their dimension mapping (Scheduling /
  Context / Permissions / AgentComms / UserComms / Isolation), when to use
  each, and minimal `delegate(...)` call examples. Use this skill when crafting
  `delegate` or `spawn-teammate` calls, or when designing the `mode:` field of
  a TASK.md frontmatter.
iconName: route
icon: "🛣️"
hashtags: [delegation, modes, orchestration, spawn, presets]
category: orchestration
---

# Delegation Modes

ktw-agents has **9 named delegation modes** that bundle 6 orchestration
dimensions into reusable presets. The mode you pick determines:

- whether the child runs synchronously or in the background
- what context (history, workspace) it inherits
- what tools it can use
- how it communicates with the parent and siblings
- whether it shares the parent's process / git worktree / VPS

This skill helps you pick the right mode + write the correct `delegate(...)`
call. The exhaustive per-mode reference (with the full 6-dimension config)
is available by getting the instructions of skill
`ktw-agents:/skill/delegation-mode-reference`. The quick pick guide is available by
getting the instructions of skill `ktw-agents:/skill/delegation-decision-tree`.

## The 9 modes at a glance

| Mode | Scheduling | Context | Permissions | Comms | Isolation |
|------|-----------|---------|-------------|-------|-----------|
| `worker` | foreground | none | inherits | direct | in-process |
| `background` | background | none | inherits | direct + notify | in-process |
| `fork` | background | full conversation | bubble | direct | in-process |
| `isolated` | background | none | inherits | direct | git worktree |
| `fork-isolated` | background | full conversation | bubble | direct | git worktree |
| `coordinator` | foreground | none | safe-subset (auto) | mailbox + delegate | in-process |
| `team-leader` | foreground | none | inherits | mailbox + spawn-teammate + group | in-process |
| `teammate` | background | none | inherits | mailbox + group | in-process (independent abort) |
| `remote` | background | none | (planned) | KTSP | subprocess |

Full matrix with dimension values: get the instructions of skill
`ktw-agents:/skill/delegation-comparison`.

## Quick decision tree

```
Will the parent block on this child's result? (synchronous)
  → Yes → worker (simple) or coordinator (orchestrates more)
  → No  → background (just need result later) or teammate (joins a team)

Does the child need the parent's full conversation history?
  → Yes → fork (or fork-isolated if also needs file isolation)
  → No  → other modes (default: history=none for memory efficiency)

Does the child need a separate git worktree (file isolation)?
  → Yes → isolated (or fork-isolated if also needs history)
  → No  → other modes

Is this child going to spawn / coordinate other agents?
  → Yes → coordinator (worker-restricted children) or team-leader (team members)
  → No  → leaf modes (worker, background, fork, isolated)

Does this child need to talk with siblings during work?
  → Yes → teammate (subscribed to mailbox + group topics)
  → No  → worker / background

Should this child run on a different machine?
  → Yes → remote (deferred to ms08-isolation-external task)
  → No  → in-process modes
```

## Anatomy of a `delegate(...)` call

```jsonc
{
  "mode": "background",                 // ← REQUIRED — picks the preset
  "task_name": "ad-hoc",                // or a defined task name
  "prompt": "Summarize this in one line: ...",
  "name": "summarizer",                 // optional logical name (used in topics)
  "input": { "doc": "..." }             // optional input map (validated)
}
```

When `mode` is omitted, the worker preset is used. The mode's preset is
applied via `presets.ResolvePreset(mode, overrides)` — caller-supplied
overrides win over preset defaults.

## Per-mode quick reference

For the full preset (all 6 dimensions, prompt template, common patterns),
get the instructions of skill `ktw-agents:/skill/delegation-mode-reference`. It
includes per-mode sections for: worker, background, fork, isolated,
fork-isolated, coordinator, team-leader, teammate, remote.

## Mode-to-topic mapping (cross-reference)

Different modes enable different topic-based communication. See the
**topic-conventions** skill for the topic family details, but in short:

| Mode | Subscribes to | Publishes to |
|------|---------------|--------------|
| worker / background / fork / isolated / fork-isolated | (none auto) | (none auto) |
| coordinator | (none) | `delegate` tool spawns children, reads `to.coordinator.message` via mailbox |
| team-leader | `to.<self>.message`, `group.<team>.message` | `group.<team>.message` (broadcast), `to.<name>.message` (direct) |
| teammate | `to.<self>.message`, `group.<team>.message` (auto-subscribed at first turn) | same |

The mailbox-EventBus bridge (orchestration plugin) ensures `send-message(to:
X)` and `to.X.message` topic subscriptions are equivalent — pick whichever
surface fits your subscriber.

## Override pattern

A delegating agent can override individual dimensions of the preset:

```jsonc
{
  "mode": "worker",
  "task_name": "ad-hoc",
  "prompt": "...",
  "max_turns": 5,                     // override Scheduling.MaxTurns
  "max_budget_usd": 0.10,             // override Scheduling.MaxBudgetUSD
  "permission_mode": "restricted"     // override Permissions.PermissionMode
}
```

Overrides merge non-zero fields onto the preset; preset values are kept
otherwise. Use this for **tightening** a mode (e.g., a worker with a cost
cap) — for major behavior changes, switch to a different mode.

## Common pitfalls

1. **Defaulting silently to worker** — omitting `mode` works but hides intent.
   Always set `mode:` explicitly so future readers know.
2. **Coordinator without a tool restriction in mind** — coordinator mode
   automatically restricts SPAWNED workers to a safe tool subset
   (file-read/write/edit, glob, complete). The coordinator itself keeps full
   tools. Don't over-spawn under coordinator if you need workers with bash
   access — use team-leader instead.
3. **fork-isolated for trivial sub-tasks** — full conversation history + git
   worktree is heavy. Reserve for genuinely independent investigations.
4. **teammate without a team** — teammate mode requires the parent to have
   created a team first via `create-team`. Spawning a teammate without a
   team gives an unbound agent that can't broadcast.
5. **remote mode** — deferred to `spawn-behavior-isolation-external` task in
   ms08 follow-ups. Currently has the preset but no functional implementation.

## Cross-references

- Skill **topic-conventions**: which topics each mode enables (mailbox,
  group broadcast, protocols)
- Decision `modules/tasks/decisions/OD-037-how-does-a-caller-say-how-a-child-runs-beside.cwm.md`:
  the 6 dimensions + design rationale for the mode catalog
- Pattern `modules/tasks/delegation.md`: how presets
  compose dimensions
- Source: `internal/extensions/task/api/engine/presets/modes.go` (preset
  values), `prompts.go` (per-mode prompt templates injected into the system
  prompt)
