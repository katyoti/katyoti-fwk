---
id: rule-topic-constructors-only
title: Topic strings come from constructors in topics.go (and subjects.go)
severity: error
scope: source
applies-to:
  paths: ["internal/extensions/task/api/engine/"]
  module-kind: extension
checks:
  - kind: grep-must-not-match
    path: "internal/extensions/task/api/engine/*.go"
    exclude: ["topics.go", "subjects.go", "topic_events.go", "subject_events.go"]
    regex: '"child\.[^"]*\.(started|done|failed|suspended|progress)"'
    message: 'Inline child.* topic string. Use ChildStartedTopic / ChildDoneTopic / ChildFailedTopic / ChildSuspendedTopic / ChildProgressTopic from topics.go.'
  - kind: grep-must-not-match
    path: "internal/extensions/task/api/engine/*.go"
    exclude: ["topics.go", "subjects.go", "topic_events.go", "subject_events.go"]
    regex: '"to\.[^"]*\.cancel"'
    message: 'Inline "to.<name>.cancel" topic string. Use ChildCancelTopic from topics.go.'
  - kind: grep-must-not-match
    path: "internal/extensions/task/api/engine/*.go"
    exclude: ["topics.go", "subjects.go", "topic_events.go", "subject_events.go", "liveness.go"]
    regex: '"heartbeat\.[^"]*"'
    message: 'Inline "heartbeat.<runID>" topic string. Use HeartbeatTopic from topics.go.'
  - kind: grep-must-not-match
    path: "internal/extensions/task/api/engine/*.go"
    exclude: ["topics.go", "subjects.go", "topic_events.go", "subject_events.go"]
    regex: '"workflow\.[^"]*\.(started|done|failed)"'
    message: 'Inline "workflow.<runID>.<phase>" topic string. Use WorkflowStartedTopic / WorkflowDoneTopic / WorkflowFailedTopic from topics.go.'
  - kind: grep-must-not-match
    path: "internal/extensions/task/api/engine/spawner.go"
    regex: '^func (Child|To|Heartbeat|Workflow)[A-Z][a-zA-Z]+Topic\('
    message: 'Topic constructor defined in spawner.go. Move to topics.go (the canonical home).'
introduced: 2026-05-17
authoritative: CLAUDE.md ("Topic Conventions" + "Subject Conventions")
---

# Rule: Topic constructors only

All topic strings used by the engine MUST come from constructors in
`internal/extensions/task/api/engine/topics.go` (or `subjects.go` for the
NATS-bridged subjects). Inline string literals like `"child.foo.done"` are
forbidden.

## Rationale

Inline topic strings hide three real problems:

1. **Typo drift**: `"child.fanout.done"` vs `"child.fan-out.done"` — runtime miss,
   no compile-time check. The audit found four such drifts before this rule.
2. **Refactor friction**: renaming a topic prefix (e.g. for the NATS subject
   migration) requires grep + manual updates across many files. With
   constructors, the rename is a single function edit.
3. **Discoverability**: a new contributor sees `topics.go` as the catalog of
   every topic the engine speaks. Inline strings hide that catalog.

## Common violations

- `bus.Publish("child."+name+".done", ...)` — typo-prone string concat
- `HeartbeatTopicPrefix + runID` — pre-2026-05 pattern, replaced by `HeartbeatTopic(runID)`
- A new event author adds `"workflow.cancelled"` instead of extending `topics.go` with `WorkflowCancelledTopic`

## Auto-fix

None. The fix is mechanical (move literal → constructor) but requires
choosing or naming a constructor; not safely automatic.

## Test cases

- `internal/extensions/task/api/engine/spawner_impl.go` uses `ChildStartedTopic(spec.Name)` → pass.
- A file with `bus.Publish("child.foo.done", ...)` → fail with rule-topic-constructors-only.
- `internal/extensions/task/api/engine/liveness.go` uses `HeartbeatTopic(runID)` → pass.

## History

- 2026-05-17 introduced. Concurrent with the move of 4 child-topic constructors from `spawner.go` to `topics.go` (`move-child-topic-constructors` task).
