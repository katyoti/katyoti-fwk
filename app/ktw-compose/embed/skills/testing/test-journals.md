---
name: test-journals
description: "Inspect a past run from its session journals (LLM, user, and terminal views plus traces) to answer what happened and score it. Load for post-run debugging or scoring without re-running."
advertised: false
hashtags: [testing, journals, sessions, evidence]
category: guide
---

# Session journals

Every run already produced its evidence; this method just reads it. Free.

## Where

Each run creates a session directory under the data dir's sessions root:

```
<sessions>/<timestamp>_<task>/
  llm.jsonl        # LLM perspective: requests, responses, tool calls/results
  user.jsonl       # user perspective: prompts, inputs, messages
  terminal.jsonl   # terminal protocol events
```

Entries share a monotonic `seq` — correlate across journals by it.

## Read

```bash
ls -lt <sessions>/ | head -5                       # most recent runs
grep '"type":"tool_call"' <session>/llm.jsonl      # what was called
grep '"type":"tool_result"' <session>/llm.jsonl    # what came back
```

`ktw-utils session tree / aggregate / show` walk multi-agent runs: the tree
of children, per-agent aggregates, and per-agent detail.

## What it answers

- which tools/skills/references a run actually used (vs what was advertised)
- where a run went wrong (the first error/retry, the turn it derailed)
- budgets: tokens, turns, wall-clock per agent
- scoring input for `ktw-compose:/skill/run-quality-evaluation`

## Rules

- Journals are after-the-fact: if the question needs new behavior, run with
  `ktw-compose:/skill/test-run-trace` instead.
- Never clean sessions while an evaluation references them — the journal IS
  the evidence.
