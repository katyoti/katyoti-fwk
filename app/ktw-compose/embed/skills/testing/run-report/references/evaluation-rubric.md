# Evaluation Rubric — how to judge efficiency

The skill produces qualitative LLM-driven evaluations for every delegation, tool usage, and skill usage. This file defines how to judge them.

## Severity tags

Use exactly these 4 severity levels. Pick the one that best fits.

| Tag | Meaning |
|-----|---------|
| ✓ good | Worked as intended. Reasonable token cost, no waste, clear outcome. |
| ⚠ acceptable | Works but with minor friction (e.g., 1 retry, slight over-fetching, small redundancy). Worth noting but not a problem. |
| ✗ wasteful | Significant inefficiency (e.g., 3+ identical tool calls, agent re-asking for data it should already have, polling without backoff). |
| ✗✗ failed | The step did not achieve its goal (errors, abandoned work, agent ended turn without using tools). |

## Evidence requirement

Every evaluation MUST cite specific evidence. Bad evaluations are vague ("the agent did well"); good evaluations point to concrete data.

**Bad** (vague):
> The plan-builder did a good job.

**Good** (with evidence):
> ✓ good — Plan-builder produced a 4-milestone plan in 3 turns (1131 in / 942 out tokens) and got user approval on the first iteration.

## What to look for per category

### Per-delegation evaluations (section 3 of report)

For each agent in the tree, judge:

1. **Did the work match the prompt?** Compare the parent's `delegate(...)` input against the child's final `complete(summary)`.
2. **Was the turn count reasonable?** Workers should usually complete in 3-7 turns. Coordinators in 5-15. Anything higher needs justification.
3. **Did it retry?** Look for repeated identical tool calls or repeated failed delegate attempts.
4. **Did it return useful data?** A `complete(summary)` that just says "task done" is wasteful — the parent can't use it.
5. **Did it ignore its input?** If the parent passed structured data via `input` and the child asked the user to re-supply it, that's `✗ wasteful`.

**Examples:**

- ✓ good — goal-interview completed in 5 turns (4 ask-user rounds + final confirm), returned full structured goals/criteria/constraints in complete(summary). 1107 input tokens.
- ⚠ acceptable — plan-builder needed 6 turns instead of 4, but the extra rounds were spent iterating on user feedback (turn 4 had a real adjustment). Worth the cost.
- ✗ wasteful — team-designer was delegated twice (turns 3 and 5 of the coordinator) because the first attempt's complete summary lacked role assignments. The retry consumed an extra 942 output tokens.
- ✗✗ failed — plan-builder ended turn 2 with `stopReason: end_turn` without calling any tool. The parent received an empty result and had to retry.

### Per-tool evaluations (section 4 of report)

For each tool name in the aggregate `toolsByName`:

1. **Was the count appropriate?** `complete` should be called once per agent. `delegate` count should match the number of distinct sub-tasks. `task-status` is normal up to ~10 polls per background task.
2. **Did errors occur?** Cross-reference `errorCount` and any `tool_error` events.
3. **Was input/output proportional?** Tools that return huge results from tiny inputs (like reading a large file) are fine. Tools that consume huge inputs and return nothing (like a failed file-write) are wasteful.

**Examples:**

- ✓ good — `ask-user` called 8 times across goal-interview (4 rounds × text + confirm). Each call had a clear question and a useful response.
- ⚠ acceptable — `task-status` called 9 times polling the plan-writer teammate. Within the polling threshold (10) but verbose — consider longer backoff.
- ✗ wasteful — `delegate(plan-builder)` called 3 times in the coordinator session. The retries were caused by the child not seeing input data on the first call.

### Per-skill evaluations (section 5 of report)

For each skill in `skillsByAgent`:

1. **Was the skill loaded then used?** Check if the agent's subsequent assist_text shows it actually applied the methodology.
2. **Was loading worth the tokens?** `skills-get-instruction` returns ~5000 tokens of skill content. If the agent only used 10% of it, the load was costly.
3. **Did the agent follow the rubric in the skill?** For example, `interview-technique` says "3-5 rounds max" — did the agent honor that?

**Examples:**

- ✓ good — goal-interview loaded `interview-technique` and visibly followed its 4-round structure (open exploration → clarifying probes → success criteria → confirm).
- ⚠ acceptable — plan-builder loaded `planning-methodology` but only used the milestone structure section. The constraint-first principle was ignored.
- ✗ wasteful — team-designer loaded `team-design-patterns` but the resulting team structure didn't reflect the methodology's "one owner per deliverable" principle.

## Top-level efficiency overview (section 6 of report)

This is the synthesis. Answer 3 questions:

1. **Did the goal get achieved?** Yes / No / Partial. Cite the final output and compare to the initial input.
2. **Was the cost reasonable?** Compare total tokens to "what a single agent would spend on the same task". If the multi-agent overhead doubled the cost, that's `⚠`.
3. **Was the time reasonable?** Total wall-clock time for an interactive task should be 2-5 minutes. Background-heavy tasks can be longer. Anything over 10 minutes needs explanation.

## Risks & recommendations (section 7 of report)

Be specific and actionable:

**Bad recommendation**:
> Make the prompts better.

**Good recommendation**:
> Tighten the goal-interview TASK.md to enforce a max of 4 rounds — currently it does 5-6 because the LLM keeps adding "one more clarifying question". Add an explicit early-stopping rule.

## What NOT to do

- **Don't invent numbers.** Only cite metrics that appear in the ktw-utils output.
- **Don't be polite.** If something was wasteful, say so. The user wants honest feedback.
- **Don't over-explain.** Each evaluation is 1-2 sentences. The reader can infer details from the cited evidence.
- **Don't grade on intent.** The agent's "trying its best" doesn't excuse waste. Grade on outcomes.
- **Don't skip the negative examples.** A report with all ✓ tags is suspicious. Real runs always have some friction worth flagging.
