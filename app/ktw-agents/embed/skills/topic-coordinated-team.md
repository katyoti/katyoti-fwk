---
name: topic-coordinated-team
description: >-
  Worked example of the coordinator + specialists pattern using direct topics.
  Shows the topology, full TASK.md, and tracing notes for a manager that
  delegates to N parallel workers and collects results via to.coordinator.message.
iconName: users
icon: "👥"
hashtags: [topics, coordinator, example]
category: orchestration
advertised: false
---

# Coordinated Team Example

A complete worked example of the coordinator + specialists pattern using
direct topics. Mirrors `app/ktw-agents/embed/tasks/demo-delegate-team-coordination/`.

## Topology

```
              ┌──────────────────────────┐
              │  coordinator (LLM agent) │
              └──┬────────┬────────┬─────┘
                 │        │        │   delegate(mode: background)
                 ▼        ▼        ▼
         ┌─────────┐  ┌─────────┐  ┌─────────┐
         │ alice   │  │  bob    │  │ carol   │
         │(worker) │  │(worker) │  │(worker) │
         └────┬────┘  └────┬────┘  └────┬────┘
              │            │            │   send-message(to: "coordinator", ...)
              └────────────┴────────────┘
                           │
                           ▼  (mailbox-bridge → to.coordinator.message)
                ┌──────────────────────────┐
                │  coordinator's mailbox   │
                │ (read via check-messages)│
                └──────────────────────────┘
```

## TASK.md (coordinator)

```yaml
---
name: demo-coordinator
type: agent
description: "Coordinator + 3 specialist workers"
config:
  mode: coordinator
  capability: powerful
  plugins: [standard]
---

# Coordinator

You are the coordinator of three specialists. Your job:

1. Delegate three sub-tasks IN PARALLEL using `delegate(mode: background, ...)`:
   - alice: "Name 3 colors"
   - bob:   "Name 3 shapes"
   - carol: "Name 3 musical instruments"

   Each spawn returns a task ID immediately (background mode is non-blocking).

2. Poll `task-status(task_id)` for each child until status is "done".

3. Read the children's outputs.

4. Compose ONE sentence that uses at least one item from each category.

5. Call `complete(summary: "...")` with the synthesis.
```

The specialists are spawned with `mode: background` so they run concurrently.
The coordinator polls via `task-status`.

## Why direct topics work here

This pattern uses **direct messages** (`to.coordinator.message`) implicitly
because the orchestration plugin's mailbox-EventBus bridge mirrors every
`send-message(to: X, ...)` call to the topic `to.X.message`. The coordinator
doesn't subscribe to topics directly — it reads its mailbox via
`check-messages`, and the bridge populates the mailbox.

The coordinator's own name is set via the orchestration plugin's auto-naming.
Workers know to target `"coordinator"` because that's the convention.

## When to extend this pattern

- **Need workers to talk to each other?** Switch to a team-leader pattern
  (P2 in the catalog) so workers can directly message each other.
- **Need shared state?** Switch to Magentic (P3) with a ledger machine.
- **Need approval before each delegation?** Add a `protocol.<id>.plan-approval`
  exchange between coordinator and a human-gate or supervisor agent.

## Tracing this in practice

When you run a task built like this, the bus emits:
- `child.alice.started`, `child.alice.done` (× 3 for each worker)
- `to.coordinator.message` (× 3 — one reply per worker)
- `workflow.<runID>.started/done` if the coordinator is wrapped in a workflow

The `--dry-trace <file>` CLI flag captures all of these for inspection.
The GUI Team tab renders them live.
