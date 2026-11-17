---
name: workflow-primitives
description: "Per-primitive YAML reference for the ktw-agents workflow DSL: the shape, semantics, and pitfalls of all 13 control-flow primitives. Load after create-workflow when you need exact field shapes."
iconName: puzzle
icon: "🧩"
hashtags: [workflow, yaml, primitives, reference]
category: orchestration
advertised: false
---

# Per-Primitive Reference

Each primitive: YAML shape, semantics, example, on_error usage, pitfalls.

Source: `internal/extensions/task/internal/wkflow/node.go`,
`loader.go`, `task_strategy.go`.

---

## Expression evaluation — TWO modes (read this first)

String fields are evaluated against the workflow Scope (`inputs.*`,
`state.*`, `results.*`, `loop.*`, `now`). But there are **two distinct
modes**, and using the wrong one silently breaks the workflow:

**Mode A — BARE predicate contexts.** The whole string IS the expression,
evaluated directly. Do NOT wrap in `${...}` (a `${` here is a syntax error).

| Field | Example |
|---|---|
| `if.condition` | `condition: "results.check.passed == true"` |
| `while.condition` | `condition: "state.retries < 3"` |
| `switch.on` | `on: "results.classify.category"` |
| `foreach.items` | `items: "results.list.paths"` |

**Mode B — `${...}`-WRAPPED value contexts.** Literal by default; a string
is only evaluated when wrapped in `${...}`. A bare string stays literal.

| Field | Literal | Evaluated |
|---|---|---|
| `input:` binds | `q: "hello"` | `q: "${inputs.user_input}"` |
| `assign.value` | `value: "ready"` | `value: "${state.retries + 1}"` |
| `return.value` | `value: "done"` | `value: "${results}"` |

Rule of thumb: control-flow PREDICATES (A) exist to evaluate, so they take
the expression raw. Data-carrying VALUES (B) usually carry literals, so
evaluation is opt-in. `assign.target`, `switch` case keys, and `loop.times`
are always literal (never expressions).

---

## delegate (and the implicit `task:` sugar)

Runs ONE child state machine. THE child-run primitive (absorbed the former
`spawn` on 2026-06-10). Three addressing forms — pick exactly one:

- inline `task:` (anonymous child definition),
- `taskName:` (a registered task), or
- `uri:` (a task action URI, `<provider>:tasks/<name>`).

A step with a `task:` block and no `kind:` is sugar for `kind: delegate`
(inline form) — the most common shape.

**YAML shape**:
```yaml
- name: <step-name>          # required, used as results key + topic suffix
  task:                      # inline form (OR taskName: OR uri:)
    type: <kind>             # task | agent | simulator | ledger | template | program | workflow
    # ... type-specific config (prompt, capability, plugins, output, etc.)
  mode: worker               # delegation mode → dimensions + await semantics
  max_duration_s: 0          # optional per-delegate wall-clock cap
  input:                     # optional; Mode B (${...} to evaluate)
    foo: "${results.previous-step}"
    bar: "${inputs.user_input}"
  on_error:                  # optional — see workflow-on-error skill
    strategy: continue       # OR fallback / retry / fail
```

**Await is mode-derived**:
- Foreground modes (`worker`, `coordinator`, `team-leader`, empty=worker):
  the workflow SUSPENDS until `child.<name>.done` / `.failed`; the child's
  output lands in `results.<name>`.
- Background modes (`background`, `fork`, `isolated`, `fork-isolated`,
  `teammate`): the workflow does NOT suspend — `results.<name>` gets
  `{run_id, child, background: true}` immediately and the next step runs.
  Join later with `kind: wait, for_event: "child.<name>.done"`.

`child.<name>.started/.done/.failed` lifecycle events fire on both paths
(consumed by the GUI Team tab). See the **delegation-modes** skill for the
full mode → dimension mapping.

