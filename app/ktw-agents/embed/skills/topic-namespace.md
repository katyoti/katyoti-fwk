---
name: topic-namespace
description: >-
  Full topic namespace and payload field reference for ktw-agents EventBus.
  Lists every constructor function, payload struct, and the bus capabilities.
  Use when you need the exhaustive type listing — not the decision tree (load
  topic-conventions for that).
iconName: book
icon: "📒"
hashtags: [topics, eventbus, reference]
category: orchestration
advertised: false
---

# Topic Namespace — Full Reference

The complete topic namespace + payload field listing. Source of truth:
`internal/extensions/task/api/engine/topics.go` and `topic_events.go`.

## Constructor functions

Always use these — never write topic strings inline.

```go
// Child lifecycle (emitted by Spawner)
ChildStartedTopic(name)    // child.<name>.started
ChildDoneTopic(name)       // child.<name>.done
ChildFailedTopic(name)     // child.<name>.failed
ChildSuspendedTopic(name)  // child.<name>.suspended
ChildProgressTopic(name)   // child.<name>.progress
ChildCancelTopic(name)     // to.<name>.cancel  ← actually a "to" topic, see below

// Agent-to-agent direct
AgentMessageTopic(name)    // to.<name>.message
AgentTaskTopic(name)       // to.<name>.task
AgentResultTopic(name)     // to.<name>.result

// Group / team broadcast
GroupMessageTopic(team)        // group.<team>.message
GroupAnnouncementTopic(team)   // group.<team>.announcement

// Workflow lifecycle
WorkflowStartedTopic(runID)    // workflow.<runID>.started
WorkflowDoneTopic(runID)       // workflow.<runID>.done
WorkflowFailedTopic(runID)     // workflow.<runID>.failed

// Structured protocols (correlation ID)
PlanApprovalTopic(id)   // protocol.<id>.plan-approval
PermissionTopic(id)     // protocol.<id>.permission
ShutdownTopic(id)       // protocol.<id>.shutdown

// Ledger (Magentic shared state)
LedgerSnapshotTopic(id) // ledger.<id>.snapshot
LedgerUpdateTopic(id)   // ledger.<id>.update
LedgerCompleteTopic(id) // ledger.<id>.complete
```

## Payload structs (typed)

All payloads embed `Type / Timestamp / Emitter` and implement `EventStamper`
so the executor auto-stamps them before publish.

### Child lifecycle

```go
ChildStartedEvent {
    ChildName, ChildKind, RunID, ParentRunID string
}
ChildDoneEvent {
    ChildName, RunID string
    Output any  // child machine's StepResult.Output
}
ChildFailedEvent {
    ChildName, RunID, Error string
}
```

### Agent-to-agent

```go
AgentMessageEvent {
    From, To, Team, Text, Summary string
}
// AgentTaskEvent and AgentResultEvent share a similar shape with
// task-specific / result-specific fields.
```

### Workflow lifecycle

```go
WorkflowStartedEvent {
    RunID, Name string
    Inputs map[string]any
}
WorkflowDoneEvent {
    RunID string
    Output any
}
WorkflowFailedEvent {
    RunID, Error string
}
```

### Protocols (request/response)

```go
PlanApprovalRequest {
    ID, AgentName, Plan string
}
PlanApprovalResponse {
    ID string
    Approved bool
    Feedback string  // optional
}

PermissionRequest {
    ID, AgentName, ToolName string
    Reason string  // optional
}
PermissionResponse {
    ID, Decision string  // "allow" | "deny" | "allow_always" | "deny_always"
    Rule string          // optional rule to add
}

ShutdownRequest  { ID string }
ShutdownResponse { ID string; Acknowledged bool }
```

### Ledger (Magentic)

```go
LedgerSnapshotEvent {
    State map[string]any
    Version int
}
LedgerUpdateEvent {
    Patch map[string]any  // RFC 7396 JSON Merge Patch
    Source string         // optional sender ID for audit
}
LedgerCompleteEvent {
    FinalState map[string]any  // optional override of current state
    Reason string              // optional
}
```

## Wildcards

**The bus does NOT support wildcard subscriptions.** You must subscribe to
exact topic strings constructed from the constructor functions.

For cross-cutting observability (tracing, debugging), use
`bus.SubscribeAny(handler)` which fires on every Publish regardless of topic.
This is what the dry-run tracer and the GUI's terminal-protocol translator
use to bridge ms08 topics to the GUI Team tab.

## Bus = Channel = EventBus

Three layers, often confused:

- `eventbus.EventBus` — the lowest-level pub/sub interface. Implementations:
  `MemoryEventBus` (in-process), `NopEventBus`.
- `engine.Channel` — wraps the bus, adds per-executor inbox routing and a
  Dead Event Queue. Created by TaskService once per process.
- `engine.ChannelFrom(ctx)` — context helper to retrieve the channel inside a
  factory or plugin. Use `ch.Bus()` to get the underlying EventBus.

When writing a plugin that needs to publish: get the bus via
`engine.ChannelFrom(ctx).Bus()`. When subscribing inside a workflow Step
handler: use a `WaitSpec` so the executor's inbox routing handles delivery
correctly.
