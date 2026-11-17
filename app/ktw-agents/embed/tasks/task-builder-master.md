---
name: task-builder-master
title: Task Builder
type: workflow
description: "Guided item-creation loop: interview/extract → plan → execute → evaluate → review → iterate"
iconName: hammer
icon: "🛠️"
hashtags: [builder, master, workflow, items]
category: builder
config:
  steps:
    # ── Phase a: gather requirements (interview OR extract from existing code) ──
    - kind: switch
      "on": "inputs.source"
      cases:
        interview:
          name: interview
          task:
            type: task
            taskName: task-builder-interview
          input:
            seed: "${inputs.seed}"
            item_type_hint: "${inputs.item_type}"
        extract:
          name: extract
          task:
            type: task
            taskName: task-builder-extract
          input:
            source_path: "${inputs.source_path}"
            item_type_hint: "${inputs.item_type}"
      default:
        name: interview
        task:
          type: task
          taskName: task-builder-interview
        input:
          seed: "${inputs.seed}"
          item_type_hint: "${inputs.item_type}"

    # ── Phase b: plan ──
    - name: plan
      task:
        type: task
        taskName: task-builder-planner
      input:
        # Either results.interview OR results.extract — the planner reads whichever is present
        spec_from_interview: "${results.interview}"
        spec_from_extract: "${results.extract}"
        item_type_hint: "${inputs.item_type}"

    # ── Phase c+d: execute → evaluate loop ──
    - kind: assign
      target: "iteration"
      value: 0
    - kind: assign
      target: "quality_score"
      value: 0
    - kind: assign
      target: "threshold"
      value: "${inputs.threshold ?? 0.7}"
    - kind: assign
      target: "max_iter"
      value: "${inputs.max_iterations ?? 3}"
    - kind: assign
      target: "previous_findings"
      value: []

    - kind: while
      condition: "state.quality_score < state.threshold && state.iteration < state.max_iter"
      max_iterations: 10
      body:
        kind: sequence
        steps:
          - name: execute
            task:
              type: task
              taskName: task-builder-executor
            input:
              plan: "${results.plan}"
              iteration: "${state.iteration}"
              previous_findings: "${state.previous_findings}"

          - name: evaluate
            task:
              type: task
              taskName: task-builder-evaluator
            input:
              plan: "${results.plan}"
              execute_result: "${results.execute}"

          - kind: assign
            target: "quality_score"
            value: "${results.evaluate.quality_score ?? state.threshold}"

          - kind: assign
            target: "previous_findings"
            value: "${results.evaluate.findings ?? []}"

          - kind: assign
            target: "iteration"
            value: "${state.iteration + 1}"

    # ── Phase e: review + summary ──
    - name: review
      task:
        type: task
        taskName: task-builder-reviewer
      input:
        plan: "${results.plan}"
        final_evaluation: "${results.evaluate}"
        iterations: "${state.iteration}"
        threshold: "${state.threshold}"
        max_iterations: "${state.max_iter}"

    - kind: return
      value:
        item_type: "${results.plan.item_type}"
        name: "${results.plan.name}"
        files: "${results.execute.files}"
        evaluation: "${results.evaluate}"
        iterations: "${state.iteration}"
        verdict: "${results.review.verdict}"
        summary: "${results.review.summary}"

---

# Task Builder — Master

Guided lifecycle for creating new ktw-agents items (tasks, agents, skills, services, sidecars).

## What it does

Five-phase orchestrated loop:

```
   ┌─────────────────┐    ┌─────────────────┐
   │  a. interview   │ OR │  a. extract     │   ← inputs.source picks one
   │  (ask user)     │    │  (read code)    │
   └─────────┬───────┘    └─────────┬───────┘
             └──────────┬───────────┘
                        ▼
             ┌──────────────────────┐
             │  b. planner          │   ← uses ktw-agents:workflow-yaml-spec,
             │  (structured JSON)   │     ktw-agents:delegation-modes,
             └──────────┬───────────┘     ktw-agents:topic-conventions skills
                        ▼
             ┌──────────────────────┐
             │  while q < threshold │
             │  AND iter < max:     │
             │  ┌──────────────┐    │
             │  │ c. executor  │    │   ← uses ktw-agents:ktw-item-creator skill
             │  └──────┬───────┘    │
             │         ▼            │
             │  ┌──────────────┐    │
             │  │ d. evaluator │    │   ← uses ktw-agents:run-quality-evaluation skill
             │  └──────┬───────┘    │
             │         ▼            │
             │  assign quality      │
             │  assign iteration++  │
             └──────────┬───────────┘
                        ▼
             ┌──────────────────────┐
             │  e. reviewer         │   ← non-destructive verdict
             │  (summary + verdict) │
             └──────────┬───────────┘
                        ▼
                    return
```

## What to watch in the journal / GUI Team tab

- 5-7 child sessions over the run (1 interview/extract + 1 planner + N×{executor + evaluator} + 1 reviewer)
- `child.execute.done` and `child.evaluate.done` repeat per iteration
- `state.quality_score` and `state.iteration` advance each loop turn (visible in `--dry-trace`)
- Final output is the JSON map declared in the return step

## Common usage

```bash
# Interactive interview, defaults
ktw-agents run task-builder-master

# Build a specific item type with a starter prompt
ktw-agents run task-builder-master \
  --context '{"item_type": "task", "seed": "Build a task that summarizes a PDF"}'

# Extract from existing code instead of interviewing
ktw-agents run task-builder-master \
  --context '{"source": "extract", "source_path": "internal/extensions/foo/", "item_type": "service"}'

# Tighter quality / fewer iterations
ktw-agents run task-builder-master \
  --context '{"threshold": 0.85, "max_iterations": 5}'
```

## How this composes ms08

- **`type: workflow`** — uses Node AST primitives (switch, assign, while, return)
- **`while`** — bounded by both quality target AND hard iteration cap (`max_iterations: 10` in the YAML is just the engine's safety; the runtime cap from `state.max_iter` is the real one)
- **`assign`** — propagates loop state (`quality_score`, `iteration`, `previous_findings`)
- **`switch on:`** — selects between interview and extract sources at workflow boot
- **`taskName: ...`** — references named specialists in the registry (loaded from embedded tasks)
- **Specialist capabilities** — each specialist declares `plugins: [standard, augmented]` plus `skills: ["*"]` so it can load the right reference skill at runtime (skills come from the `skills:` list, never the plugin bundle)

This task is itself a worked example of the workflow-composition pattern. It eats its own dogfood — see also `app/ktw-demo/embed/tasks/demo-delegate-magentic.md` for another example with a different topology.
