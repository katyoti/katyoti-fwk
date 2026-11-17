---
name: evaluate-session
title: Evaluate a session
type: task
advertised: true
description: "Evaluate a finished ktw-agents run from its session journals. Runs the improve-artifact EVALUATE phase (collect metrics → score the run against the criteria → report) against a given session id, using run-quality-evaluation as the scoring engine. Emits a machine-readable evaluation beside the session plus a human-readable report, then completes with a compact verdict. Input: a session id."
iconName: chart-bar
icon: "📊"
hashtags: [evaluation, session, run, improvement, method]
category: development
config:
  # A capable model — judging a run against criteria with evidence is
  # reasoning, not boilerplate.
  capability: smart
  # Journal reads go through `session-read` (BY SESSION ID — no shell, no
  # path-hunting); `workspace` (from standard) writes the evaluation + report
  # files. NO bash: an in-loop eval must not shell out to read journals. NOT
  # augmented — no web/scripting needed; granting them just invites stray calls.
  plugins:
    - standard
    - session-read
  # The scoring engine is advertised:false, so it is loaded by EXACT URI
  # (a `*` selector lists only advertised skills). improve-artifact
  # carries the evaluate-phase method + report shape.
  skills:
    - "ktw-compose:/skill/run-quality-evaluation"
    - "ktw-compose:/skill/improve-artifact"
---

# Evaluate a session

You evaluate ONE finished run — the session `inputs.session_id` — and produce a
comparable evaluation. This is the **evaluate phase** of the improve-artifact
loop (steps 4–6: collect metrics → score → report), run on demand against a run
that already happened. You do not change the subject and you do not re-run it;
you measure what the session already recorded.

Load the method once: `skills-get-instruction` for
`ktw-compose:/skill/improve-artifact` (the evaluate-phase shape + the report
structure) and `ktw-compose:/skill/run-quality-evaluation` (the scoring engine —
how to score the four dimensions with evidence). The engine skill owns the output
contract; follow it exactly. The journal read here is via **`session-read`** (BY
SESSION ID) — never a shell / `ktw-utils` / a constructed path.

## Steps

### 1 — Collect metrics (improve-artifact §4)

Read the run with **`session-read`** (turn-addressed, by id — no shell, no path):

- `session-read(inputs.session_id)` → the OVERVIEW map: `turns`, `system_prompt_chars`,
  the tool **`histogram`** (per-tool counts), `tool_call_total` vs `tools_distinct`
  (redundancy), the **`toolset`** block (`given`/`used`/`never_used` — over-provisioning),
  total duration, and a one-line-per-turn `index`.
- Zoom for evidence: `session-read(id, turns: "<a-b>")` → per-turn `tool_calls`
  (name + input) + `duration_ms` — repeated identical calls = waste, long
  durations = stalls. `tool: <name>` / `grep: <text>` narrow the index first.
- For a **delegation tree**, evaluate each child by ITS OWN session id the same
  way (a child's id is on its `task-status.sessionId`); session-read reads one
  session per call.

Capture tool-call counts + order, redundancy, the catalog-vs-used surface, errors
(turns ending with no tool call), duration, and the produced artifacts (read them
with `file-read` at their known home). If the id resolves to nothing, stop and
report that plainly rather than inventing a verdict.

### 2 — Score the run (improve-artifact §5, via run-quality-evaluation)

Score the four dimensions the engine defines — how the work was done, efficiency,
result quality vs the goal, and achievement — each with the 4-tag severity
(`✓ good / ⚠ acceptable / ✗ wasteful / ✗✗ failed`). **Every verdict cites concrete
evidence** (a turn count, a token number, the specific tool call). Judge
achievement against `inputs.goal` when given; otherwise resolve the goal from the
session as the engine's goal-source reference describes.

### 3 — Write the evaluation + report (improve-artifact §6)

1. Build the machine-readable evaluation to the engine's output contract,
   validate it against that shape, and write it with `file-write` to
   **`workspace_path: test_results/<session_id>.json`** — the dedicated results
   subdir (workspace-relative; `file-write` creates the parent dir, so no shell /
   `mkdir`). Record that as `evaluation_path`. A missing required field or a wrong
   type is a failure; fix it before continuing.
2. Write a human-readable report next to it (`test_results/<session_id>.md`) following improve-artifact §6
   (what was run · the criteria · what was observed · results per dimension ·
   scores · used tools · used resources by type · environment fingerprint) —
   record the path as `report_path`.

This task is the harness's evaluate phase only — it stops at the report. It does
**not** propose or apply changes (§7–§9 of improve-artifact are a separate,
user-gated step). Surface the findings; the user owns what happens next.

### 4 — Complete

Call **complete** with a single-line JSON string matching the output schema —
`achievement`, a one-line `summary`, the `critical_findings` count, and the two
file paths. Example:

```json
{"achievement":"partial","summary":"Goal mostly met but the compose worker retried 3× on a wrong path (harness defect, not subject); 1 wasteful delegation.","critical_findings":2,"evaluation_path":".../evaluation.json","report_path":".../report.md"}
```
