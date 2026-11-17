---
name: test-run-trace
description: "Run a task or agent for real with full NDJSON trace capture and judge the actual behavior. Load to test what the model really does, debug a misbehaving run, or A/B prompt text."
advertised: false
hashtags: [testing, trace, agent, run]
category: guide
---

# Run + trace

The richest method: a real LLM run with the evidence to judge it.

## Run

```bash
./bin/ktw-agents.exe run <task-or-uri> ["inline instructions"] \
  --workspace <root> --trace trace.jsonl --max-turns 40
```

- I/O routing: `--if <file>` (file input, keeps stdin free), `--is`/`--os`
  (stdin/stdout), `--pipe` (non-interactive shortcut), `--input '<json>'`
  (typed task input). `--pipe` consumes stdin — never use it for tasks that
  ask the user; use `--if` or `ktw-compose:/skill/test-virtual-terminal`.
- The trace file is fsync-per-line — `tail -f` sees events live.

## Watch it live — ALWAYS add `--console` for a test run

A real run — especially one that delegates children — is otherwise SILENT until
it finishes: stdout buffers and the operator has no signal that anything is
happening (or hanging). **Every test run you launch for someone to watch should
use `--console`**: a read-only multi-pane view, one pane per task run (the parent
PLUS every delegated child), each filling live with its tool calls and text as
the work unfolds. It is the feedback channel that proves "something is
happening" — the default for `test-scenario` and any multi-agent / agent-as-human
harness run.

- **On Windows, launch it in its OWN visible window** from Git Bash with `start`, so
  the console actually pops up (a redirected or backgrounded run shows nothing):

  ```bash
  start ./bin/ktw-agents.exe run <task> --console --input '<path-to-scenario.json>'
  ```

  On a Unix tty just run it inline: `./bin/ktw-agents.exe run <task> --console --input '<json>'`.

- Read-only — the console never answers prompts (it observes the shared event
  bus), so it's safe for self-driving runs. Press `q` to close when done.
- `--console` (live human feedback) and `--trace` (machine-readable evidence)
  compose — use both: watch the panes, keep the trace file for the verdict.

## Read the trace

Event types: `turn_start/turn_end`, `pre_api_call/post_api_call` (prompts +
responses), `model_stream` (text deltas), `pre_tool_exec/post_tool_exec`
(tool I/O), `decision`, `error`, `result` (stop reason, usage).

```bash
# tool calls in order
grep '"type":"pre_tool_exec"' trace.jsonl | jq -r '.tool'
# turns + outcome
grep -c '"type":"turn_start"' trace.jsonl; grep '"type":"result"' trace.jsonl | jq .
# token spend
grep -oE '"(input|output)_tokens":[0-9]+' trace.jsonl | awk -F: '{s[$1]+=$2} END {for(k in s) print k,s[k]}'
```

## Judge

Count turns, tool calls, errors, retries, wasted calls; check the deliverable
against what was asked; check which skills/references were loaded (and which
advertised ones were not). Score per
`ktw-compose:/skill/run-quality-evaluation` when a structured verdict is
needed.

## Rules

- One run is an anecdote — repeat before trusting a delta (the model is
  stochastic).
- Package the scenario as a task (`ktw-compose:/skill/skill-eval-method`) so
  the run re-executes unchanged.
- Keep the trace file — it is the evidence.
