---
name: action-demo-item-goal
type: simulator
kind: object
types: [goal]
description: "Action-system demo — a kind:object action targeting `goal` objects. Offered on every goal's action bar."
config:
  duration_ms: 0
  output:
    status: completed
    note: "kind:object types=[goal] — offered on a goal's action bar."
---
A `kind: object` action-system demo. It targets `goal` objects; the action
appears on every goal's action bar, with the goal threaded as the target.
