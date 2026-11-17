---
name: improve-artifact
description: "Measured, user-gated improvement loop for an artifact; load to evaluate, propose, apply only approved changes, then re-run and compare — iterating a skill/task/agent/prompt to a quality threshold."
advertised: true
hashtags: [evaluation, improvement, loop, method]
category: guide
---

# Artifact improvement

Improvement is a measured loop, not a one-shot edit. Every iteration produces
a comparable report; **no change is applied without explicit user
validation**. Nine steps — 1–3 set up the harness once, 4–9 are the loop.

```
1 define → 2 make runnable → 3 build eval task
        ┌──────────────────────────────────────┐
        ▼                                      │
4 run + metrics → 5 evaluate → 6 report →      │
7 propose → 8 user decides → 9 apply validated ┘ (compare vs previous)
```

## 1. Define what to evaluate

Name the **subject** (a task, a skill, an agent, a workflow, a prompt, a CLI
surface…), its **stated purpose** (the anchor every score is judged against),
and the **criteria**: the dimensions that matter (performance, budget, time,
user UX, output quality vs purpose), the severity rubric, and the evidence
rule — every verdict must cite something observable. For a skill, the
dimensions are already fixed in `ktw-compose:/skill/skill-eval-criteria`;
for other artifacts, adapt the same dimensions to the subject.

Fix the **stop conditions** now, not later: the acceptance targets per
dimension ("done when output quality ≥ …, no critical findings, ≤ N turns"),
a per-iteration budget, and a max iteration count. A loop without targets
ends on fatigue, not on done.

## 2. Define what must run to make it evaluable

The subject is only measurable inside a run. Decide the **scenario**: the
triggering goal, the exact inputs (paths, data, utilities available), the
simulated user answers if the subject asks questions, and the expected
deliverable. Decide the **method** — single run, A/B against a baseline, or
multi-scenario sweep (`ktw-compose:/skill/skill-eval-method` carries the
choice criteria; they apply to any artifact, not just skills).

## 3. Build the evaluation task

Package the scenario as a **ktw task** (a flat agent-task file in the
workspace's `tasks/` folder): frontmatter pins capability and plugins; the
body carries the scenario and ends with "call **complete** with …". The task
IS the harness — it re-runs unchanged every iteration, which is what makes
reports comparable. A scenario that lives only in a chat prompt cannot loop.

## 4. Run and collect metrics

Execute the evaluation task with tracing on. Capture: the session id, the
trace/journals, turn count, tool-call counts and order, errors and retries,
token usage, wall-clock, and the produced artifacts. Do not clean anything up
— the logs are the evidence base.

**On the first iteration, validate the harness before trusting the
baseline**: did the run fail or detour because of the evaluation task itself
(a wrong path, a missing input, an unreachable utility) rather than the
subject? A harness defect is fixed in the task and the baseline re-run — the
loop must never "improve" the subject to compensate for a broken harness.

**LLM subjects are stochastic** — one run is an anecdote, not a measurement.
When the budget allows, run the scenario 2–3 times per iteration and score
the set; when it does not, treat single-run deltas as provisional and say so
in the report. Never credit a change for what one run's variance can explain.

## 5. Evaluate

Score the run against the phase-1 criteria from the metrics, logs, and the
deliverable's quality (for agent runs,
`ktw-compose:/skill/run-quality-evaluation` walks the session and returns the
structured verdict). Tag each dimension with a severity and its evidence.

## 6. Build the report

One document that resumes the iteration:

1. **What was run** — subject, evaluation task, method, iteration number
2. **The criteria** — dimensions + rubric from step 1
3. **What was observed** — factual run behavior, with evidence
4. **Results** — good / bad per dimension, severity-tagged
5. **Scores** — the step-5 verdict
6. **Used tools** — every tool the run called, with call counts
7. **Used resources, by type** — every skill / reference / task / config the
   run loaded, grouped by resource type
8. **Environment fingerprint** — binary versions/build times, LLM
   capability/profile/model, catalog state. Without it, a delta may measure
   platform drift, not the change.
9. *(iteration ≥ 2)* **Delta vs previous report** — per dimension and per
   metric: improved / unchanged / regressed — only comparable when the
   fingerprints match; flag any drift explicitly

Items 6–7 come straight from the trace; they expose dead weight (resources
advertised but never loaded) and gaps (content the run needed but could not
reach) — both are improvement candidates.

## 7. Propose changes — Pareto

From the scores and logs, identify the N highest-leverage improvements (~20%
of effort for ~80% of impact). For each: short title + category
(perf / quality / security / architecture), observed problem with precise
location, expected impact (quantified where possible), estimated effort
(low / medium / high), regression risk (low / medium / high), and the
proposed approach — described technically, **nothing touched yet**. Sort by
impact/effort descending. Every item traces to step-6 evidence.

## 8. The user decides — the hard gate

Present the report and ask explicitly:

- Which improvements to apply (numbers, all, or a subset)
- Whether some must come first or in a specific order
- Which constraints apply (compatibility, branches, files in production, …)

Record the decision on every item — **do** / **planned** / **will not be
done** — in the report. **No change without user validation.** The report
with decisions is the iteration's final artifact. A **planned** item needs a
home (a task or a backlog entry, named in the report) — otherwise it
evaporates between iterations.

## 9. Apply and loop

Apply ONLY the items marked **do**, exactly as approved. Mind attribution:
when several changes land in one iteration, the next delta cannot tell which
one moved the score — a regression can hide behind an improvement. Batch
low-risk items freely; apply a **high-regression-risk item in its own
iteration** so its effect is measurable in isolation. For a subject other
consumers depend on (a shared skill, a shared reference), name the blast
radius in the proposal and, when it matters, add their scenarios to the run.

Then loop to step 4: re-run the SAME evaluation task, produce the next
report, and fill its delta section against the previous one — improvements
must show up in the measurements, and regressions must be caught, not
assumed. Stop when the step-1 acceptance targets are met, when the user says
stop, when the max iteration count is reached, or when an iteration yields no
validated items.

For an **embedded** subject whose sidecar source is in this workspace, run the
whole loop on the overlay so each re-run sees the edit with no rebuild: apply
into `.katyoti/overwrites/<sidecar>/…`, re-evaluate live, and `promote` only the
final validated version into source — `ktw-compose:/skill/overlay-iteration`.

## Rules

- The evaluation task never changes mid-loop (else reports stop being
  comparable). If the scenario itself must change, that starts a new loop at
  step 1.
- Keep every iteration's report — the sequence is the improvement history.
- Evidence first: no proposal without a measurement behind it, no verdict
  without a citation into the logs.
- The user owns the changes; you own the measurements.
