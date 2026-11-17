---
name: topic-magentic
description: >-
  Worked example of the Magentic pattern (manager + specialists + shared
  ledger machine). Shows the topology, full TASK.md, the ledger-manager /
  ledger-worker plugin role split, pitfalls (id vs runID, polling vs
  subscriptions, ledger termination + safety hook), and tracing notes.
iconName: database
icon: "🗂️"
hashtags: [topics, magentic, ledger, example]
category: orchestration
advertised: false
---

# Magentic Pattern Example (shared ledger)

A complete worked example of the Magentic pattern: manager + specialists +
shared `ledger` machine holding the plan and coordination state. Mirrors
`app/ktw-agents/embed/tasks/demo-delegate-magentic/`.

## Topology

```
                    ┌──────────────────────────┐
                    │       workflow           │
                    │     kind: parallel       │
                    └──┬─────┬─────┬──────┬────┘
                       │     │     │      │
              ┌────────▼─┐ ┌─▼──┐ ┌▼───┐ ┌▼───┐
              │ ledger   │ │mgr │ │alic│ │bob │
              │  machine │ │(LLM)│ │e   │ │    │
              └────┬─────┘ └─┬──┘ └─┬──┘ └─┬──┘
                   │         │      │      │
                   │         │      │      │  ledger-update
                   │         │      │      │  ledger-snapshot
                   │         ▼      ▼      ▼
                   │   ┌──────────────────────┐
                   └──►│ ledger.<id>.snapshot │ ◄── all 3 agents
                       │ ledger.<id>.update   │ ◄── manager + alice + bob
                       │ ledger.<id>.complete │
                       └──────────────────────┘
```

## TASK.md (full)

```yaml
---
name: demo-delegate-magentic
type: workflow
description: "Magentic: manager + 2 specialists coordinate via shared ledger"
config:
  steps:
    - kind: parallel
      branches:
        - name: shared-ledger
          task:
            type: ledger
            id: "magentic-demo"   # ← stable string ID; agents address this
            initial:
              status: "open"
              plan:
                step1: { task: "name a color", assigned_to: null, result: null }
                step2: { task: "name a shape", assigned_to: null, result: null }

        - name: manager
          task:
            type: task
            capability: powerful
            plugins: [standard, ledger-manager]    # ← manager: snapshot + update + complete + safety hook
            prompt: |
              Manager. The shared ledger ID is "magentic-demo".

              Phase 1 (assign):
              1. ledger-snapshot(ledger_id: "magentic-demo")
              2. ledger-update with patch:
                 { "plan": { "step1": { "assigned_to": "alice" },
                             "step2": { "assigned_to": "bob" } } }

              Phase 2 (wait for completion — event-driven, NOT polling):
              3. Loop using ledger-wait-update (NOT ledger-snapshot):
                 - ledger-wait-update(ledger_id: "magentic-demo",
                                      since_version: <last seen>)
                   blocks until a newer snapshot arrives.
                 - If both plan.step1.result and plan.step2.result are set,
                   exit; otherwise update since_version and call again.
                 NEVER loop ledger-snapshot with same args — the agent
                 loop's repeat-tool-call guard will abort you.
              4. ledger-update with { "status": "done" }

              Phase 3 (terminate the ledger):
              5. ledger-complete(ledger_id: "magentic-demo") so the
                 surrounding workflow can finalize. The OnResult safety
                 hook fires this automatically if you forget — but always
                 prefer explicit teardown.

              Then complete with the synthesis.

        - name: alice
          task:
            type: task
            capability: fast
            plugins: [standard, ledger-worker]
            prompt: |
              You are alice. Ledger ID: "magentic-demo".

              Loop (max 8 turns):
              1. ledger-snapshot(ledger_id: "magentic-demo")
              2. Find your assignment in plan (where assigned_to == "alice")
              3. Once found: do the task, then ledger-update with
                 { "plan": { "step1": { "result": "<answer>" } } }
                 (replace step1 with whichever step is yours)
              4. complete

        - name: bob
          task: { ... same shape as alice ... }

    - kind: return
      value: "results.shared-ledger"
---
```

Run with:
```
ktw-agents run demo-delegate-magentic
```

## Why use ledger over direct messages here?

- **Implicit coordination**: agents don't need to know each other's names.
  They read the shared plan, find their slot, write back. No "alice → bob"
  routing.
