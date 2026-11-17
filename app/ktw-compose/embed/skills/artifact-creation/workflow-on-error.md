---
name: workflow-on-error
description: "Error-handling reference for the ktw-agents workflow DSL: the three on_error forms and the parallel on_branch_error. Load when designing retry, recovery, or fallback in a workflow task."
iconName: alert-triangle
icon: "⚠️"
hashtags: [workflow, on_error, retry, fallback, recovery]
category: orchestration
advertised: false
---

# on_error Reference

Every workflow node can carry an `on_error:` attribute that controls what
happens when the node fails. Default behavior: propagate the failure up the
activation stack until either an `on_error` handles it or the workflow itself
fails.

## Three forms

### Form 1 — string strategy (terse)

```yaml
- name: risky
  task: { type: simulator, fail_probability: 1.0 }
  on_error: continue
```

Strategies (string form):
- `continue` — record failure, run next sibling
- `fail` — propagate (this is the default; rarely written explicitly)

### Form 2 — map with strategy + retries

```yaml
- name: flaky
  task: { type: agent, prompt: "fetch the remote data" }
  on_error:
    strategy: retry
    max_retries: 3
```

Strategies (map form):
- `continue` / `fail` — same as string form
- `retry` — re-run the same step up to `max_retries` times before giving up
- `fallback` — run a separate node instead (see Form 3)

### Form 3 — map with fallback node

```yaml
- name: primary
  task: { type: simulator, fail_probability: 1.0 }
  on_error:
    fallback:
      name: backup
      task:
        type: simulator
        output: { recovered: true, source: "cache" }
```

The `fallback:` value is a complete node (any kind). When `primary` fails,
the fallback node runs in its place. The fallback's output replaces the
failed step's output in `results.<primary-name>`.

When `fallback:` is set, `strategy:` is implicitly `fallback`.

## parallel-specific: on_branch_error

Parallel nodes additionally have `on_branch_error:` (separate from `on_error`):

```yaml
- name: fanout
  kind: parallel
  on_branch_error: fail_fast    # OR collect (default: fail_fast)
  branches: [...]
  on_error: continue            # OPTIONAL — handles whole-parallel failure
```

- `fail_fast` (default): a branch failure cancels all sibling branches and
  the parallel node fails (which then triggers `on_error` if set)
- `collect`: a branch failure is recorded; siblings continue. The parallel
  completes when all branches end (success or failure). Failed branches'
  results are nil in `results.<branch-name>`.

The two attributes compose: `on_branch_error: collect, on_error: continue`
means "run all branches, never fail this node" — useful when partial
results are acceptable.

The loader rejects `on_branch_error` values other than `fail_fast` or
`collect`.

## Worked example: parallel + fallback

From `app/ktw-agents/embed/tasks/demo-parallel-fallback/TASK.md`:

```yaml
- name: fanout
  kind: parallel
  on_branch_error: collect
  branches:
    - name: branch-a-success
      task: { type: simulator, output: { source: "A" } }
    - name: branch-b-success
      task: { type: simulator, output: { source: "B" } }
    - name: branch-c-with-fallback
      task: { type: simulator, fail_probability: 1.0 }
      on_error:
        fallback:
          name: branch-c-fallback
          task: { type: simulator, output: { source: "C-backup" } }
```

Branch C's primary always fails, but `on_error.fallback` rescues it. With
`on_branch_error: collect`, all 3 branches contribute to results.

## Pitfalls

1. **`on_error: continue` only works for sequence-style flow** — inside a
   parallel branch, `continue` doesn't apply (parallel handles it via
   `on_branch_error`). Don't mix the two.
2. **Retry doesn't reset state** — if the failing step modified shared
   state before failing, retry sees the modified state. Keep retried steps
   idempotent.
3. **Fallback runs IN PLACE of the failed node** — its output is what
   `results.<failed-name>` contains. So `results.primary` is actually the
   fallback's output if fallback ran. If you need to know whether fallback
   triggered, include a marker in the fallback's output (e.g., `{recovered:
   true}`).
4. **Nested on_error** — fallback nodes can themselves have `on_error:`
   for chained recovery. But keep it shallow; deeply nested error handling
   is hard to reason about.
5. **No global error handler** — there's no workflow-level `on_error`.
   Wrap the whole workflow's body in a sequence with the last step having
   on_error if you want catch-all behavior, or use `try/catch` patterns at
   the application layer (currently not supported).

## Reading on_error in the source

Loader: `internal/extensions/task/internal/wkflow/loader.go:loadOnError`
Strategy enum: `OnErrorStrategy` constants
Runtime handler: `task_strategy.go:handleNodeError`
