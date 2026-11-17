---
name: delegation-decision-tree
description: >-
  Question-by-question decision tree for picking a ktw-agents delegation mode.
  Walks through synchronous vs async, foreground orchestration depth,
  background context inheritance, file isolation needs, and team participation
  to land on one of the 9 named modes. Includes sanity-check examples and a
  quick lookup table. Load this skill alongside `delegation-modes` when you
  want a guided path rather than a full reference.
iconName: git-branch
icon: "🌲"
hashtags: [delegation, modes, decision-tree, picking, orchestration]
category: orchestration
advertised: false
---

# Mode Decision Tree

A walkthrough of questions to ask when picking a mode.

## Q1 — Synchronous or async?

Will the parent agent **block** waiting for this child's result before
continuing its own work?

- **YES, block** → continue to Q2 (foreground modes)
- **NO, fire-and-forget or parallel** → continue to Q3 (background modes)

## Q2 — Foreground modes (parent blocks)

How much orchestration does the child do itself?

- **Just a leaf task** (parse / write / classify / decide) → **`worker`**
- **Spawns 1+ workers and synthesizes** → **`coordinator`**
  (workers get auto safe-tool-subset)
- **Manages a team that talks among themselves** → **`team-leader`**
  (members spawned via `spawn-teammate`, can group-broadcast)

## Q3 — Background modes (parent doesn't block)

Does the child need to inherit the parent's full conversation history?

- **YES, full context** → continue to Q4 (fork variants)
- **NO, fresh context** → continue to Q5 (background / isolated / teammate)

## Q4 — Fork variants (history inheritance)

Does the child also need a separate git worktree (file isolation)?

- **YES, isolated files too** → **`fork-isolated`** (heaviest mode)
- **NO, just shared workspace** → **`fork`**

## Q5 — Non-fork background modes

Does the child need a separate git worktree?

- **YES, file isolation** → **`isolated`**
- **NO, shared workspace** → continue

Will the child join an existing team and message siblings?

- **YES, team member** → **`teammate`** (only via `spawn-teammate` from a
  team-leader)
- **NO, standalone background work** → **`background`**

Will the child run on a different machine?

- **YES, remote host** → **`remote`** (deferred; not functional yet)
- **NO, in-process** → use one of the modes above

## Sanity-check examples

### "I need to parse 3 docs in parallel and aggregate"
- Synchronous? Could be either. If parent waits → coordinator. If parent
  does other work → spawn 3 background workers + check status later.
- **Pick**: `coordinator` if you want the orchestration logic in the LLM
  (recommended for richer synthesis), or 3 × `background` if you want the
  parent to do the synthesis itself.

### "I need a teammate that critiques the picker's choice"
- Multiple agents talk to each other → team setup.
- **Pick**: parent runs as `team-leader`, spawns `picker` and `critic` via
  `spawn-teammate` (both `teammate` mode).

### "I want to refactor a module in isolation; revert if it breaks"
- File isolation needed.
- **Pick**: `isolated`. Worktree auto-cleans on success; preserved on
  failure for review.

### "Try implementing this with approach B while I keep going with A"
- Need full context (so child knows what 'A' is) + file isolation.
- **Pick**: `fork-isolated`. Heavy but correct.

### "Spawn a worker that just runs a calculation, then exits"
- Simplest case.
- **Pick**: `worker` (or `background` if you don't want to wait).

## Quick lookup table

| Want to … | Pick |
|-----------|------|
| Spawn a worker, wait for result | `worker` |
| Spawn parallel workers and synthesize | `coordinator` (or N × `background`) |
| Run a debate / pair-review with messages | `team-leader` + N × `teammate` |
| Inherit my full conversation | `fork` |
| Inherit my conversation + isolate files | `fork-isolated` |
| Isolate files only | `isolated` |
| Just run something later | `background` |
| Run on another machine | `remote` (NYI) |