- **Resumable**: the ledger snapshot is the full state. If alice crashes and
  retries, she just reads the snapshot and resumes from there.
- **Multi-writer convergence**: all agents can mutate the same document
  using RFC 7396 merge patches. Last-writer-wins per merged key, and the
  `Version` field lets consumers detect stale reads.
- **Manager observability**: manager polls one snapshot to see ALL progress
  rather than checking N teammate mailboxes.

## Pitfalls and conventions

### `id:` vs auto runID

The ledger machine's `id:` config is **mandatory** when you want sibling
agents to address it. Without an explicit `id`, the ledger uses its
auto-assigned runID for topics — but sibling agents have no way to know that
runID at YAML-write time. Always set `id:` to a stable string in the workflow.

### Plugin role split (manager vs worker)

Two role-specific plugins, NOT a single `ledger` plugin (deprecated):

| Plugin            | Tools                                   | Who gets it                                        |
|-------------------|-----------------------------------------|----------------------------------------------------|
| `ledger-manager`  | snapshot + update + **`ledger-complete`** | The delegator (manager / coordinator / team-leader) |
| `ledger-worker`   | snapshot + update                       | Specialists / delegates                             |

The split encodes lifecycle responsibility — only the manager can tear down
the shared state. Workers can't accidentally `ledger-complete` and bring the
whole workflow down.

The `ledger-manager` plugin also wires an `OnResult` + `OnAbort` safety hook
that auto-fires `LedgerCompleteEvent` for every ledger this agent has touched,
in case the manager finished without calling `ledger-complete` explicitly.
This is belt-and-suspenders: prefer explicit teardown in the prompt, but the
hook makes "manager forgot" non-catastrophic.

Neither plugin is in the `standard` group by default — per-task opt-in keeps
the tool surface focused.

### Polling vs blocking wait — use `ledger-wait-update`, NOT looped `ledger-snapshot`

The agent loop has a **repeat-tool-call circuit breaker** that aborts agents calling the same tool with identical arguments 3+ times. This makes naïve polling (looped `ledger-snapshot`) impossible — the agent gets aborted before any state change shows up in the snapshot.

The fix is `ledger-wait-update(ledger_id, since_version)`:
- Blocks until snapshot.version > since_version (or timeout, default 30s)
- Each call has a different `since_version`, so the repeat-call guard never fires
- Faster than polling (no 500ms gaps), cheaper (fewer API calls), event-driven (returns the moment something changes)

**Rule of thumb**:
- `ledger-snapshot` → "give me the current state right now" (one-shot read)
- `ledger-wait-update` → "wait until something changes, then give me the new state" (the coordination primitive)
- **Never** loop `ledger-snapshot` waiting for a change.

Internally the plugin subscribes lazily on first interaction and caches each snapshot event. The wait variant just blocks on the cache until the version bumps past the floor the agent passed in.

### Ledger termination is the manager's responsibility

The ledger machine has no natural end — it suspends forever waiting for
`LedgerCompleteEvent`. The `ledger-manager` plugin gives the manager the
`ledger-complete` tool to fire it explicitly, AND wires a safety hook so
the workflow doesn't hang if the manager forgets.

In normal operation: manager calls `ledger-complete` as its last action →
ledger machine terminates → workflow's parallel block finalizes → workflow
done.

In failure paths (manager crashes, hits max turns, runs out of budget):
the manager plugin's `OnResult` / `OnAbort` hook fires `ledger-complete`
on the manager's behalf. No hang, even on the unhappy path.

If you run a ledger machine standalone (not inside a workflow with a
`ledger-manager`-equipped agent), you're responsible for sending
`LedgerCompleteEvent` directly to terminate it.

For dry-run mode: `ledger` is in the `DryRunSubstituted` set so it's
replaced by a simulator (otherwise the dry-run would hang forever waiting
for non-existent agent tool calls).

## Tracing in practice

When this runs, the bus emits:
- `child.shared-ledger.started`, `child.manager.started`, `child.alice.started`, `child.bob.started`
- `ledger.magentic-demo.snapshot` (after every state change — many of these)
- `ledger.magentic-demo.update` (one per `ledger-update` call from any agent)
- `child.manager.done`, `child.alice.done`, `child.bob.done`
- `workflow.<runID>.done`

The GUI Team tab renders all four cards live, and the ledger snapshot stream
(if you click into the ledger card) shows the plan evolving turn by turn.
