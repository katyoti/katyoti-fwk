---
name: create-workflow
description: "How to author a ktw workflow — a task with type:workflow whose config.steps is a tree of orchestration nodes. Load when creating a task that coordinates multiple steps with no per-step reasoning."
advertised: false
hashtags: [workflow, authoring, orchestration, create-items]
category: guide
---

# Authoring a workflow

A workflow IS a task with `type: workflow` — the body is documentation and the
real work is `config.steps`, a tree of nodes that orchestrate child machines
(agents, simulators, ledgers, templates). Reach for a workflow when the
coordination is **fixed and structural** — sequence, parallel, branch, loop —
with no per-step LLM reasoning. If each step must reason, those steps are agents
the workflow spawns. Load `ktw-compose:/skill/create-task` for the lifecycle
and `ktw-compose:/skill/task-format` for the frontmatter contract.

## Anatomy

```yaml
---
name: my-workflow
type: workflow
description: "What this workflow does"
config:
  steps:
    - name: step-a
      task: { type: task, prompt: "..." }
    - name: step-b
      task: { type: simulator, output: { x: 1 } }
      input: { prev: "results.step-a" }     # ← expression binding
    - kind: return
      value: "results.step-b"
---
Body markdown is documentation only — workflows have no prompt body.
```

The top-level `config.steps:` is an implicit **sequence**. Each entry is either a
**spawn** (has a `task:` block → runs one child machine) or a **control node**
(has a `kind:`).

A self-contained workflow (inline `task:` blocks) is one flat `<name>.md`, like
any task. If steps reference **child task files**, use the folder form
`tasks/<slug>/TASK.md` with the children beside it — see
`ktw-compose:/skill/task-format` for the file-layout rule.

## The 12 primitives

Pick the node by the coordination it expresses:

| Need | Primitive |
|---|---|
| run steps in order | `sequence` (implicit at top level) |
| run a child machine | spawn (a `task:` block) |
| run branches at once, wait for all | `parallel` |
| run branches, take the first to finish | `race` |
| branch on a condition | `if` / `switch` |
| repeat | `loop` (count) / `while` (cond) / `foreach` (collection) |
| pause for an external event | `wait` |
| set workflow state | `assign` |
| end with a value | `return` |

The full per-primitive YAML shape + semantics is
`ktw-compose:/skill/workflow-primitives`. Worked end-to-end
examples are `ktw-compose:/skill/workflow-examples`.

## Bindings, errors, modes

- **Expressions** wire one step's output into another's input
  (`results.<step>`, `inputs.<x>`, `state.<x>`, `loop.<x>`). The grammar is
  `ktw-compose:/skill/workflow-expressions`.
- **On error**, a node's `on_error` controls continue / retry / fallback /
  propagate — `ktw-compose:/skill/workflow-on-error`.
- A spawned agent's **delegation mode** is `ktw-agents:/skill/delegation-modes`;
  multi-agent communication patterns are `ktw-agents:/skill/topic-conventions`.
  Name them; do not restate them.

## Testing

Run it and inspect the session — confirm each step spawned, bindings resolved,
branches/loops took the expected path, and the `return` produced the result.
First-run failures are catalogued in
`ktw-compose:/skill/failure-patterns`.
