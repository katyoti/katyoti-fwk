---
name: action-demo-global
type: simulator
kind: global
description: "Action-system demo — a kind:global action, offered everywhere (the workspace-wide action registry). No target context. ms-action-system: the global scope."
config:
  duration_ms: 0
  output:
    status: completed
    note: "kind:global — offered on every node's action bar; no target threaded."
---
A `kind: global` action-system demo. Global actions are offered regardless of
the current node's type (and, once the global bar lands, on the workspace-level
bar). The resolver includes them in every `ActionList`. No target context is
threaded — a global action operates on the workspace, not a specific node/item.
