---
name: workflow-examples
description: "Worked end-to-end YAML examples for ktw-agents workflows: sequential, parallel-with-fallback, shared-ledger, coordinator, and foreach fan-out. Load when you need a concrete template to start from."
iconName: code
icon: "📜"
hashtags: [workflow, examples, demo, templates]
category: orchestration
advertised: false
---

# Worked Examples

Five complete workflow YAMLs distilled from the embedded `demo-*` tasks.
The originals are in `app/ktw-agents/embed/tasks/demo-*/TASK.md` and are
runnable via `ktw-agents run <name>`.

---

## 1. Sequential pipeline (3 agents in order)

From `demo-pipeline-agents`. Three agent steps run sequentially; each step's
output feeds the next via `input:` bindings.

```yaml
config:
  steps:
    - name: researcher
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: "Name 3 colors. Reply with one comma-separated line."

    - name: analyst
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: |
          The researcher gave you these colors: {{ .colors }}
          Categorize each as warm/cool/neutral.
      input:
        colors: "results.researcher"

    - name: writer
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: |
          Categorization: {{ .categorization }}
          Compose ONE sentence using all 3 colors.
      input:
        categorization: "results.analyst"

    - kind: return
      value: "results.writer"
```

Topology: 3 child sessions appear in sequence in the GUI Team tab.

---

## 2. Parallel + on_error.fallback (zero LLM cost)

From `demo-parallel-fallback`. Three branches in parallel; branch C's
primary always fails and recovers via fallback. Pure simulator — zero LLM
tokens.

```yaml
config:
  steps:
    - name: fanout
      kind: parallel
      on_branch_error: collect
      branches:
        - name: branch-a-success
          task: { type: simulator, duration_ms: 100, output: { source: "A" } }
        - name: branch-b-success
          task: { type: simulator, duration_ms: 150, output: { source: "B" } }
        - name: branch-c-with-fallback
          task: { type: simulator, fail_probability: 1.0 }
          on_error:
            fallback:
              name: branch-c-fallback
              task: { type: simulator, output: { source: "C-backup" } }
    - kind: return
      value: "results"
```

Topology: 3 child sessions concurrent + 1 fallback after branch C fails.

---

## 3. Magentic — shared ledger + manager + N specialists

From `demo-magentic-pattern`. Workflow.parallel runs a `ledger` machine
alongside a manager + 2 specialist agents that coordinate via the ledger.

```yaml
config:
  steps:
    - kind: parallel
      branches:
        - name: shared-ledger
          task:
            type: ledger
            id: "magentic-demo"   # stable string for sibling addressing
            initial:
              status: "open"
              plan:
                step1: { task: "name a color", assigned_to: null, result: null }
                step2: { task: "name a shape", assigned_to: null, result: null }

        - name: manager
          task:
            type: task
            capability: powerful
            plugins: [standard, ledger]   # ledger plugin is opt-in
            prompt: |
              Read the ledger, assign step1 to alice and step2 to bob,
              poll until both have results, then mark status: done.

        - name: alice
          task:
            type: task
            capability: fast
            plugins: [standard, ledger]
            prompt: |
              Watch the ledger for your assignment, do the work,
              post the result back via ledger-update.

        - name: bob
          task:
            type: task
            capability: fast
            plugins: [standard, ledger]
            prompt: "(same as alice with step2)"

    - kind: return
      value: "results.magentic-team.manager"
```

See **topic-conventions** skill's Magentic example for ledger details.

---

## 4. Coordinator + 3 background workers (agent-as-coordinator pattern)

From `demo-team-coordination`. **Note: this is `type: agent` (not workflow)** —
included here for contrast. Use this when LLM intelligence drives orchestration;
use a workflow when topology is static and known up front.

```yaml
type: agent
config:
  mode: coordinator
  capability: powerful
  plugins: [standard]
---
You are the coordinator. Delegate three trivial sub-tasks IN PARALLEL using
delegate(mode: background, task_name: "ad-hoc"):
- worker 1: "Name 3 colors"
- worker 2: "Name 3 shapes"
- worker 3: "Name 3 musical instruments"

Poll task-status for each until done, then synthesize one sentence.
Call complete with the synthesis.
```

---

## 5. Foreach over a collection (parallel mode)

Pattern fragment (no shipped demo). Useful when the fan-out width is only
known at runtime.

```yaml
config:
  steps:
    - name: list-files
      task:
        type: agent
        prompt: "List the .md files under inputs/ and return their paths."
      # Output assumed: { paths: ["a.md", "b.md", "c.md"] }

    - kind: foreach
      items: "results.list-files.paths"   # ← `items:` (NOT `collection:`)
      mode: parallel
      body:
        name: process
        task:
          type: task
          plugins: [standard]
          prompt: |
            Process the file at path: {{ .path }}
            (iteration {{ .index }} of the batch)
        input:
          path: "loop.item"
          index: "loop.index"

    - kind: return
      value: "results"
```

With `mode: parallel`, all items spawn concurrently. With `mode: sequential`
(default), items run one at a time.

If you need multiple steps per iteration, wrap them in a sequence:

```yaml
- kind: foreach
  items: "results.list-files.paths"
  mode: sequential
  body:
    kind: sequence
    steps:
      - name: read
        task: { type: ... }
        input:
          path: "loop.item"
      - name: process
        task: { type: ... }
        input:
          content: "results.read"
```

---

## When to compose

- **Pipeline** → top-level `steps[]`, no nested control
- **Parallel investigation** → wrap in `kind: parallel`
- **Conditional branch** → `kind: if` (wrap multi-step branches in
  `kind: sequence`)
- **Polling loop** → `kind: while` with a `state` counter — remember
  `max_iterations:` is mandatory
- **Iterate over discovered items** → `kind: foreach` with `items:`
  (`mode: parallel` for fan-out)
- **Race against a timeout** → `kind: race` with a `kind: wait` branch
  (using `duration_ms:`)
- **Multi-step recovery** → `on_error: { fallback: { kind: sequence, steps: [...] } }`
