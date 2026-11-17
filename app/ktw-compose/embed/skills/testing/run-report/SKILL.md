---
name: run-report
description: "Write a readable markdown report of a finished ktw-agents run: its delegation graph and what each agent did and spent. Load when asked how a run went or why it was slow."
advertised: false
hashtags: [testing, report, run, journals, evaluation]
category: guide
---

# run-report — Human-Readable Run Reports

## What this skill does

Produces a markdown report `report.md` in the run's session folder (the universe's sessions folder: `ktw-agents dirs`, `{sessions}/{session-id}/`) that lets a human (or another LLM) review what happened during a ktw-agents run. The report covers the full delegation tree (parent + all children) with per-agent metrics, tool usage, skill usage, and qualitative LLM-generated efficiency commentary.


## Workflow

### Step 1: Resolve the session id

If the user provided a session id, use it directly. Otherwise, list sessions and pick the most recent top-level run:

```bash
./bin/ktw-utils.exe session tree --json
```

This returns the most recent root session with no parent. Use its `sessionId`.

### Step 2: Build the delegation tree

```bash
./bin/ktw-utils.exe session tree <session-id> --json
```

The output is a recursive tree of `DelegationNode` objects. Each node has: `runId`, `sessionId`, `dir`, `task`, `mode`, `model`, `provider`, `parentRunId`, `startedAt`, `children`.

Read [references/data-shape.md](references/data-shape.md) for the full schema.

### Step 3: Aggregate metrics across the tree

```bash
./bin/ktw-utils.exe session aggregate <session-id> --json
```

Returns `AggregateMetrics` with tree-wide totals plus a `perAgent` array. Tools by name, skills by agent, tokens, durations, errors — all summed.

### Step 4: Get per-agent details

For each agent in the tree, fetch its turn-by-turn details:

```bash
./bin/ktw-utils.exe session show <session-id> --json
```

This returns per-turn user_text, assist_text, tool_calls (name + input + result), tokens, duration. You'll use this to fill the per-agent sections of the report.

### Step 5: Generate the report

Read [references/report-template.md](references/report-template.md) for the markdown skeleton with all 8 required sections. Fill it in using the data from steps 2-4.

For each delegation/tool/skill usage, generate an LLM evaluation following the rubric in [references/evaluation-rubric.md](references/evaluation-rubric.md). Each evaluation must:
- Use a severity tag: ✓ good / ⚠ acceptable / ✗ wasteful / ✗✗ failed
- Cite specific evidence (turn number, tool call id, token count, etc.)
- Be 1-2 sentences max — no fluff

### Step 6: Write the report

Write the final markdown to:
```
{sessions}/{root-session-id}/report.md
```

Confirm to the user with the path and a 1-line summary of the most notable finding (e.g., "Report written. The plan-builder retried 3 times — possibly worth tightening its prompt.").

## Required tools

This skill uses: **Bash** (for ktw-utils calls), **Read** (for references), **Write** (for the final report).

## Companion skill

The machine-readable counterpart is `run-quality-evaluation` (separate skill). Both consume the same data sources but produce different output formats. This skill is for humans; that one is for LLMs that need structured JSON analysis.

The machine-readable companion is `ktw-compose:/skill/run-quality-evaluation`.
