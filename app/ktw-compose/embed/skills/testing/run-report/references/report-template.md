# Report Template

The skill produces a markdown report following this skeleton. Replace `{placeholders}` with real data from `ktw-utils session tree`, `session aggregate`, and `session show`.

## Skeleton

````markdown
# Run Report — {task}

**Session**: `{root-session-id}`
**Started**: {started-at}
**Status**: {status}  ({completed | error | aborted})
**Duration**: {total-duration} ({total-duration-ms} ms total compute time)
**Total tokens**: {total-tokens} (in: {input-tokens}, out: {output-tokens})
**Agents**: {agent-count}
**Errors**: {error-count}

---

## 1. Delegation graph

```mermaid
graph TD
  {render-mermaid-here}
```

**Read**: `references/mermaid-rendering.md` for the styling rules. Use these conventions:
- Solid arrows (`-->`) for sync delegations (worker mode)
- Dotted arrows (`-.->`) for async delegations (background mode)
- Dashed arrows (`==>`) for spawn-teammate (team mode)
- Node label format: `task_name<br/>[mode]<br/>{turns}t {tokens}tok`

---

## 2. Goal & Outcome

**Initial input**: {first user message from root agent's turn 1}

**Final outcome**: {last assistant text or complete() summary from root agent}

**Was the goal achieved?**: {yes / no / partial} — {1-2 sentence explanation citing evidence}

---

## 3. Per-agent breakdown

For each agent in the delegation tree (depth-first, parent before children):

### {task-name} `({mode})` — `{run-id}`

- **Session**: `{session-id}`
- **Turns**: {turns}
- **Tokens**: {total-tokens} (in: {input}, out: {output})
- **Duration**: {duration-ms} ms
- **Errors**: {error-count}
- **Tools used**: {comma-separated tool names with counts: file-write(2), ask-user(4), ...}
- **Skills loaded**: {comma-separated skill names if any, else "none"}

**What it did**: {1-3 sentence narrative based on assist_text + tool calls across turns. Focus on the work done, not the mechanics.}

**Input received**: {summary of the initial user message — for delegated agents this is the parent's prompt}

**Output produced**: {summary of the final result — complete() summary or last assist text}

**Evaluation**: {severity-tag} {1-2 sentence judgment with evidence — cite turn numbers, token counts, retry counts, etc.}

---

## 4. Tool usage summary

| Tool | Count | Total time | Notes |
|------|-------|-----------|-------|
| {tool-1} | {n} | {ms} ms | {evaluation} |
| {tool-2} | {n} | {ms} ms | {evaluation} |
| ... | ... | ... | ... |

**Top observations**:
- {bullet point about most/least efficient tool}
- {bullet point about any tool that was wasteful or had errors}

---

## 5. Skill usage summary

For each skill loaded by any agent:

### {skill-name}

- **Loaded by**: {comma-separated agent task names}
- **Used in**: {turn numbers where skill content influenced subsequent decisions}
- **Evaluation**: {severity-tag} {1-2 sentences — was the skill content actually applied? was it worth the tokens?}

If no skills were loaded, write: "No skills were loaded during this run."

---

## 6. Efficiency overview

**Top-level evaluation**: {severity-tag} {1-2 sentences answering: did the goal get achieved efficiently given the constraints?}

**Token efficiency**: {tokens-per-deliverable ratio commentary}

**Time efficiency**: {wall-clock vs compute-time commentary, mentioning parallelism if any}

**Retry/waste detection**: {note any duplicate tool calls, redundant delegations, or LLM end-turn-without-tool events}

---

## 7. Risks & recommendations

What went well:
- {1-3 bullets — concrete things the run did right}

What could be improved:
- {1-3 bullets — specific changes that would make the next run better, e.g., "tighten the plan-builder prompt to discourage retries"}

What to watch:
- {0-2 bullets — non-blocking observations to monitor across future runs}

---

## 8. Raw data references

- **Root session journal**: `{sessions}/{root-session-id}/llm.jsonl`
- **Terminal journal**: `{sessions}/{root-session-id}/terminal/terminal.jsonl` (if terminal protocol was used)
- **Tree structure**: run `ktw-utils session tree {root-session-id}` to reproduce
- **Aggregate metrics**: run `ktw-utils session aggregate {root-session-id}` to reproduce
````

## Filling rules

1. **Order matters**: emit sections 1-8 in order. Don't skip sections — write "N/A" if data is missing.
2. **Use the rubric**: every "Evaluation" line must include a severity tag (see [evaluation-rubric.md](evaluation-rubric.md)).
3. **Cite evidence**: every judgment must reference specific turn numbers, tool call ids, or numeric metrics.
4. **Be terse**: aim for ~200-400 lines total. Don't pad.
5. **No invented metrics**: only report what `ktw-utils` actually returned. If a number is 0, say so explicitly.
6. **Summarize, don't dump**: never paste raw JSON or full message contents into the report. Distill to 1-3 sentences per element.
