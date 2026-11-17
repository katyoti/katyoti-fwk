---
name: skill-eval-run
description: "Run a skill autonomously with a simulated user, then score it against the criteria from its session journals. The evaluation method's engine; load to actually evaluate a skill."
advertised: false
hashtags: [evaluation, autonomous, simulation, create-items]
category: guide
---

# Running a skill autonomously to score it

This is the loop every evaluation method runs. It produces a scored verdict for
one (skill, scenario) pair with **no human in the loop** — the user's side is
simulated. Repeat it per scenario / per variant for the A/B and sweep methods
(`ktw-compose:/skill/skill-eval-method`).

## 1. Build the scenario

A scenario is: the **goal/prompt** that should trigger the skill, plus a
**simulated user** — a pre-scripted transcript of answers the skill's `ask-user`
prompts will receive. Write the answers a realistic user would give; for an
adversarial scenario, give terse or off-topic ones. Record the goal explicitly
so scoring has a fixed target (it is also resolvable via
`ktw-compose:/reference/artifact/eval/goal-source.md`).

Package the scenario as a **ktw task** (a flat agent-task file in the
workspace's `tasks/` folder) per `ktw-compose:/skill/skill-eval-method` — the
run is then just executing that task with tracing on, and the same task
re-runs unchanged for regression, A/B, or sweep.

## 2. Run it autonomously

Drive a run that loads the skill, feeding the scripted answers wherever it would
otherwise block on the user (the HumanGate / `ask-user` is satisfied from the
transcript, not a live prompt). The run proceeds to completion unattended and
produces a **session** — capture its id. A run that blocks waiting for a real
answer the transcript did not cover is itself a finding (the skill asked an
unanticipated question).

## 3. Read what happened

The run left full journals. Evaluate them with
`ktw-compose:/skill/run-quality-evaluation` against the session id — it walks the
tree, aggregates, and per-agent detail and returns the structured verdict.

## 4. Score against the skill's criteria

Map the run evaluation onto the six skill dimensions in
`ktw-compose:/skill/skill-eval-criteria` — performance, budget, time, user UX,
and **output quality vs the skill's stated purpose**. The simulated-user angle
makes UX measurable: were the questions the skill asked the right ones, in a
reasonable number of rounds, or did it over-interview / ask for things the
transcript already implied?

## 5. Emit the verdict

Produce the scored evaluation to
`ktw-compose:/reference/artifact/eval/output-schema.md`, tagged per dimension
with the severity rubric and each verdict carrying its evidence. For A/B or
sweep, emit one per run plus the comparison/aggregate the method asked for.

## What "autonomous" buys

Because the user is scripted, the same scenario is repeatable — you can re-run
after editing the skill and compare, or sweep many scenarios, without a human
re-answering each time. That repeatability is the whole point: it turns "does
this skill feel good" into a measured, re-runnable score.
