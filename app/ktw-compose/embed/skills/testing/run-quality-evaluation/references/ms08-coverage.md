# ms08 Coverage — What This Skill Sees

A precise audit of which ms08 task types this skill can fully evaluate vs
those with gaps, plus workarounds.

## Gap matrix

| Task type | Children appear in `session tree`? | Aggregate metrics covered? | Per-child detail? | Verdict |
|-----------|-----------------------------------|---------------------------|-------------------|---------|
| `type: agent` (single-agent) | n/a | yes | n/a | ✅ Full |
| `type: agent`, `mode: coordinator` + `delegate` children | yes (each child = own session) | yes | yes | ✅ Full |
| `type: agent`, `mode: team-leader` + `spawn-teammate` children | yes | yes | yes | ✅ Full |
| `type: workflow`, `kind: spawn/parallel/race` of agent children | NO (one parent session) | yes (totals only) | NO | ⚠ Partial |
| `type: workflow` containing `simulator` children | NO | yes (totals only) | NO | ⚠ Partial |
| `type: workflow` containing `ledger` machine + agent siblings | ledger NO; agents NO | partial (LLM agents counted; ledger is zero-cost) | NO | ⚠ Partial |
| Background-delegated sub-agents (via `delegate(mode: background)`) | yes | yes | yes | ✅ Full |
| Workflow nested inside agent (delegated workflow) | yes (workflow's session) | yes | partial | ⚠ Partial |

## Why the gap

`ktw-utils session tree` reconstructs the delegation tree by walking
`agent.session_start` events whose `ParentRunID` is set. Two paths set
`ParentRunID`:

1. **`delegate` tool** → spawns a sub-agent via BackgroundTaskManager →
   creates a NEW session → emits `session_start` with `ParentRunID` → seen
   by `session tree`. ✅
2. **`workflow.spawner.LocalSpawner`** → spawns a child machine inline within
   the parent's process / shared bus, NOT a new session. The child emits
   `child.<name>.started` on the bus (consumed by GUI Team tab via the
   translator bridge from gui-bridge-agent-sessions-to-team) but NOT a
   session-level event. So `session tree` doesn't see it. ⚠

This is by design: workflow children share the parent's checkpoint /
recorder so they don't need separate sessions. But it leaves
`session tree` with an incomplete view for workflow-typed runs.

## Workarounds for evaluating workflow runs

Until `ktw-utils` is extended (see "Future work" below), use one of:

### (a) Read the parent session's journal for workflow.* and child.* events

The parent workflow's session journal contains these events even though
they don't create separate sessions:

```bash
./bin/ktw-utils.exe session show <workflow-session-id> --json | jq '.events[] | select(.topic | startswith("workflow.") or startswith("child."))'
```

Each event has timing, payload, runID. Can extract per-child duration,
on_error path coverage (was a `child.<name>.failed` followed by a
`child.<name>-fallback.started`?), parallel collect mode efficiency.

### (b) Use `--dry-trace <file>` for topology inspection

Re-run the workflow under `--dry-run --dry-trace events.jsonl` to capture
ALL bus events at zero LLM cost. Inspect events.jsonl for:

- Workflow lifecycle (`workflow.<id>.{started,done,failed}`)
- Child timing (`child.<name>.{started,done,failed}`)
- Ledger activity (`ledger.<id>.{snapshot,update}`)
- Inter-agent messages (`to.<name>.message`, `group.<team>.message`)

The dry-run substitutes agents with simulators so the topology + event
flow is identical (modulo content). Useful for evaluating "did the
workflow exercise its branches as expected" without re-paying for LLM.

### (c) Run the evaluator alongside a live GUI Team tab session

The GUI Team tab's Zustand store (after gui-team-tab-functional landed)
already captures all team.* events via the translator bridge. Future
iteration could expose this state via an HTTP endpoint or file dump for
the evaluator. Not implemented yet.

## Multi-agent wasteful patterns (added in ms08)

Beyond the original "ignored input / retries / polling without backoff"
patterns, ms08-era runs can show:

- **Stranded ledger updates** — a `ledger-update` was published but no
  subsequent `ledger-snapshot` was read by anyone. Likely the agent forgot
  to coordinate, or the ledger was abandoned mid-run.
- **Uncollected parallel branches** — a `kind: parallel` with
  `on_branch_error: collect` had branches succeed but their `results.<name>`
  was never read by a downstream step (only the workflow's final return).
- **Fallback never triggered** — an `on_error: { fallback }` was declared
  but the primary always succeeded. Either the fallback is dead code or
  the failure scenario isn't being exercised in tests.
- **Cancelled siblings (race / fail_fast)** — sibling branches were
  cancelled because one branch won/failed. Acceptable for race, suspicious
  for fail_fast (suggests under-tested error paths).
- **Empty teammate mailboxes** — a teammate was spawned but never received
  a message during its run. Likely over-spawning or coordination failure.
- **Coordinator over-supervision** — coordinator's tool calls > 3 ×
  delegations (e.g., excessive `task-status` polling). Tag as `⚠ acceptable`
  if low-cost tool, `✗ wasteful` if high-cost.
- **Mode mismatch** — a `mode: coordinator` agent that spawned 0 children
  (just talked); or a `mode: worker` that tried (and failed) to use
  orchestration tools. Suggests the wrong mode was picked.

These are recorded as entries in the `efficiency.wastefulPatterns` array
of evaluation.json, with `severity` and a one-line description per the
rubric.

## Future work (not in this update)

- **Extend `ktw-utils session aggregate` to include workflow/child/ledger
  topic counts** — would let evaluators get per-workflow-child stats
  without manually parsing journals.
- **New `ktw-utils session topology --json`** — dedicated command that
  walks workflow + agent events together to produce a unified delegation
  tree.
- **Replay-from-dry-trace** — read a saved `--dry-trace events.jsonl` and
  produce evaluation.json directly (zero new LLM cost).

These are tracked outside this skill — open new todos when needed.
