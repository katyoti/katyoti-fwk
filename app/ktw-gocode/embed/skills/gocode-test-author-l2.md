---
name: gocode-test-author-l2
description: >-
  How the tester persona authors L2 `.tasktest` simulator scenarios.
  Token-free, deterministic; preferred over L3 for behavioral tests
  that can be expressed via the simulator strategy.
icon: "🧫"
iconName: flask
hashtags: [tester, l2, tasktest, simulator, ktw-gocode]
category: runbook
provides: [skill]
---

# L2 Tasktest Authoring

L2 is the **simulator-based scenario layer**. Use it instead of
real-LLM L3 whenever the UC's behavior can be expressed as a
deterministic Machine execution. Concepts.md §9.

## What L2 verifies

- Workflow-level behaviors (spawn, suspend, resume, cancel
  cascade, parallel + race, on_error)
- Per-spawn dimensions in composition (e.g. coordinator restricts
  worker permissions)
- Event-bus topic routing
- Anything that needs to exercise the Machine engine without
  hitting a real LLM

**Not in scope for L2**: real LLM behavior (prompt-conditioned
outputs, model-specific tool-call ordering) — that's L3.

## File location + TCID

```
modules/<m>/tests/L2/uc-<NNN>-<slug>.tasktest
```

TCID convention (concepts.md §9 / `coverage/matcher.go`):

```
L2-uc<NNN>-<slug>
```

Test-case item record:

```yaml
type: test-case
id: L2-uc053-spawn-resume
body:
  id: L2-uc053-spawn-resume
  layer: L2
  test_ref: modules/tasks/tests/L2/uc-053-spawn-resume.tasktest
  last_run_status: not-run
```

## The `.tasktest` shape

A `.tasktest` is YAML describing:

1. **Setup** — the simulator workflow to run
2. **Expectations** — journal assertions to make after the run

Skeleton:

```yaml
# tests/L2/uc-053-spawn-resume.tasktest
name: spawn-resume-roundtrip
description: |
  UC-053: Spawner.Spawn → child.<name>.done → parent resumes
  with child's output. Uses simulator children to avoid LLM cost.

workflow:
  type: workflow
  config:
    steps:
      - spawn:
          name: worker
          task_type: simulator
          input:
            duration_ms: 50
            output: { tag: "from-worker" }
      - return:
          value: ${{ results.worker.output.tag }}

expectations:
  workflow_done: true
  workflow_output:
    equals: "from-worker"
  journal:
    contains_topics:
      - child.worker.started
      - child.worker.done
      - workflow.${runID}.done
    forbidden_topics:
      - workflow.${runID}.failed
```

## Patterns by UC family

### Spawn / resume

Use a simulator task with `duration_ms` to ensure the parent waits.
Assert that `child.<name>.done` reaches the parent's inbox and the
parent's next step sees the child's output.

### Cancel cascade

Spawn N simulator workers, one configured with
`fail_probability: 1.0`, then assert siblings receive
`to.<name>.cancel` topic emissions when `on_error: fail_fast`.

### Permission gating (security UCs — negative DoD required)

Use the simulator's `request_tools` field (when wired) or run an
agent-loop machine with the SafeToolSubset + assert the journal
does NOT contain disallowed tool calls.

### Per-spawn dimensions

Spawn children with explicit `mode` (worker / coordinator / etc.)
and assert the resulting child config matches the preset.

## Hard rules

- **Simulator by default**. Only use `type: agent-loop` with a
  real LLM when the UC intrinsically needs LLM judgment (then it
  belongs in L3, not L2).
- **Deterministic outputs**. Avoid `duration_ms: 0` or
  `fail_probability: 0.5` — pick a single concrete outcome per
  scenario.
- **Use the `${runID}` macro** for journal-topic assertions that
  reference the workflow's own id.
- **Don't depend on wall-clock timing**. Use small `duration_ms`
  values (50ms) only as race-condition test scaffolding, not as
  load tests.

## Negative DoD scenarios

For security UCs, write a scenario asserting the FORBIDDEN
behavior never happens:

```yaml
expectations:
  journal:
    forbidden_topics:
      - tool.bash.invoked   # MUST NOT happen when bash is disallowed
      - permission.granted: { tool: bash }
```

Mark the test-case item `negative: true`.

## Running

```sh
ktw-gocode tasktest run modules/<m>/tests/L2/uc-053-spawn-resume.tasktest
```

Or via verify:

```sh
ktw-gocode module-verify --id <module-uuid> --uc UC-053 --layer L2
```

Verify aggregates: passing L2 + (any) L1 → UC `covered` →
`status_cache: covered`.

## After authoring

1. Run locally via `tasktest run` — must pass deterministically
2. Create the test-case item with the TCID + layer + test_ref
3. Trigger `module-verify` to update UC status_cache + write a
   test-run item

## References

- `internal/extensions/simulator/task_strategy.go` — simulator
  config (DurationMs / Output / FailProbability / FailMessage)
- `modules/tasks/tests/L2/` — worked examples
- Concepts.md §9 — coverage state machine
