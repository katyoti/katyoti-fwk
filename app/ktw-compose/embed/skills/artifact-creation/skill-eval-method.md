---
name: skill-eval-method
description: "Choose how to evaluate a skill — a single autonomous run, A/B against a baseline, or a multi-scenario sweep — and when each fits. Load after defining criteria, before running the evaluation."
advertised: false
hashtags: [evaluation, method, skill, create-items]
category: guide
---

# How to evaluate a skill

All methods share one engine — `ktw-compose:/skill/skill-eval-run` (run the
skill autonomously, read the journals, score against
`ktw-compose:/skill/skill-eval-criteria`). The methods differ in how many runs
you do and what you compare.

## The three methods

- **Single autonomous run** — run the skill once on one representative scenario,
  score it. The default. Use when you want a baseline read of whether the skill
  works and is efficient, or for a quick regression check after editing it.
- **A/B against a baseline** — run two variants on the *same* scenario (the
  current skill vs an edit, or vs a sibling that claims the same trigger) and
  compare the scored dimensions head-to-head. Use when the question is "is this
  change better?" or "which of two skills should own this trigger?".
- **Multi-scenario sweep** — run the skill across several scenarios (easy /
  typical / adversarial / edge) and aggregate. Use when one run is not
  representative — the skill branches on input, or you need confidence it holds
  up, not just that it works once.

## Choosing

- One representative scenario, just need a verdict → **single run**.
- Comparing a change or two candidates → **A/B**.
- The skill has distinct input cases, or you need robustness not a spot check →
  **sweep**.
- Combine: a sweep where each scenario is itself an A/B is the strongest
  evidence, at the highest cost — reserve it for a skill you are about to rely on.

Cost scales with run count (each run spends a real budget). Match the method to
the decision: do not sweep when a single run answers the question, and do not
trust a single run when the skill's behaviour varies by input.

## Package the scenario as a runnable task

Whatever the method, write each scenario as a **ktw task** (a flat `<name>.md`
agent task in the workspace's `tasks/` folder): frontmatter pins the
capability and plugins; the body carries the scenario — the goal, the exact
inputs/paths, the utility or skill under test, and the expected deliverable,
ending with "call **complete** with …". The evaluation run is then just
executing that task with tracing on — fully reproducible: re-run it unchanged
after editing the skill and compare scores, hand the same task to an A/B
variant, or clone it per scenario for a sweep. A scenario that lives only in a
chat prompt cannot be re-run; a scenario that is a task IS the harness.

## Then run

Hand the chosen method + scenario tasks to `ktw-compose:/skill/skill-eval-run`.
