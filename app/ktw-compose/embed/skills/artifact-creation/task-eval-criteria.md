---
name: task-eval-criteria
description: "The grid for judging a task or agent: context efficiency, capability fit, performance, budget, outcome quality, and run hygiene. Load to define 'good' before evaluating a task or agent."
advertised: false
hashtags: [evaluation, criteria, task, agent, create-items]
category: guide
---

# What makes a task/agent good

This grid **is the task's ideal state** (the ISA anchor): an evaluation scores the
actual run against it and reports the gap. A row at ✓/GOOD = that part of the
ideal is met; a ✗/⚠ = a gap to close. The eval concludes with either
`NOTHING TO OPTIMISE` (no gaps) or the clean list of gap-closing work — never a
vague "mostly fine" (see `ktw-compose:/skill/task-evaluation` §4).

Before evaluating, fix what you are measuring. A task/agent is judged on the grid
below. Score each row with the severity rubric, always citing concrete evidence.
The first two dimension groups are **measured** (numbers read from the session,
not opinions); the rest are judged.

## A. Context efficiency (MEASURED — read from the system prompt)

The model's context should carry only what the task needs. Read the actual
`system` string and `tools` array from the session (see
`ktw-compose:/skill/task-eval-run` for the read). Then:

- **A1 — Prompt size vs complexity.** System-prompt tokens against a budget for
  the task's shape. Rough budgets: a single-tool / single-step task ≤ ~4 K tokens
  of system prompt; a multi-step agent ≤ ~8 K; an open chat with broad skills
  ≤ ~12 K. A one-tool task at 48 K is ✗✗.
