# Data Shape — what ktw-utils returns

This skill consumes 3 ktw-utils commands. All return JSON when called with `--json`.

## 1. `ktw-utils session tree [session-id] --json`

Returns the delegation tree rooted at the given session (or the most recent root if omitted).

```jsonc
{
  "runId": "run-bf67682c6bac3558",            // unique run id, matches the executor's emitter
  "sessionId": "2026-04-07T08-02-31_goal-planner",  // on-disk directory name
  "dir": "C:\\...\\.config\\sessions\\2026-04-07T08-02-31_goal-planner",
  "task": "goal-planner",                     // task name from session_start
  "mode": "coordinator",                      // worker | coordinator | teammate | background | (empty)
  "model": "claude-sonnet-4-20250514",
  "provider": "anthropic",
  "parentRunId": "",                          // empty for root, set for children
  "startedAt": "2026-04-07T08:02:31.4824307+02:00",
  "children": [                               // recursive — same shape as parent
    {
      "runId": "run-d9634a6daea830f8",
      "sessionId": "2026-04-07T08-02-37_goal-interview",
      "task": "goal-interview",
      "mode": "worker",
      "parentRunId": "run-bf67682c6bac3558",
      "startedAt": "2026-04-07T08:02:37.2370488+02:00",
      "children": []
    }
  ]
}
```

**Use this for**: building the Mermaid diagram (section 1) and walking the per-agent breakdown (section 3).

## 2. `ktw-utils session aggregate [session-id] --json`

Returns tree-wide totals plus a flat per-agent array.

```jsonc
{
  "rootSessionId": "2026-04-07T08-02-31_goal-planner",
  "agentCount": 5,
  "totalTokens": 21141,
  "inputTokens": 14572,
  "outputTokens": 6569,
  "totalDurationMs": 234678,
  "totalTurns": 18,
  "totalErrors": 0,
  "toolsByName": {
    "ask-user": 8,
    "delegate": 4,
    "complete": 5,
    "file-write": 2,
    "skills-get-instruction": 3,
    "create-team": 1,
    "spawn-teammate": 1,
    "task-status": 9
  },
  "skillsByAgent": {
    "goal-interview": ["interview-technique"],
    "plan-builder": ["planning-methodology"],
    "team-designer": ["team-design-patterns"]
  },
  "perAgent": [
    {
      "runId": "run-bf67682c6bac3558",
      "sessionId": "2026-04-07T08-02-31_goal-planner",
      "task": "goal-planner",
      "mode": "coordinator",
      "status": "complete",
      "turns": 5,
      "inputTokens": 4971,
      "outputTokens": 823,
      "totalTokens": 5794,
      "durationMs": 66852,
      "errorCount": 0,
      "toolsByName": {"delegate": 3, "create-team": 1, "spawn-teammate": 1, "task-status": 9, "complete": 1}
    }
    // ... one entry per agent
  ]
}
```

**Use this for**: report header (section 1), tool usage summary (section 4), skill usage summary (section 5), efficiency overview (section 6).

## 3. `ktw-utils session show <session-id> --json`

Returns per-turn details for one session. Call this for each agent in the tree to fill the per-agent narrative (section 3).

```jsonc
{
  "session_id": "2026-04-07T08-02-31_goal-planner",
  "task": "goal-planner",
  "model": "claude-sonnet-4-20250514",
  "provider": "anthropic",
  "status": "complete",
  "duration_ms": 66852,
  "turns": 5,
  "total_tokens": 5794,
  "tool_calls": {"delegate": 3, "create-team": 1, ...},
  "turn_details": [
    {
      "number": 1,
      "user_text": "Please start the task.",     // initial input or ask-user response
      "assist_text": "I'll help you create...",  // assistant text content
      "tool_names": ["delegate"],
      "errors": 0,
      "duration_ms": 11582,
      "input_tokens": 145,
      "output_tokens": 387
    }
    // ...
  ]
}
```

**Use this for**: per-agent "What it did" narrative (section 3), evidence citations in evaluations (turn numbers, token counts).

## Conventions

- **Run id format**: `run-{16 hex chars}` — this is the unique identifier of an agent run within the process. Stored in the executor's emitter and copied to session_start events.
- **Session id format**: `{ISO timestamp}_{task name}` — the on-disk directory name. Multiple sessions can have the same task name (different timestamps).
- **Mode values**: `worker` | `coordinator` | `team-leader` | `teammate` | `background` | `fork` | `isolated` | `fork-isolated` | (empty for sessions without a `mode:` field in their TASK.md frontmatter)
- **Empty data**: if a session was killed or is in-progress, `summary.json` is missing and metrics may be 0 or partial. The skill should handle this gracefully — write "in-progress" or "0" rather than crashing.

## Field name consistency

Note that ktw-utils uses different JSON conventions across commands:
- `session tree` and `session aggregate` use camelCase (`runId`, `sessionId`, `parentRunId`)
- `session show` uses snake_case (`session_id`, `total_tokens`, `tool_calls`)

This is historical inconsistency in the existing tool. The skill should map between them as needed.
