---
name: failure-patterns
description: "A symptom-to-fix catalog for a misbehaving agent or task run: no tool calls, wrong tool name, missing complete, delegate 'task not found'. Load when a run does not behave as expected."
advertised: false
hashtags: [debugging, failures, create-items]
category: guide
---

# Common Failure Patterns and Fixes

## Agent Behavior Issues

### Agent responds with text only, no tool calls
**Symptom**: Task completes with 0 tool calls, agent just writes a text response.
**Cause**: Prompt lacks explicit tool-use mandate.
**Fix**: Add to TASK.md body:
```
You MUST use your tools — do not just respond with text.
Available tools: complete, delegate, file-read, file-write, file-edit, glob, grep.
```

### Agent calls wrong tool name
**Symptom**: `tool 'X' not found` in journal.
**Cause**: Tool name mismatch (a snake_case spelling — `ask_user` for
`ask-user` — matches no tool and no permission gate) or LLM confused by prompt
vocabulary.
**Fix**: Use bold tool names in prompt, spelt as registered (kebab-case; a
group's methods are `<group>-<method>`). List exact names. Avoid backticks
around tool names. Beware collisions with prompt vocabulary (`task`, `run`).

### Agent doesn't call complete
**Symptom**: The run ends as soon as the model stops calling tools, its last
text taken as the summary — no `complete` call in the journal, no structured
result for the parent — or it runs until `MaxTurns` / `MaxBudgetUSD` /
`MaxDurationS`.
**Cause**: The body never orders the complete tool by name.
**Fix**: Add a termination line: "When ALL deliverables are produced, you MUST
use the **complete** tool to return <result>." (Not in a `mode: chat` task: a
driven chat never completes itself.) Explicit budget exhaustion shows as
`agent.budget_exceeded` in the session trace.

### Agent loops on same tool call
**Symptom**: Same tool called 3+ times with identical input.
**Cause**: Repetition — agent stuck in a loop.
**Fix**: The repetition detector (built-in) should catch this. If not,
simplify the prompt and remove ambiguity. Polling tools (`task-status`,
`check-messages`) have an anti-loop threshold of 10, not 3 — repeated polls
are expected for those.

## Delegation Issues

### "task not found" error
**Symptom**: `delegate to "X": task "X" not found`
**Cause**: Task name doesn't match any registered task.
**Fix**: Run `./bin/ktw-agents sc tasks list` (or `<binary> tasks list`) to
see registered tasks. Verify the `name:` field in TASK.md frontmatter
matches the slug you're delegating to. Rebuild after adding new tasks
(embedded FS is baked into the binary at build time).

### Background task never completes
**Symptom**: `task-status` shows `running` indefinitely.
**Cause**: Child task doesn't call `complete`, or child crashes silently.
**Fix**: Check child TASK.md for the complete instruction. Inspect the
child's session via `ktw-utils session show <child-session-id>`. The
child's session-id is in the parent's `delegate` tool result.

### Fork child starts fresh (no parent context)
**Symptom**: Forked agent doesn't know parent's conversation.
**Cause**: `HistoryMode` not set to `full_fork`, or `InitialMessages` empty.
**Fix**: Use `mode: fork` (which sets `HistoryMode: full_fork` automatically
via the preset). If invoking programmatically, ensure
`AgentRunRequest.InitialMessages` is populated.

### Worker spawned via coordinator can't write files
**Symptom**: file-write returns permission-denied or "tool not allowed".
**Cause**: Coordinator mode automatically restricts workers to `SafeToolSubset`
(file-read, file-write, file-edit, glob, complete) under
`PermissionMode: restricted` — this is intentional.
**Fix**: If the worker genuinely needs broader tools, spawn directly with
`mode: worker` instead of through a coordinator, or override the
preset via the delegate's `permission_overrides` (if supported).

## Interactive (ask-user) Issues

### ask-user fails with EOF
**Symptom**: `read input: EOF` in tool result.
**Cause**: Running with stdin piped (`echo "..." | ktw-agents run ...`)
consumes stdin; nothing left for ask-user.
**Fix**: Use `--if input.json` instead of piping, or use the `ktw-test`
virtual terminal harness which provides programmatic POST /input.