**Pitfalls**:
- `name:` MUST be unique within siblings (it's the results key).
- Exactly one of `task:` / `taskName:` / `uri:` — two or zero is an error.
- A `uri:` here MUST address a task (`<provider>:tasks/<name>`); a sync
  action URI (a method or script) belongs to `call`, not `delegate`.

---

## call

Invokes an action URI as ONE synchronous request/response through
ActionService — a script (`<provider>:scripts/<name>`) or a service method
(`<provider>:<slot>/<method>`). NOT a child state machine: no run id, no
lifecycle events, no Team-tab card, no dimensions. The result lands in
`results.<name>`.

**YAML shape**:
```yaml
- name: enrich
  kind: call
  uri: "ktw-example:example/search"   # SYNC action (script or method)
  input:                                     # Mode B (${...} to evaluate)
    query: "${results.extract.app_name}"
  timeout_ms: 30000                          # optional; default 60000, cap 600000
  on_error:                                  # optional — retry/fallback/continue
    strategy: retry
    max_retries: 2
```

**Semantics**: ActionService runs the action, bounded by `timeout_ms`. One
typed `call.<name>.completed` event fires per attempt (uri / duration /
attempt / outcome) for journal + session-view observability. Failures
(including timeout) route through `on_error`.

**Pitfalls**:
- `uri:` MUST be a SYNC action; a task URI is an error ("use delegate").
- Dimension params (`mode:`, `max_duration_s`, …) are NOT valid on a call —
  they're delegate-only and are rejected at load time.
- `on_error: retry` re-invokes the action — ensure it is idempotent.
- For method-call semantics WITH a run id / budget / Team presence, wrap the
  action as a task and use `delegate` instead.

---

## sequence

Run children in order, one at a time. Each child completes before the next
starts. Implicit at the top-level (`config.steps[]` IS a sequence).

**YAML shape**:
```yaml
- kind: sequence
  steps:
    - name: a
      task: { ... }
    - name: b
      task: { ... }
```

**Semantics**: pushes children one at a time onto the activation stack.
Failure of any child propagates up unless its `on_error` handles it.

**When to use explicitly**: inside a parallel branch that needs multiple
sequential steps; inside an if/else body; inside a loop/while/foreach body
that needs more than one step. At the top level, just use `config.steps[]`
directly — no `kind: sequence` wrapper needed.

---

## parallel

Spawn N branches concurrently, wait for all (or first failure if `fail_fast`).

**YAML shape**:
```yaml
- name: fanout
  kind: parallel
  on_branch_error: fail_fast      # OR collect (default: fail_fast)
  branches:
    - name: branch-a
      task: { ... }
    - name: branch-b
      task: { ... }
```

**Semantics**:
- All branches spawn at once via the spawner
- Workflow suspends on `child.<name>.done` / `child.<name>.failed` per branch
- `fail_fast`: a branch failure cancels all sibling branches and propagates
  the failure (like `race` but waits for cancels to complete)
- `collect`: a branch failure is recorded but siblings continue; the parallel
  completes when all branches have ended (success or failure)

**Output**: `results.<branch-name>` for each branch (failed branches → nil).

**Pitfalls**:
- **Every branch MUST be a delegate** (a `task:`/`taskName:`/`uri:` step).
  Non-delegate branch kinds (`kind: sequence`, `kind: wait`, …) are rejected
  at PARSE time. For composite branch logic, author it explicitly as a
  nested workflow child: `task: {type: workflow, steps: [...]}` (its own
  results/state scope; pass what it needs via `input:`).
- All branches see the SAME `results.*` and `state.*` snapshot (no live
  cross-branch communication via results — use topics for that)
- `on_branch_error` is at the parallel node level; the parallel node ITSELF
  can also have `on_error:` for handling the whole parallel's failure
- **Per-branch `on_error:` works inside parallel** (continue / retry /
  fallback) and takes precedence over `on_branch_error` for that branch:
  `continue` records a nil result, `retry` respawns the branch under the
  same name, `fallback` spawns the fallback delegate — its output lands
  under the FALLBACK child's name. Recovery is single-level: a failing
  fallback falls through to `on_branch_error`.
- The loader rejects `on_branch_error` values other than `fail_fast` or
  `collect`

---

## race

Spawn N branches concurrently. First to SUCCEED wins; the rest are cancelled.

**YAML shape**:
```yaml
- name: fastest-source
  kind: race
  timeout_ms: 5000              # optional: bound the whole race
  branches:
    - name: cache
      task: { ... }
    - name: live
      task: { ... }
  on_error:                     # timeout-default pattern
    fallback:
      name: default-source
      task: { type: simulator, output: { from: "default" } }
```

**Semantics** (first-SUCCESS-wins — like `Promise.any`, not `Promise.race`):
- All branches spawn at once.
- The first `child.<name>.done` wins: its output becomes
  `results.<branch-name>`; all other branches receive `Spawner.Cancel`.
- A branch FAILURE does NOT settle the race — that branch is eliminated and
  the survivors keep racing.
- If EVERY branch fails, the race fails with a structured Failure, subject to
  the race node's `on_error` (retry/fallback/continue).
- `timeout_ms` (optional): if no branch settles within the window, all
  children are cancelled and the race fails — combine with
  `on_error: fallback` for the timeout-default pattern (shown above).
  Fine print: an intervening branch FAILURE restarts the window; with no
  intervening events (the normal timeout case) it behaves as an absolute bound.

**When**: redundant queries (cache + live source) where a slow-but-failing
source must not kill a slower-but-succeeding one; with-timeout fetches.

**Pitfalls**:
- **Every branch MUST be a delegate** — non-delegate branch kinds are
  rejected at parse time. The old `kind: wait` timeout-branch idea is
  superseded by the `timeout_ms:` attribute.

---

## if

Binary branch on a boolean expression.

**YAML shape**:
```yaml
- kind: if
  condition: "results.check.passed == true"
  then:
    name: passed-step
    task: { ... }
  else:                           # optional
    name: failed-step
    task: { ... }
```

For multiple steps in a branch, wrap them in a sequence:
```yaml
- kind: if
  condition: "inputs.dry_run == false"
  then:
    kind: sequence
    steps:
      - name: deploy
        task: { ... }
      - name: notify
        task: { ... }
  else:
    name: log-skip
    task: { ... }
```

**Semantics**: evaluates `condition` against the current Scope. If truthy,
runs `then`; else `else`. Both `then` and `else` are SINGLE nodes (maps),
not lists.

**Pitfalls**:
- `then:` and `else:` each take ONE node, not a list — wrap multiple steps
  in `kind: sequence` with a `steps:` array
- `condition` must be a STRING expression (quotes), not a YAML boolean
- Truthiness rules (per scope.go): `bool true` / non-zero number / non-empty
  string → truthy; everything else → falsy

---

## switch

Multi-way branch on a value expression.

**YAML shape**:
```yaml
- kind: switch
  on: "results.classify.category"     # ← field name is `on:` (NOT `value:`)
  cases:
    "warm":
      name: warm-handler
      task: { ... }
    "cold":
      name: cold-handler
      task: { ... }
  default:                             # optional, single node
    name: unknown-handler
    task: { ... }
```

For multiple steps in a case, wrap in a sequence:
```yaml
- kind: switch
  on: "results.classify.category"
  cases:
    "warm":
      kind: sequence
      steps:
        - name: a
          task: { ... }
        - name: b
          task: { ... }
```

**Semantics**: evaluates `on`, matches against each case key (string
equality), runs that case's body. Falls through to `default` if no match.

**Pitfalls**:
- The field name is `on:` (NOT `value:`)
- Each case body and `default:` is a SINGLE node — wrap multiple steps in
  `kind: sequence`

---

## loop

Run a body N times.

**YAML shape**:
```yaml
- kind: loop
  times: 3                        # ← field name is `times:` (NOT `count:`); LITERAL int only (not an expression)
  body:
    name: iteration
    task: { ... }
```

For multiple steps in the body, wrap in a sequence:
```yaml
- kind: loop
  times: 5
  body:
    kind: sequence
    steps:
      - name: step1
        task: { ... }
      - name: step2
        task: { ... }
```

**Semantics**: runs `body` (a SINGLE node) `times` times sequentially.
`loop.index` is NOT set inside `loop` — use `foreach` over a range for that.

**Pitfalls**:
- The field name is `times:` (NOT `count:`)
- The loader rejects `times` values that are not `> 0`
- `body:` is a SINGLE node — wrap multiple steps in `kind: sequence`

---

## while

Loop while a condition is truthy.

**YAML shape**:
```yaml
- kind: while
  condition: "state.retries < 3"
  max_iterations: 10              # MANDATORY safety cap (must be > 0)
  body:
    kind: sequence
    steps:
      - name: try
        task: { ... }
      - kind: assign
        target: "retries"
        value: "${state.retries + 1}"   # Mode B: ${...} to evaluate
```

**Semantics**: evaluates `condition` BEFORE each iteration. If truthy, runs
body. `max_iterations` caps total runs to prevent runaway loops.

**Pitfalls**:
- `max_iterations:` is **MANDATORY** and must be `> 0` — the loader fails
  if it is missing or zero
- `body:` is a SINGLE node — wrap multiple steps in `kind: sequence`
- The body MUST mutate state in a way that eventually makes `condition`
  falsy, or you'll hit `max_iterations` and fail
- Use `assign` to update `state` between iterations

---

## foreach

Iterate over a collection (list).

**YAML shape**:
```yaml
- kind: foreach
  items: "results.list-files.paths"   # ← field name is `items:` (NOT `collection:`)
  mode: sequential                     # OR parallel (default: sequential)
  max_concurrency: 4                   # optional cap when mode: parallel
  body:
    name: process
    task: { ... }
    input:                               # Mode B: ${...} to evaluate
      path: "${loop.item}"
      index: "${loop.index}"
```

For multiple steps in the body, wrap in a sequence:
```yaml
- kind: foreach
  items: "results.list.paths"
  mode: parallel
  body:
    kind: sequence
    steps:
      - name: read
        task: { ... }
      - name: process
        task: { ... }
```

**Semantics**:
- `mode: sequential` — runs body once per item, in order. `loop.index` and
  `loop.item` are injected into the Scope for the body
- `mode: parallel` — spawns ALL items concurrently (like a parallel node
  derived from the collection size). Sibling iterations don't see each
  other's results

**Pitfalls**:
- The field name is `items:` (NOT `collection:`)
- `body:` is a SINGLE node — wrap multiple steps in `kind: sequence`
- `items` must evaluate to a list; a non-list errors out
- In parallel mode, results land in `results.<body-name>-<index>` (or
  similar); check the loader for exact key shape
- Loop variables are scoped to the body — they don't leak out

---

## wait

Pause the workflow.

**YAML shape**:
```yaml
- kind: wait
  duration_ms: 1000                # OR
  for_event: "ledger.shared.snapshot"  # ← field name is `for_event:` (NOT `topic:`)
  timeout_ms: 5000                 # optional timeout when waiting for_event
```

**Semantics**: workflow returns `StepResult{Wait: ...}` and the executor
blocks on the inbox until the condition is satisfied (timer fires or
matching event arrives). With `for_event:`, an optional `timeout_ms:`
caps the wait.

**Pitfalls**:
- The field name is `for_event:` (NOT `topic:`)
- Use `duration_ms:` for a timer-only wait, `for_event:` for an event wait,
  or both (event with a timeout)

**When**: rate limiting, polling intervals, waiting for an external event.

---

## assign

Write to `state.Data`. The shared workflow data namespace.

**YAML shape**:
```yaml
- kind: assign
  target: "key"                     # write target (key in state.Data); LITERAL, never an expression
  value: "${results.previous.field}" # Mode B: ${...} to evaluate; bare = literal
```

**Semantics**: `value` is a Mode-B field — wrap in `${...}` to evaluate
against the Scope, otherwise it is written literally. The result is written
to `state.<target>`. Read it back via `state.<key>` in subsequent expressions.

**When**: track loop counters, accumulate results across iterations, store
intermediate computations.

---

## return

Terminate the workflow with an output value.

**YAML shape**:
```yaml
- kind: return
  value: "${results.writer}"      # Mode B: ${...} to evaluate; bare = literal
```

**Semantics**: `value` is a Mode-B field — wrap in `${...}` to evaluate
(it recurses the whole value tree, so `${...}` inside a nested map/list is
evaluated too). The result is returned as `StepResult.Output` (terminal);
the workflow stops, no further steps run. A bare string is returned literally.

**Pitfalls**:
- Place at the END of the top-level sequence (after all real work)
- Do NOT use inside parallel branches — it'll race against siblings
- Without a `return`, the workflow's output defaults to the full `results`
  map at completion

---

## Nested workflows (the `workflow` machine type)

A delegate child can itself be a `type: workflow` — this is how you build
multi-LEVEL trees (a workflow that spawns workflows that spawn leaves):

```yaml
- name: phase-1
  task:
    type: workflow              # the child IS a workflow
    steps:                      # its own primitive tree
      - name: gather
        task: { type: agent, prompt: "..." }
      - kind: return
        value: "${results.gather}"
  mode: coordinator             # mode/dimensions apply to THIS level
```

- **Dry-run pass-through**: under dry-run, `workflow` children are NOT
  substituted by the simulator (only leaf agent/task/program/etc. kinds are),
  so the tree keeps its full STRUCTURE while the leaves become deterministic
  simulators. This is the canonical way to simulate a delegation tree.
- **Mode + policy propagation**: each level's `mode:` applies at that level;
  sticky policies (e.g. coordinator-restricts-children) propagate down to
  grandchildren automatically.
- The nested workflow's `return` value becomes `results.<name>` at the
  parent level, exactly like any other delegate.

### Simulating delegation trees

Nested workflows + simulators are the canonical way to simulate a
long-running, multi-mode delegation tree with ZERO LLM cost:

- Leaves are `type: simulator` tasks (`duration_ms`, `fail_probability`,
  `output`); set `record_context: true` to embed `__spawn_dimensions` in
  each leaf's output — assert per-child modes, permissions, and sticky
  policies (children AND grandchildren) directly from results.
- Depth comes from nested `type: workflow` children; per-level `mode:`
  (`_mode:` inside a nested workflow's config) drives policy propagation.
- Under DRY-RUN, `workflow` children pass through while leaf kinds
  (agent/task/program/…) are auto-substituted by simulators — a real
  workflow keeps its structure with simulated leaves.
- Modeless `task:` steps under a policy-carrying parent ARE restricted
  (the parent's policies apply even without an explicit `mode:`).

Reference: the embedded **demo-delegation-tree** task (ktw-demo) — a
coordinator root with worker/flaky/nested branches, a background delegate
+ join, and `record_context` leaves. $0 to run.

---

## Composition rules

- Any primitive can nest inside any other (sequence inside if inside foreach,
  etc.) — EXCEPT concurrency branches (see below).
- `branches:` in parallel/race is a LIST of nodes, but every branch MUST be a
  `delegate` — non-delegate branch kinds are rejected at parse time; author
  composite branches explicitly as `task: {type: workflow, steps: [...]}`.
- `body:` in loop/while/foreach is a SINGLE node (wrap multiple steps in
  `kind: sequence`)
- `then:` / `else:` in if are SINGLE nodes (wrap multiple steps in
  `kind: sequence`)
- `cases.<value>:` and `default:` in switch are SINGLE nodes (wrap
  multiple steps in `kind: sequence`)
- Expression mode (A bare predicate vs B `${...}` value) is per-field — see
  the **Expression evaluation** section at the top.
