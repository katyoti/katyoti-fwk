---
name: test-dry-run
description: "Validate a task or workflow's topology and event flow at zero LLM cost, replacing every child with a deterministic simulator. Load before paying tokens for a new task or to smoke-test orchestration."
advertised: false
hashtags: [testing, dry-run, workflow, topology]
category: guide
---

# Dry-run

The orchestration is exercised end-to-end; the children are simulated.

## Run

```bash
./bin/ktw-agents.exe run <task-or-uri> --dry-run --dry-trace events.jsonl
```

- Forces pipe mode; prints a topology report at the end.
- `--dry-trace` captures every workflow/child event as JSONL for comparison
  or programmatic assertion.

## What it answers

- the task file parses and dispatches (frontmatter, config block, type)
- workflow nodes spawn/sequence/return as declared; on-error paths trigger
- child lifecycle events (`child.*.started/done/failed`) flow as expected
- a refactor of the engine/orchestration didn't change the event topology
  (diff two `--dry-trace` files)

## What it cannot answer

No real LLM, no real tool effects, no real outputs — behavior and output
quality need `ktw-compose:/skill/test-run-trace`. A green dry-run plus a bad
prompt is still a bad task.

## Rules

- Always dry-run a new workflow BEFORE the first real run — topology errors
  are free here and expensive there.
- Keep a known-good `--dry-trace` next to complex workflows as the
  comparison baseline.
