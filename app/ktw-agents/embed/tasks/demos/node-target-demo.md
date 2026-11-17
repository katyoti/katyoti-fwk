---
name: node-target-demo
type: simulator
kind: object
types: [document]
filter: |
  object.content?.status == "active" && id != ""
description: Demo of a kind:object task with an optional el filter over the object — the GUI offers this task only on an active document with a non-empty id. Same expression syntax authors already use in workflow if/foreach/while.
config:
  duration_ms: 0
  output:
    status: completed
    note: this task targets `document` objects that pass the active+id filter; check Runtime.CurrentNode at the strategy.
---
This task is a self-documenting demo of the `kind: object` targeting feature
plus the optional `filter:` expression. The runtime threads the target object
through TaskExecRequest.Runtime.CurrentNode so the strategy can read it; the
filter decides whether the task is offered for a given object.
