---
name: gocode-test-author-l3
description: >-
  How the tester persona authors L3 (real-LLM end-to-end) test
  scenarios. Reserved for UCs that intrinsically need LLM judgment —
  cost-incurring, slower, less deterministic. Default to L2 unless
  the UC's behavior CANNOT be expressed with the simulator.
icon: "🌐"
iconName: globe
hashtags: [tester, l3, end-to-end, llm, ktw-gocode]
category: runbook
provides: [skill]
---

# L3 End-to-End Scenario Authoring

L3 is the **real-LLM scenario layer**. It runs an agent-loop
machine against a real LLM provider and asserts on the resulting
journal / output. Concepts.md §9.

## When L3 is the right answer

Use L3 ONLY when:

- The UC tests **LLM judgment** itself (model picks correct tool
  per the prompt; model handles ambiguous inputs)
- The UC needs **cross-binary orchestration** that simulators
  can't fake (e.g. agent → real ktw-docs sidecar → real Pandoc)
- The UC involves **prompt engineering invariants** (system
  prompt produces expected response shapes)

For everything else, use L2 with the simulator. L3 burns tokens,
runs slower, and adds non-determinism.

## File location + TCID

```
modules/<m>/tests/L3/uc-<NNN>-<slug>/TASK.md
```

TCID convention:

```
L3-uc<NNN>-<slug>
```

Test-case item record:

```yaml
type: test-case
id: L3-uc150-magentic-end-to-end
body:
  id: L3-uc150-magentic-end-to-end
  layer: L3
  test_ref: modules/<m>/tests/L3/uc-150-magentic-end-to-end/TASK.md
  last_run_status: not-run
```

## The TASK.md shape

L3 scenarios are TASK.md files with `type: agent-loop` + an
input-script that drives the agent + assertion script at the end.

```markdown
---
name: magentic-end-to-end
type: agent-loop
description: UC-150 — Magentic ledger pattern reaches consensus
config:
  max_turns: 30
  llm_profile: claude-sonnet-4-6
---

You are the planner agent in a Magentic team. The team has three
specialists (researcher / coder / reviewer). Your job is to:

1. Read the ledger via `ledger.get`
2. Propose the next action via `ledger.propose`
3. Wait for the specialist response
4. Loop until all DoD checkboxes are checked
5. Call `complete` with a one-line summary

# Acceptance test assertions

When the run completes, the harness asserts:

- run_result.next == "done"
- ledger.completed_checkboxes() == ledger.total_checkboxes()
- journal contains exactly N tool calls to `ledger.propose`
  (where N <= max_turns)
```

## Hard rules

- **Cost discipline**: L3 scenarios MUST set `max_budget_usd` in
  their config to bound LLM spend. Default ceiling: $0.10 per
  scenario.
- **Idempotency**: re-running the same scenario MUST produce a
  pass / fail verdict — even with LLM non-determinism, the
  assertions must be robust to small wording variations.
- **No L3 for negative-DoD**: must-not-happen invariants belong
  in L1 or L2 (deterministic). LLMs can be "almost never" wrong;
  that's not a security guarantee.

## Pattern — robust assertions

Don't assert on exact LLM outputs (brittle). Assert on:

- **Tool-call sequences** — "called X before Y" is robust across
  rewordings
- **State transitions** — "ledger went from in-progress to
  complete" is observable regardless of how the LLM phrased it
- **Schema shape** — "the final output matches `{ summary: string,
  artifacts: []string }`" works regardless of wording

Brittle patterns to avoid:

- "The output contains the exact phrase 'task complete'" — too
  prompt-fragile
- "The model used exactly 5 tool calls" — count variance is
  normal
- "The journal has a specific tool-call order" — unless ordering
  is enforced by the agent loop

## Running

L3 scenarios run via:

```sh
ktw-gocode tasktest run modules/<m>/tests/L3/uc-150-magentic-end-to-end/TASK.md
```

Or via verify, but only when the `--layer` filter includes L3:

```sh
ktw-gocode module-verify --id <module-uuid> --uc UC-150 --layer L3
```

CI runs L1+L2 only by default; L3 runs on schedule or pre-promotion
(when budget allows).

## After authoring

1. Run once locally with a real LLM to confirm the scenario passes
   (budget the run — set `--max-budget-usd 0.10`)
2. Create the test-case item with TCID + layer + test_ref
3. Add the UC to the L3 schedule (see `quality-tooling.md`)
4. Verify will pick it up; status_cache flips to `validated`
   once L1 + L2 + L3 all pass

## References

- `internal/extensions/task/` — agent-loop machine + tasktest harness
- Companion: `gocode-test-author-l1`, `gocode-test-author-l2`
- Concepts.md §9 — coverage state machine
