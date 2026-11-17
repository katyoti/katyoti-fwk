---
name: action-demo-item-collection-goal
type: simulator
kind: collection
types: [goal]
description: "Action-system demo — a kind:collection action targeting the `goal` type. Offered at the head of a goal list, to act on all goals rather than one."
config:
  duration_ms: 0
  output:
    status: completed
    note: "kind:collection types=[goal] — offered at the head of a goal list."
---
A `kind: collection` action-system demo. It targets the `goal` type as a whole:
the action appears at the head of a goal list (act on the type, e.g. "create a
goal" / "verify all"), not on one goal.
