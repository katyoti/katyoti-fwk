
# Goal Source — Fallback Chain

The skill needs to know what the original goal was to assess `achievement.achieved` and `quality.alignmentScore`. There is no single canonical source — different tasks express goals differently. Apply this fallback chain in order, stopping at the first source that yields a non-empty, non-boilerplate goal.

## Step 1 — Explicit `--goal` parameter

If the caller invoked the skill with an explicit goal text (e.g., `--goal "Build a CRM system for 50 users"`), use it verbatim.

```jsonc
"goal": { "source": "param", "text": "Build a CRM system for 50 users" }
```

**When to use**: When the caller (e.g., goal-planner running its own self-evaluation loop) knows the goal best because it produced it.

## Step 2 — Root session's first `user_text`

Read `ktw-utils session show <root-session-id> --json`. Look at `turn_details[0].user_text`. If non-empty AND not boilerplate, use it.

```jsonc
"goal": { "source": "first_user_text", "text": "Help me plan a 6-month engineering org restructuring" }
```

**Boilerplate to reject** (case-insensitive, exact or near-exact match):
- "Please start the task."
- "Begin."
- "Start."
- "Run the task."
- "" (empty)
- Any string shorter than 10 characters
- Any string that is purely a JSON object (e.g., `{"input":"foo"}`) — that's input data, not a goal

**When this works**: Interactive tasks where the user's first turn states the goal directly (most goal-planner sessions).

## Step 3 — Root session's TASK.md

If steps 1 and 2 fail, locate the TASK.md for the root task. The path depends on whether the task is embedded or external:

- **Embedded tasks**: `app/ktw-agents/embed/tasks/{task-name}/TASK.md`
- **Test fixtures**: `test_files/test_case_*/tasks/{task-name}/TASK.md` — find via `ktw-agents tasks list --home <home>` if needed
- **Workspace tasks**: check the session's working directory for a TASK.md

Extract:
1. The frontmatter `description:` field, OR
2. The first paragraph after the H1 heading

Whichever is more substantive.

```jsonc
"goal": { "source": "task_md", "text": "Goal-oriented planning: clarify goals via interview, build a plan, design teams, produce a planning document" }
```

**When this works**: Non-interactive tasks (test fixtures, embedded utility tasks) where the goal is encoded in the task definition.

## Step 4 — None

If steps 1-3 all fail, set:

```jsonc
"goal": { "source": "none", "text": "" }
```

When this happens, the skill MUST also:
- Set `achievement.achieved = "unknown"`
- Set `achievement.confidence = 0.0`
- Leave `quality.criteriaMet = []`
- Leave `quality.alignmentScore = 0.0`
- Note in `summary` that no goal could be determined and the assessment is structural only

## Examples

### Example 1: goal-planner interactive run
- Step 1: no `--goal` param
- Step 2: `turn_details[0].user_text` = `"I want to build a customer portal for our SaaS product"` → ✓ use this

```jsonc
"goal": { "source": "first_user_text", "text": "I want to build a customer portal for our SaaS product" }
```

### Example 2: team-test fixture run
- Step 1: no `--goal` param
- Step 2: `turn_details[0].user_text` = `"Please start the task."` → boilerplate, reject
- Step 3: TASK.md description = `"Test: verify team creation, teammate spawning, and team listing"` → ✓ use this

```jsonc
"goal": { "source": "task_md", "text": "Test: verify team creation, teammate spawning, and team listing" }
```

### Example 3: caller-driven self-evaluation
- Step 1: `--goal "Produce a 3-milestone plan with verification criteria"` → ✓ use this immediately

```jsonc
"goal": { "source": "param", "text": "Produce a 3-milestone plan with verification criteria" }
```

### Example 4: orphan session (goal lost)
- Step 1: no `--goal` param
- Step 2: `turn_details` is empty (session was killed before any turn completed)
- Step 3: TASK.md cannot be located (task was renamed/removed since the session ran)

```jsonc
"goal": { "source": "none", "text": "" },
"achievement": { "achieved": "unknown", "confidence": 0.0, "evidence": [], "gaps": [] }
```
