
# Output Schema — JSON shape

The skill produces a single JSON document with this exact shape. The document is written to `.config/sessions/{root-session-id}/evaluation.json` and also echoed to stdout.

## Top-level keys

```jsonc
{
  "schemaVersion": "1.0",
  "rootSessionId": "string",        // e.g., "2026-04-07T08-02-31_goal-planner"
  "rootRunId":     "string",        // e.g., "run-bf67682c6bac3558"
  "task":          "string",        // root session's task name, e.g., "goal-planner"
  "evaluatedAt":   "ISO-8601 UTC",  // when the skill ran, NOT when the session ran

  "how":         { /* see § How */ },
  "quality":     { /* see § Quality */ },
  "efficiency":  { /* see § Efficiency */ },
  "achievement": { /* see § Achievement */ },

  "summary": "string"   // 1-paragraph plain-English verdict for callers that just want the headline
}
```

The 4 dimension keys map directly to the task's analysis requirements:
- `how` — (a) how the work was done across delegations
- `quality` — (b) result quality vs goal
- `efficiency` — (c) credits/tokens, tools, time
- `achievement` — (d) goal achievement yes/no/partial

## § How — execution structure

```jsonc
"how": {
  "agentCount":  number,            // total distinct agents in the tree (incl. root)
  "totalTurns":  number,            // sum of turns across all agents
  "delegationTree": [
    {
      "runId":       "string",
      "task":        "string",
      "mode":        "string",      // worker | coordinator | teammate | background | (empty)
      "parentRunId": "string",      // empty for root
      "turns":       number,
      "children":    [ /* same shape recursively */ ]
    }
  ],
  "executionOrder": [
    // flat list, sorted by startedAt
    {
      "runId":     "string",
      "task":      "string",
      "startedAt": "ISO-8601",
      "input":     "string",        // turn 1 user_text from session show, may be empty
      "output":    "string",        // complete summary if present, else last assist_text, else ""
      "tools":     ["string", ...]  // distinct tool names this agent used (sorted)
    }
  ]
}
```

## § Quality — alignment with goal

```jsonc
"quality": {
  "goal": {
    "source": "param | first_user_text | task_md | none",
    "text":   "string"              // empty if source == "none"
  },
  "finalOutput": {
    "source": "root_complete_summary | last_assist_text | none",
    "text":   "string"
  },
  "criteriaMet": [
    // LLM extracts criteria from the goal, then judges each
    {
      "criterion": "string",        // verbatim or paraphrased from goal
      "met":       "yes | no | partial",
      "evidence":  "string"         // 1 sentence with concrete reference (file, turn, summary)
    }
  ],
  "alignmentScore": number,         // 0.0..1.0 — LLM-assessed overall alignment
  "missingElements": ["string", ...] // empty if alignmentScore >= 0.95
}
```

**Notes**:
- `criteriaMet` is at most 8 entries — if the goal has many criteria, the LLM should pick the most load-bearing ones.
- `alignmentScore` is rounded to 2 decimals (e.g., `0.85`).
- The LLM should NOT invent criteria not present in the goal. If `goal.source == "none"`, `criteriaMet` MUST be `[]`.

## § Efficiency — cost and waste

```jsonc
"efficiency": {
  "totalTokens":     number,
  "inputTokens":     number,
  "outputTokens":    number,
  "totalDurationMs": number,
  "toolsByName": { "<tool>": <count>, ... },   // copied verbatim from session aggregate
  "perAgent": [
    {
      "runId":          "string",
      "task":           "string",
      "tokens":         number,
      "durationMs":     number,
      "turns":          number,
      "tools":          { "<tool>": <count>, ... },
      "efficiencyTag":  "✓ good | ⚠ acceptable | ✗ wasteful | ✗✗ failed",
      "efficiencyNote": "string"      // 1-2 sentences with cited evidence per evaluation-rubric.md
    }
  ],
  "wastefulPatterns": [
    // discrete findings from the LLM's review of the data
    {
      "pattern":  "string",           // short id like "retry_delegate", "polling_no_backoff", "ignored_input"
      "agent":    "string",           // task name or runId
      "evidence": "string",           // 1 sentence pointing to specific turns/tool calls
      "tag":      "⚠ acceptable | ✗ wasteful | ✗✗ failed"
    }
  ]
}
```

**Notes**:
- Raw metric fields (`totalTokens` … `toolsByName`) come straight from `ktw-utils session aggregate --json`. Do not invent numbers.
- `efficiencyTag` and `efficiencyNote` are LLM judgments — load skill `evaluation-rubric` via `get-instructions`.
- `wastefulPatterns` is allowed to be empty if the run is clean. But per the rubric, "real runs always have some friction worth flagging" — be honest, not polite.

## § Achievement — goal yes/no/partial

```jsonc
"achievement": {
  "achieved":   "yes | no | partial | unknown",
  "confidence": number,              // 0.0..1.0, rounded to 2 decimals
  "evidence": [
    "string", ...                    // bullet-style facts that support `achieved`
  ],
  "gaps": [
    "string", ...                    // empty if achieved == "yes"
  ]
}
```

**Decision matrix**:
| `quality.alignmentScore` | `criteriaMet` predominant value | → `achieved` |
|---|---|---|
| ≥ 0.90 | mostly `yes` | `yes` |
| 0.50–0.89 | mix of `yes`/`partial` | `partial` |
| < 0.50 | mostly `no` or empty | `no` |
| N/A | `goal.source == "none"` | `unknown` |

`confidence` reflects how reliable the judgment is — low if data is incomplete (sessions in-progress, missing summary.json) or if the goal source is weak (auto-extracted from boilerplate first user_text).

## § Summary

```jsonc
"summary": "string"
```

A single paragraph (3-5 sentences) plain English. Must include:
1. The achievement verdict (`yes/no/partial`) with the headline reason.
2. The most notable efficiency finding (good or bad).
3. One actionable suggestion if `achieved != "yes"` or any `wastefulPatterns` exist.

## Validation rules (the LLM must satisfy these)

1. **Schema completeness**: every top-level key (`schemaVersion`, `rootSessionId`, `rootRunId`, `task`, `evaluatedAt`, `how`, `quality`, `efficiency`, `achievement`, `summary`) MUST be present.
2. **No invented metrics**: every number in `efficiency.totalTokens`, `inputTokens`, `outputTokens`, `totalDurationMs`, `toolsByName`, and `perAgent.*` MUST come from `ktw-utils session aggregate` or `session show` output.
3. **No invented criteria**: `quality.criteriaMet` entries MUST trace to text in `quality.goal.text`.
4. **Severity tags exact match**: use the exact strings `"✓ good"`, `"⚠ acceptable"`, `"✗ wasteful"`, `"✗✗ failed"` (no variants).
5. **Empty arrays not null**: missing data is `[]`, never `null`.
6. **Empty strings allowed**: `goal.text`, `finalOutput.text` can be `""` if their source is `none`.
7. **JSON validity**: output MUST be parseable JSON. No trailing commas, no comments.
