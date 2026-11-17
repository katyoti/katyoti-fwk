---
name: node-context
description: "From a node's id, what a worker outside KaTyoti needs to act on it: its context, its actions, its pane, its links. Load when a to-do names its node, or to work on an object by id."
hashtags: [cwm, context, node, worker, outside]
category: methodology
---

# node-context — a node's context, from its id

A step of work runs **on a node** (a feature, a goal, a module, a request…): its to-do
names it (`about`). Inside KaTyoti a chat started on the node receives its context and its
tools; a worker outside gets the same from the node's id, through `ktw-agents`. Use
`--out json` when reading back.

## 1 — What a chat on it receives

```
ktw-agents object-context agent-context --id <node> --out json
```
answers `system` (the rules, decisions and notes in force at the node, layered from the
project down to it — read them, they bind the work), `skills` (load each), `tools` (the
tool groups an agent on it gets) and `notes` (what was left out and why).

Each tool group (`plugin`) is a service with a command group of its own:
`ktw-agents <group> --help` lists its verbs (`comments`, `requests`, `todos`, `cwm`, …).

## 2 — What can be done on it

```
ktw-agents action-surface list --scope object --id <node> --out json
```
answers its actions — label, kind, `uri`. Run one with its input:
```
ktw-agents action run --uri <uri> --context '{"id":"<node>"}' --input '{…}'
```

## 3 — The node as a person sees it

```
ktw-agents object-pane detail --id <node> --out json              # the object, its type, capabilities, links, children
ktw-agents object-pane transition-options --id <node> --out json  # where it may move, and what stands in the way
```

## 4 — Around it

```
ktw-agents cwm object-get --id <node> --out json
ktw-agents cwm object-list --type <type> --out json
ktw-agents cwm edge-list --object-id <node> --out json
```
Read its parent, its children by type and its edges (`depends-on`, `serves`, `targets`…)
to place the work; `ktw-agents:/skill/cwm-graph-navigation` says how to walk the graph.

## Rules

- Read the context before acting; the rules it carries bind the work.
- Change the node only through its verbs and actions — never by editing its files.
- A move a gate refuses says what is missing: do that, never force it.
