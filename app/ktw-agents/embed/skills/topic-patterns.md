---
name: topic-patterns
description: >-
  Catalog of topic-based coordination patterns (P1-P6) with use-cases,
  example shapes, and anti-patterns. Use when matching a coordination
  problem to a known pattern before designing from scratch.
iconName: layout-grid
icon: "🧩"
hashtags: [topics, patterns, coordination]
category: orchestration
advertised: false
---

# Pattern Catalog by Topic Shape

When you have a coordination problem, match it to one of these patterns
before designing from scratch.

## P1 — Direct fan-out + collect (coordinator pattern)

**Shape**: One coordinator agent dispatches N specialists in parallel via
delegate. Specialists send results back via direct messages. Coordinator
collects, synthesizes, completes.

**Topics used**: `to.<coordinator-name>.message` (incoming results)

**When to use**:
- Independent sub-tasks that don't talk to each other
- Specialists are stateless workers (no shared memory between them)
- Result aggregation is centralized

**Example shape**:
```yaml
type: agent
config:
  mode: coordinator
  plugins: [standard]
---
Delegate 3 sub-tasks. Each specialist will send-message to "coordinator" with
their result. Use check-messages to read replies. Synthesize once all replied.
```

**Pitfall**: coordinator must NAME itself something the specialists know to
target. Default is `"coordinator"` if not set explicitly via the team layer.

## P2 — Team broadcast (team-leader pattern)

**Shape**: A team-leader agent creates a team, spawns N teammates. Teammates
talk to each other via `group.<team>.message` (broadcast) or `to.<other-name>.message`
(direct). Leader monitors via `check-messages` on its own mailbox.

**Topics used**:
- `group.<team>.message` (broadcast within team)
- `to.<teammate>.message` (direct between teammates)

**When to use**:
- Specialists need to collaborate during their work (not just at the end)
- Pair review / critique flows
- Anyone-talks-to-anyone communication

**Example shape**:
```yaml
type: agent
config:
  mode: team-leader
  plugins: [standard]
---
1. create-team color-team
2. spawn-teammate alice (instructed to send-message to bob, then check-messages for reply)
3. spawn-teammate bob (instructed to check-messages, reply, complete)
4. Watch task-status until both done
5. Synthesize and complete
```

See `app/ktw-agents/embed/tasks/demo-delegate-team-messaging/TASK.md` for a
complete working example.

## P3 — Magentic (shared mutable ledger)

**Shape**: A `ledger` machine holds a shared JSON state document. Manager +
specialists all read snapshots and emit RFC 7396 merge patches. The plan
itself lives in the ledger; coordination is implicit (read state → act → write
state).

**Topics used**:
- `ledger.<id>.snapshot` (state broadcast on every change)
- `ledger.<id>.update` (patches from any agent)
- `ledger.<id>.complete` (terminate ledger with optional final state)

**When to use**:
- Plan-and-execute with iterative refinement
- Many small contributors converging on a shared deliverable
- Manager needs to see overall progress without polling individual agents

**Architecture**:
```yaml
type: workflow
config:
  steps:
    - kind: parallel
      branches:
        - { name: shared-ledger, task: { type: ledger, id: "magentic-1", initial: { plan: {...} } } }
        - { name: manager,  task: { type: task, plugins: [standard, ledger], prompt: "..." } }
        - { name: alice,    task: { type: task, plugins: [standard, ledger], prompt: "..." } }
        - { name: bob,      task: { type: task, plugins: [standard, ledger], prompt: "..." } }
```

Full example: `app/ktw-agents/embed/tasks/demo-delegate-magentic/TASK.md`

**Critical**: All ledger participants need `plugins: [standard, ledger]`
(NOT just `standard` — the ledger plugin is opt-in). Without it, agents have
no `ledger-update` / `ledger-snapshot` tools.

## P4 — Request/Response Protocol (approval, permission, shutdown)

**Shape**: Agent A publishes a request with a correlation ID; Agent B
publishes a response with the same ID. Used when one agent needs an explicit
yes/no from another.

**Topics used**:
- `protocol.<id>.plan-approval` (request: PlanApprovalRequest, response: PlanApprovalResponse)
- `protocol.<id>.permission` (request: PermissionRequest, response: PermissionResponse)
- `protocol.<id>.shutdown` (request: ShutdownRequest, response: ShutdownResponse)

**When to use**:
- Permission escalation from a worker to its team-leader
- Plan approval gate before expensive execution
- Graceful shutdown coordination

**Pattern**:
1. Requester generates a UUID `id`, publishes RequestEvent on `protocol.<id>.<name>`
2. Requester subscribes (or uses WaitSpec) on the same topic for the matching ResponseEvent
3. Responder receives request, sends ResponseEvent with the same ID
4. Requester filters incoming events by ID to match its own request

The orchestration plugin already implements this pattern internally for
permission requests (`request-permission` / `resolve-permission` / `list-permissions`
tools). Direct topic use is for custom protocols.

## P5 — Workflow lifecycle observation

**Shape**: External observer (GUI, recorder, dashboard) wants to know when a
workflow starts, completes, or fails — without participating in it.

**Topics used**:
- `workflow.<runID>.started` (WorkflowStartedEvent)
- `workflow.<runID>.done` (WorkflowDoneEvent)
- `workflow.<runID>.failed` (WorkflowFailedEvent)

**When to use**:
- GUI Team tab live-rendering
- Telemetry / progress reporting
- Triggering downstream automation

**How**: subscribe to `workflow.<runID>.{started,done,failed}` topics.
Auto-emitted by the workflow machine via `wrapLifecycleEvents` — no extra
code needed in the workflow YAML.

The runID is generated at `taskSvc.Run` time and returned in the response.
Subscribers must capture it from there.

## P6 — Spawner observation (for workflow handlers, not external)

**Shape**: A workflow node needs to wait for a child to complete before
proceeding. The spawner emits `child.<name>.done` and `child.<name>.failed`
automatically.

**Topics used**:
- `child.<name>.started` (ChildStartedEvent — includes ParentRunID)
- `child.<name>.done` (ChildDoneEvent — Output payload)
- `child.<name>.failed` (ChildFailedEvent — Error)

**When to use**: this is internal to the workflow engine. Spawn nodes use
`WaitSpec` to subscribe; you don't usually write this manually unless
implementing a new workflow primitive.

## Anti-patterns

- **Don't subscribe to `child.*` from agent tools** — children fire fast and
  the spawner already routes them. Use the workflow's spawn handler instead.
- **Don't broadcast on `group.*` if you only need ONE recipient** — use
  `to.<name>.message`. Broadcast wakes every teammate's mailbox unnecessarily.
- **Don't use ledger for tiny state** — for "alice replies once with a
  string", use direct messages. Ledger is for state that actually needs
  multi-writer convergence.
- **Don't reuse correlation IDs across protocol exchanges** — generate a
  fresh UUID each time.
