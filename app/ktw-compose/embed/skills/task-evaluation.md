---
name: task-evaluation
description: "Evaluate a task or agent by running it and inspecting the session for context efficiency, capability fit, and outcome quality. Load to score a task/agent, or after a bloated or tool-not-found run."
advertised: true
hashtags: [evaluation, task, agent, create-items]
category: guide
---

# Evaluating a task or agent

You measure a task/agent instead of guessing about it: run it (or take a finished
session), read the session — including the **actual system prompt and tools array
sent to the model** — and score it. Five phases, in order. The deliverable is ONE
evaluation document that resumes the run: what was run, the criteria, what was
observed, the results and scores, and a prioritized improvement list the user
decides on item by item.

This is the task/agent instance of the generic improvement loop (the skill
version is `ktw-compose:/skill/skill-evaluation`). What makes THIS evaluation
different from a skill eval: a task/agent owns its **capability surface**
(`plugins` / `skills` / `tools`) and its **system prompt**, so two dimensions are
first-class here that a skill eval ignores — **context efficiency** and
**capability fit**. These are exactly the defects that ship silently: a 48 KB
prompt for a one-tool task, or a body that says "call get-forecast" when no such
tool is in the catalog (the model then flails into `scripts-run`/`bash`).

To iterate — apply validated changes, re-run, compare against the previous
report — follow `ktw-compose:/skill/improve-artifact`.

## 1. Define the criteria

Load `ktw-compose:/skill/task-eval-criteria` — fix what "good" means across the
dimensions (context efficiency, capability fit, performance, budget, outcome
quality vs the task's `description`, run hygiene) and the severity rubric. The
task's own `description` is the anchor for outcome quality.

## 2. Pick the method

Load `ktw-compose:/skill/skill-eval-method` — the method choice (single run,
A/B vs a baseline variant, multi-scenario sweep) and scenario authoring are the
same as for skills. Match the method to the decision; do not sweep when one run
answers the question.

## 3. Run and score

Load `ktw-compose:/skill/task-eval-run` for each (task, scenario). **First decide
who runs it** — the test is conditioned by GUI-TOOL usage, NOT input: a headless
task (its goal does not require mounting live UI) the evaluator runs itself
autonomously via `ktw-agents run <name> --input '<json>'` — required input is
just a JSON value, no human needed. Only a **GUI-required task** (its deliverable
is a live-mounted UI component, or a GUI chat) must be run by the user in ktw-gui;
ask for the session id and evaluate from that captured session. Then capture the
session and score against the criteria. The run skill carries the **mechanical
reads** — pull the system prompt + tools array from the session's
`llm-model-calls.jsonl` and compute the context-efficiency + capability-fit
metrics directly (these are measured, not judged). For the outcome dimensions,
reuse `ktw-compose:/skill/run-quality-evaluation`. Emit the scored verdict (per
dimension, severity-tagged, with evidence).

## 4. Conclusion — the ISA gap report (MANDATORY shape)

The criteria (phase 1) ARE the task's ideal state. An evaluation is an ISA
completeness check: score the actual run against that ideal and **conclude with
exactly one of two outcomes** — never a vague "looks mostly fine":

- **`NOTHING TO OPTIMISE`** — every grid dimension is ✓/GOOD (no ✗, no ⚠ worth
  acting on). State it plainly; the task already matches its ideal state.
- **a clean gap list** — the concrete, actionable items that close the gap to the
  ideal. Each item is a unit of work, not an observation. Order by leverage
  (the ~20% of effort yielding ~80% of impact). For each:

  - **Short title + category** (context / capability / perf / quality / hygiene)
  - **Observed problem** — what concretely goes wrong, with its precise location
    (the frontmatter field, the prompt block, the turn in the journal, the tool call)
  - **Expected impact** — quantified where possible (prompt tokens removed, tools
    added, turns saved, script-fallback eliminated)
  - **Estimated effort** — low / medium / high · **Regression risk** — low / med / high
  - **Proposed approach** — a technical description of the fix, WITHOUT touching
    anything yet

Every gap item must trace back to evidence from phase 3 — no speculative items.
A ⚠ that isn't worth fixing is stated as such and excluded from the list (so the
list is purely "what must be done"), not left as ambiguous noise.

- **Short title + category** (context / capability / perf / quality / hygiene)
- **Observed problem** — what concretely goes wrong, with its precise location
  (the frontmatter field, the prompt block, the turn in the journal, the tool
  call)
- **Expected impact** — quantified where possible (prompt tokens removed, tools
  added, turns saved, script-fallback eliminated)
- **Estimated effort** — low / medium / high
- **Regression risk** — low / medium / high
- **Proposed approach** — a technical description of the fix, WITHOUT touching
  anything yet

Sort by impact/effort, descending. Every item must trace back to evidence from
phase 3 — no speculative improvements.

## 5. Selection — the user decides, item by item

Assemble the evaluation document and present it. It resumes the run: what was run,
the criteria (the ideal state), what was observed (with evidence), results per
dimension (severity-tagged), and the phase-4 conclusion — `NOTHING TO OPTIMISE`
or the gap list. If `NOTHING TO OPTIMISE`, there is nothing to decide; close.
Otherwise ask the user explicitly which gap items to apply, in what order, under
which constraints. Record the decision per item (**do** / **planned** / **will
not be done**). Do not start applying anything before the user has answered; the
report is the gate.

## Done

A task/agent evaluation is done when every dimension has a severity tag backed by
concrete evidence from the run, the context-efficiency and capability-fit metrics
are measured numbers (not estimates), and the evaluation **concludes with exactly
one of `NOTHING TO OPTIMISE` or a clean gap list** (the ISA outcome — no vague
"mostly fine"). A verdict without evidence is not an evaluation; an ambiguous
conclusion is not finished; a non-empty gap list without the user's per-item
decision is not finished.