- **A1b — Body size vs minimum (conciseness).** The AUTHORED body (the task's own
  instructions, separate from plugin-injected blocks + the skills index) should
  be the SMALLEST that still carries the procedure + DoD. MEASURE the body word
  count and estimate the minimum; report `words / est-minimum` and name the excess
  (filler, repetition, restating a tool's own description, over-long examples) — or
  flag UNDER-spec (too thin to drive the task). This is the same conciseness lens
  the skill eval applies to a skill body; a task body is loaded every run, so
  excess is recurring cost. ✓ near minimum; ✗ wasteful when ~2× the minimum.
- **A2 — Unused capability blocks.** Count `## How-to` blocks injected by plugins
  (`## Workspace`, `## Delegation`, `## Templates`, `## External Programs`, …)
  whose tools are **never called** anywhere in the run. Each unused block is dead
  weight. 0 = ✓; each unused block degrades the score.
- **A3 — Skill over-attachment.** Skills listed in `<skills_system>` vs skills
  actually loaded (`get-instructions` called) during the run. Listing ~20 and
  loading 0–1 is ✗ wasteful; the lever is a scoped `skills:` instead of a broad
  bundle.
- **A4 — Contradictory guidance.** Any mutually exclusive instructions in the
  prompt (e.g. a `## Completion` "call complete when done" block AND a
  `## Chat Mode` "there is no completion" block both present). Any contradiction
  is ✗.
- **A5 — Duplicate skill entries.** The same skill URI listed more than once in
  `<skills_system>` (count RAW `<name>` / `- <uri>` occurrences, not deduped —
  do NOT silently collapse duplicates in your reading; that hides the defect).
  Any skill listed >1× is ✗ wasteful (it inflates the prompt and confuses the
  model). 0 duplicates = ✓. Likewise flag any block or section that appears
  more than once (a repeated `<skills_system>`, a doubled capability block).

## B. Capability fit (MEASURED — prompt/body vs catalog vs calls)

The task must declare exactly the capabilities it uses, and every tool it tells
the agent to call must exist.

- **B1 — Instructed tools exist.** Every tool named in the body/prompt as
  "call X" is present in the `tools` array. A body that says "call
  `ktw-weather:weather-service/get-forecast`" with no such tool in the catalog is
  ✗✗ — it is the root of script-fallback.
- **B2 — No fallback flailing.** Tool-call attempts to non-existent tools, or a
  pivot to `scripts-run` / `bash` to do what a missing native tool should have
  done. Any occurrence is ✗ (✗✗ if it caused failure).
- **B3 — Minimal declared surface.** The declared `plugins` / `skills` / `tools`
  are the smallest set that covers the work done. A broad bundle (`augmented`,
  `full`) where a scoped list suffices is ✗ wasteful — it is the cause of A2/A3.
- **B4 — Right capability list.** Skills declared via `skills:`, sidecar methods
  via `tools:`, built-ins via `plugins:` — not crammed into the wrong list.
- **B5 — Toolset appropriateness (MEASURED — `session-read` `toolset` block).**
  The agent should be GIVEN the tools the work needs and few it does not. Read the
  overview's `toolset: {given, used, never_used}` (catalog vs invoked, computed
  deterministically — no judgment). Apply the policy:
  - **Over-provisioning** — `never_used / given`. A bloated catalog (e.g. 55 given /
    7 used → 48 never used) is ✗ wasteful: the lever is a scoped `plugins:` / `tools:`
    instead of a broad bundle (this is the surface-level cause of A2/A3).
  - **Shell/escape tool in an authoring or driver run** — `bash` / `scripts-run`
    PRESENT in the catalog of a run whose job is authoring files or driving a child
    is ⚠ (it shouldn't be reachable); USED to author files or path-hunt
    (`ls`/`cat`/`find` to explore the repo) is ✗✗ — the agent shelled out instead
    of using its proper tools. Read the offending calls from `histogram` + a
    `tool: bash` zoom.
  - "Needed" (did it have everything?) stays the judged half (B1 covers the missing
    side); "given vs used" is the measured half the `toolset` block hands over.

## C. Performance & budget (judged — reuse run-quality-evaluation)

- **C1 — Turns and tool calls** to reach the result; fewer is better, repeated
  identical calls or re-asking known data is waste.
- **C2 — Tokens and USD** vs the thresholds in
  `ktw-compose:/reference/artifact/eval/budgets.md`.

## D. Outcome quality vs purpose (judged — the anchor)

- **D1 — Goal achieved**, deliverables present, no fabricated data.
- **D2 — Completion correctness** — `complete` called appropriately; in
  `mode: chat`, NO spurious complete attempt (the tool is filtered).

## E. Run hygiene (judged)

- **E1 — Errors** — error count, abandoned turns, turns ending with no tool call.
- **E2 — Mode coherence** — the declared `mode` matches the observed loop
  behavior (chat loops back via ask-user; worker drives to complete).

## Severity rubric

Tag every judgement with exactly one (catalog of per-category examples:
`ktw-compose:/reference/artifact/eval/run-rubric.md`):

| Tag | Meaning |
|---|---|
| ✓ good | as intended, within budget, clean outcome |
| ⚠ acceptable | minor friction (one retry, slight over-fetch, one unused block) |
| ✗ wasteful | significant inefficiency (oversized prompt, many unused blocks, broad bundle, script fallback that recovered) |
| ✗✗ failed | did not achieve its goal, OR an instructed tool was absent, OR a contradiction broke the run |

## Evidence rule

Every verdict MUST cite concrete data — token counts, the exact unused block
names, the skills-listed-vs-loaded numbers, the specific missing tool URI, the
turn where the fallback happened. "The prompt was big" is not an evaluation;
"✗✗ A1 — 48,924-token system prompt for a 1-tool task (budget ~4 K); A2 — 4
unused blocks (Workspace, Delegation, Templates, External Programs); B1 —
`ktw-weather:weather-service/get-forecast` named in body, absent from the 0-entry
weather tools, agent pivoted to `scripts-run` at turn 3" is.

## The anchor

When dimensions conflict, **outcome quality vs purpose wins** — but a task that
produced the right thing through a 48 K prompt and a lucky script fallback is
still ✗ on context efficiency and capability fit. Efficiency defects are real
defects even when the run happened to succeed.
