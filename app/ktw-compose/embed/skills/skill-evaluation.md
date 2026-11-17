---
name: skill-evaluation
description: "Evaluate how good and efficient a skill is by running it autonomously and inspecting the journals. Load when asked to evaluate, score, benchmark, or A/B a skill, or to check one after editing it."
advertised: true
hashtags: [evaluation, skill, create-items]
category: guide
---

# Evaluating a skill

You measure a skill instead of guessing about it: run it autonomously against a
simulated user, read the journals, and score it. Five phases, in order. The
deliverable is ONE evaluation document that resumes the run — what was run,
the criteria, what was observed, the results and scores, and a prioritized
improvement list the user decides on item by item.

This is the skill-specific instance of the generic improvement loop. To
iterate — apply validated changes, re-run, compare against the previous
report — follow `ktw-compose:/skill/improve-artifact`; this skill is
its steps 1–8 specialized for skills.

## 1. Define the criteria

Load `ktw-compose:/skill/skill-eval-criteria` — fix what "good" means for this
skill across the six dimensions (triggering accuracy, performance, budget, time, user UX, output
quality vs the skill's stated purpose) and the severity rubric. The skill's own
`description` is the anchor: output quality is judged against what it promised.

## 2. Pick the method

Load `ktw-compose:/skill/skill-eval-method` — choose a single autonomous run, an
A/B against a baseline, or a multi-scenario sweep, and write the scenario(s):
the triggering goal plus the simulated user's scripted answers. Match the method
to the decision; do not sweep when one run answers the question.

## 3. Run and score

Load `ktw-compose:/skill/skill-eval-run` for each (skill, scenario): drive the
skill to completion with the scripted user, capture the session, evaluate it via
`ktw-compose:/skill/run-quality-evaluation`, and score against the criteria.
Emit the scored verdict (per dimension, severity-tagged, with evidence).

## 4. Pareto report — propose the high-leverage improvements

From the scores and the run's log files, identify the N improvements with the
highest leverage per the Pareto principle: the ~20% of effort that yields ~80%
of the impact. For each improvement provide:

- **Short title + category** (perf / quality / security / architecture)
- **Observed problem** — what concretely goes wrong, with its precise location
  (the skill section, the turn in the journal, the tool call, the file)
- **Expected impact** — quantified where possible (turns saved, tokens,
  latency, output-contract conformance, risk removed)
- **Estimated effort** — low / medium / high
- **Regression risk** — low / medium / high
- **Proposed approach** — a technical description of the fix, WITHOUT touching
  anything yet

Sort the list by impact/effort ratio, descending. Every item must trace back
to evidence from phase 3 — no speculative improvements.

## 5. Selection — the user decides, item by item

Assemble the evaluation document and present it. It resumes the run:

1. **What was run** — the skill, the scenario task, the method, the budget
2. **The criteria** — the dimensions and rubric fixed in phase 1
3. **What was observed** — the run's factual behavior, with evidence
4. **Results** — good / bad per dimension, severity-tagged
5. **Scores** — the phase-3 verdict
6. **The Pareto improvement list** — from phase 4

Then ask the user EXPLICITLY:

- Which improvements to apply (numbers, all, or a subset)
- Whether some must be treated first or in a specific order
- Which constraints apply (compatibility, branches, files in production, …)

Record the decision per improvement — **do** / **planned** / **will not be
done** — in the document. Do not start applying anything before the user has
answered; the report is the gate.

## Done

A skill evaluation is done when every dimension has a severity tag backed by
concrete evidence from the run, the evaluation document resumes the run
(what ran, criteria, observations, results, scores, improvements), and every
proposed improvement carries the user's decision (do / planned / will not be
done). A verdict without evidence is not an evaluation; a report without a
decision is not finished.