### ask-user prompt not visible
**Symptom**: Task hangs, no prompt appears.
**Cause**: Running in non-interactive mode without ktw-test.
**Fix**: Run interactively (no pipe), or use ktw-test which renders the
ask-user UI and lets you POST answers.

## Build/Run Issues

### Task "not found" through ktw-test but works directly
**Symptom**: `tasks list` shows the task in bash, but ktw-test run says
"task not found".
**Cause**: Windows `.exe` duplication. `go build -o bin/ktw-agents` creates
a binary without `.exe`. Go exec then resolves `.exe` suffix and finds an
OLD binary alongside.
**Fix**: Always build with `make ktw-agents` (preferred) or
`go build -o bin/ktw-agents.exe`. Delete stale `bin/ktw-agents` (no
extension) if it exists.

### 0 tokens in/out
**Symptom**: Task completes instantly with 0 tokens.
**Cause**: No LLM provider configured, or API key missing.
**Fix**: Check `<workspace>/.katyoti/config/LLMProfiles.yaml` and ensure
the relevant env var (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.) is set —
either in the shell or in `<workspace>/.katyoti/.env`.

### Task not in list
**Symptom**: `<binary> tasks list` doesn't show the task.
**Cause**: TASK.md not in correct directory or missing required frontmatter
fields (`name`, `type`, `description`).
**Fix**: Ensure file is at `app/<sidecar>/embed/tasks/<name>/TASK.md` with
valid YAML frontmatter. Each task lives in its own folder. Rebuild.

### Zombie sidecar processes after exit
**Symptom**: `ktw-*` processes linger after `ktw-agents` exits.
**Cause**: A sidecar's `ApplicationScope.Container.Close()` was not called on
shutdown (cobra's `PersistentPostRun` closes the scope — it disposes the bun
supervisor and other closers).
**Fix**: Rebuild the offending binary with `make <binary>` so it links the
current shutdown path, and confirm no stale pre-build process is still running.

## Journal / Session Diagnosis Workflow

Sessions land at `Env().SessionsDir()` —
`<XDG_DATA>/katyoti/universes/<universe>/sessions/<session-id>/`. On Windows
this resolves to `%LOCALAPPDATA%\katyoti\universes\<universe>\sessions\<id>\`.
Don't path-guess; use `ktw-utils`:

```bash
# 1. List recent sessions (most recent first)
./bin/ktw-utils session tree --recent 5

# 2. Pick a session id from the tree, then show full detail
./bin/ktw-utils session show <session-id>

# 3. Aggregated stats (tokens, cost, tool calls, errors)
./bin/ktw-utils session aggregate <session-id>

# 4. For multi-agent runs (orchestration / teams), look at the delegation graph
./bin/ktw-utils session tree <session-id>
```

What to look for:
- Did the agent call the right tools? (`session show` shows each tool_call)
- Did delegation work? (`session tree` shows parent/child relationships)
- Did `ask-user` prompts get answered? (`user.jsonl` events in `session show`)
- Did the agent call `complete`? (final tool_call should be `complete`)
- Was the output written to the expected file? (`tool_result` for file-write)

For programmatic analysis use the `run-report` skill (human-readable
markdown with Mermaid delegation graph) or `run-quality-evaluation` skill
(machine-readable JSON evaluation) — both consume `ktw-utils session`
output.

## Trace files

`--trace <file.jsonl>` writes enriched NDJSON with LLM text, tool I/O, and
streaming deltas. `--trace -` writes to stderr. Useful for inspecting
LLM-side events that don't reach the journal (e.g. mid-stream tool selection).

```bash
./bin/ktw-agents run my-task --trace trace.jsonl
jq -r 'select(.type == "agent.tool_call") | .tool' trace.jsonl
jq -r 'select(.type == "agent.turn_start") | .turn' trace.jsonl | wc -l   # turn count
```
