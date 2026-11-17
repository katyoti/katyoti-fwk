---
name: run-quality-evaluation
description: "Score a finished ktw-agents run from its session journals and emit a machine-readable evaluation.json. Load when you have a session id and need a structured verdict, not just raw metrics."
advertised: false
hashtags: [evaluation, run, journals, create-items]
category: guide
---

# Evaluating a run

Given a finished run (a root session id), read its journals and produce a
machine-readable evaluation. This is the engine the autonomous skill evaluator
(`ktw-compose:/skill/skill-eval-run`) calls after it has produced a run; it also
stands alone for evaluating any run (a goal-planner, a delegation tree).

The judging rubric is `ktw-compose:/reference/artifact/eval/run-rubric.md`; the
performance thresholds are `ktw-compose:/reference/artifact/eval/budgets.md`.

## Inputs

- **session id** — the root session id. If absent, take the most recent root
  session from the session tree.
- **goal** — the original goal, to judge achievement against. If not given
  explicitly, resolve it via `ktw-compose:/reference/artifact/eval/goal-source.md`.

## Read the journals

Three reads cover the whole tree — their exact JSON shapes are
`ktw-compose:/reference/artifact/eval/session-shapes.md`:

1. **the delegation tree** — every agent in the run, parent→child, with per-node
   turn counts and token usage.
2. **tree-wide aggregates** — totals + `toolsByName` + `errorCount`.
3. **per-agent detail** — the full LLM journal + tool calls for each node you
   need to judge closely (the ones the aggregates flag as expensive or failed).

## Judge

Score four dimensions, each with the 4-tag severity (`✓ good / ⚠ acceptable /
✗ wasteful / ✗✗ failed`) from the rubric, and **every verdict cites concrete
evidence** (turn counts, token numbers, the specific tool call):

- **how the work was done** — per-delegation: did the child's result match the
  parent's request, was the turn count reasonable, did it retry, did it ignore
  its input?
- **efficiency** — per-tool: was each tool's count appropriate, were there
  errors, was input/output proportional? Compare against the budgets.
- **result quality vs the goal** — does the final output satisfy the goal?
- **achievement** — full / partial / none, with the gap named.

## Emit

Build the evaluation document to the contract in
`ktw-compose:/reference/artifact/eval/output-schema.md`, validate it against that
shape, then write it beside the session and echo it. A missing required field or
a wrong type is a failure — fix it before declaring the evaluation done.

## References

- [data-shape.md](references/data-shape.md) — what `ktw-utils session tree / aggregate / show` return, field by field (and their camelCase / snake_case quirk).
- [evaluation-rubric.md](references/evaluation-rubric.md) — how each dimension is scored.
- [output-schema.md](references/output-schema.md) — the `evaluation.json` schema; [example-evaluation.json](references/example-evaluation.json) a filled one.
- [goal-source-fallback.md](references/goal-source-fallback.md) — where the goal comes from when none is given.
- [performance-budgets.md](references/performance-budgets.md) — the thresholds efficiency is judged against.
- [ms08-coverage.md](references/ms08-coverage.md) — extracting the multi-agent wasteful patterns the rubric names.

The human-readable companion is `ktw-compose:/skill/run-report`.
