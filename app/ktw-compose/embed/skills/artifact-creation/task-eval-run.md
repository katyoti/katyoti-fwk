---
name: task-eval-run
description: "Run a task or agent (or take a finished session) and score its context efficiency and capability fit from the real system prompt and tools. The engine behind task-evaluation; load to evaluate one."
advertised: false
hashtags: [evaluation, autonomous, task, agent, create-items]
category: guide
---

# Running a task/agent to score it

Produces a scored verdict for one (task, scenario) pair. The user side is
simulated for interactive/chat tasks (script the `ask-user` answers as in
`ktw-compose:/skill/skill-eval-run` §1). You may also point this at an existing
finished session — skip straight to "Read the session".

> **Access rule — never path-hunt for a run.** This is the EVAL ENGINE, run by
> an operator / post-hoc evaluator that HAS a shell + `ktw-utils`. An **in-loop
> agent driving a child must NOT run this mid-loop** — it has no shell and must
> never `ls`/`find`/guess a sessions-dir path. For its in-loop decisions it takes
> the child's RESULT from the **task surface**, not the filesystem:
> - **metrics** — `totalTokens` / `turns` / `toolUseCount` / `summary` from the
>   completed task's `AgentResult` (the `delegate` return / `task-status`);
> - **artifacts** — read via **`file-read` at the deliverable's known `home`**
>   (from the composition) or the paths in `AgentResult.files` — never a
>   sessions-dir scan.
>
> The session **id** (from `AgentResult` / `task-status`) is the only handle
> passed on; resolving that id to its journals is THIS engine's job (below),
> done from a shell/operator context — not by an in-loop LLM. (A future
> session-read agent tool would let the engine read journals by id as a tool,
> retiring the operator-shell dependency entirely.)

## 0. Headless or GUI-required? (decide who runs it)

Most tasks run AUTONOMOUSLY — no user needed. The deciding factor is **GUI-tool
usage, NOT input requirements**:

- **Required input is never a reason to need a human.** A task's input contract
  (`<name>.input.dsl.yaml`, and its `ui-form`) is satisfied by passing a JSON value to `--input`; the form
  is just the GUI's way to collect that same JSON. So a task with required fields
  still runs headless.
- **`ask-user` turns** are scripted (a pre-written answer transcript) — also not
  a reason to need a human.
- **GUI-required ⟺ the task must USE GUI tools to fulfil its goal** — i.e. its
  deliverable is a live-mounted UI component (`display-ui-component` /
  `complete-ui-component`) or a live GUI chat. NOTE: having those tools in the
  catalog is NOT enough — the `standard` bundle ships them, so nearly every task
  *could* call them. What matters is whether the task's WORK is mounting UI. A
  task that writes files / returns data is headless even though the UI tools sit
  unused in its catalog (hello-world is the canonical example: required input +
  a `ui-form`, yet headless — it only writes a file).

So:
- **Headless** → the evaluator runs it itself (see §1) — fully autonomous.
- **GUI-required** → ask the user to run it in ktw-gui and hand over the session
  id; evaluate from that captured session.

Evaluating from an existing session (the GUI path, or any prior run) is a
first-class entry — skip producing a run and go to "Read the session".

## 1. Produce (or take) a session

For a **headless** task, run it autonomously from the CLI — input is a JSON value,
no human in the loop:

```
ktw-agents run <task-name> --input '<json matching the input contract>'
# input can also be a file:  --input ./in.json   (or .yaml)
# add --max-turns N to bound the run; --trace - to watch the loop
```

Example (a task with REQUIRED input, run with zero user interaction):

```
ktw-agents run hello-world --input '{"name":"Alice","greeting":"Bonjour","loud":true}'
```

For a **GUI-required** task, take the session id the user provides after running it
in ktw-gui. Either way, note the **session id**. The session lives at
`<DataDir>/universes/<universe>/sessions/<id>/`. The three journals:

- `llm-model-calls.jsonl` — one line per HTTP call to the model. Each line's
  `request_body` carries **`system`** (the full system prompt) and **`tools`**
  (the exact tool catalog the model saw). THIS is the source for the measured
  dimensions — no other journal has the assembled prompt.
- `llm.jsonl` — the decoded turn-by-turn loop (assistant text, tool calls,
  tool results).
- `user.jsonl` — user-side messages.

For the delegation tree / aggregates / per-agent token totals, use the
`ktw-utils session` surface (tree / aggregate / show) exactly as
`ktw-compose:/skill/run-quality-evaluation` describes.

## 2. Read the assembled prompt + tools (the measured reads)

Navigate the run with the **session-read** tool (turn-addressed, progressive),
passing the `session_id` from `AgentResult` / `task-status` — no path, no shell:

1. **Overview** — `session-read(session_id)` returns the MAP: `turns`,
   `system_prompt_chars`, the tool **`histogram`** (name→count), `tool_call_total`
   vs `tools_distinct`, the **`toolset`** block (`given` / `used` / `never_used` —
   catalog vs invoked), and a one-line-per-turn `index`. `tool_call_total` ≫
   `tools_distinct`, or a tool with a high histogram count, is **redundancy** — a
   core EFFICIENCY signal; the `index` shows WHERE. A large `toolset.never_used`
   vs a small `used` is **over-provisioning** (B5).
2. **The cold prompt** — `session-read(session_id, turns: "0")` returns turn 0's
   `system_prompt` (the assembled prompt — length /~4 chars ≈ tokens, or use
   `AgentResult.totalTokens`) and `tools` (the agent's full catalog).
3. **Zoom for efficiency** — `session-read(session_id, turns: "<a-b>")` (or
   `tool:`/`grep:` to find them first) returns per-turn `tool_calls` (name +
   input) + `duration_ms`. **Repeated identical name+input calls = wasted
   turns**; long `duration_ms` = stalls. Judge efficiency from THIS, not the map.
   `raw: true` dumps the exact jsonl for a turn span if you must.

(Only an operator with a shell, evaluating out-of-loop, parses the raw journals
directly — an in-loop agent uses **session-read**.) Then compute:

Then compute, mechanically:

- **A1 prompt size** — token estimate above, vs the task-shape budget in
  `task-eval-criteria` §A1.
- **A1b body size** — read the task file's body (the markdown after frontmatter),
  count its words, and estimate the minimum that still carries the procedure + DoD.
  Report `words / est-minimum` and name the excess. (Separate from A1: A1 is the
  whole assembled prompt; A1b is the AUTHORED body alone.)
- **A2 unused blocks** — list the `## <Heading>` sections in `system_prompt`. For
  each capability block (Workspace, Delegation, Templates, External Programs, …),
  check whether ANY of its tools appears in the `histogram` (was called at all).
  Block present + 0 of its tools in the histogram = unused.
- **A3 skill over-attachment** — count `<skill>` entries in the `<skills_system>`
  block (listed) vs distinct `get-instructions` calls in the run (loaded).
- **A5 duplicate entries** — count RAW `<name>` (or `- <uri>`) occurrences in the
  block and compare to the count of UNIQUE URIs. Any URI appearing more than once
  is a duplicate-listing defect. **Do NOT dedupe in your reading** — report the
  raw count vs unique count (e.g. "listed=4, unique=2 → render-data-as-ui ×3").
  Also check that `<skills_system>` and each `## <capability>` block appears at
  most once.
- **A4 contradictions** — scan the section headings/text for mutually exclusive
  guidance (the canonical one: a `## Completion` block AND a `## Chat Mode`
  "no completion" block both present).
- **B1 instructed-tools-exist** — extract every tool name the task body / system
  prompt tells the agent to "call" (look for explicit tool URIs / names) and
  confirm each is in the `tools` array.
- **B2 fallback** — compare the `histogram` keys (tools called) against `tools`
  (the catalog): any called name NOT in the catalog (and any `scripts-run` /
  `bash` standing in for a missing native tool) is a fallback defect.
- **B3/B4 declared surface** — read the task frontmatter (`config.plugins` /
  `skills` / `tools`); compare what was declared against what was actually used.
- **B0 format + type compliance** — for each artifact the run produced, run
  **`object-lint`** on its node (this runs the linters the node type declares —
  compose `skill-format` / `task-format`, including the `skill-is-task-shaped`
  artifact-TYPE rule, plus the neutral basics). Record every finding (rule +
  severity). Any **error-severity** finding is a compliance defect that gates the
  verdict — the run did NOT produce a contract-clean artifact of the right type.
  This is deterministic (the lint), never an LLM re-reading prose.
  > **Two complementary lenses.** `object-lint` gives the DETERMINISTIC
  > format/type findings — gate the verdict on them. The
  > `ktw-compose:/skill/skill-format` / `task-format` SKILL is COMPLEMENTARY: it
  > carries the qualitative judgment a code linter cannot encode (tone, phrasing,
  > intonation, when a rule reasonably bends) — get-instruction that skill by its
  > URI to weigh what the findings don't capture. Reach every contract and spec
  > through the skill/resource system; keep `file-read` for the produced ARTIFACT at
  > its known home.
- **B5 toolset appropriateness** — read the overview `toolset` block:
  `given`/`used`/`never_used` (the over-provisioning numbers — no judgment needed),
  then apply the B5 policy (`task-eval-criteria`): the `never_used / given` ratio,
  and whether a shell/escape tool (`bash`/`scripts-run`) was in the catalog of an
  authoring/driver run (⚠) or USED to author/path-hunt (✗✗ — confirm with a
  `tool: bash` zoom; `ls`/`cat`/`find` to explore = path-hunting). Record
  `given`, `used`, the `never_used` count, and any shell-tool use with its turn.

Record each as a number with its evidence pointer (the call index / turn / URI).

## 3. Score and emit

Apply the `task-eval-criteria` rubric to A–E, each verdict citing the numbers from
§2 (measured) or the journals (judged). Reuse
`ktw-compose:/skill/run-quality-evaluation` for the C/D/E outcome scoring and the
`evaluation.json` envelope; ADD the measured A/B block to it:

```json
"contextEfficiency": {
  "systemPromptTokens": 0, "promptBudget": 0,
  "bodyWords": 0, "bodyWordsMinimum": 0,
  "unusedBlocks": [], "skillsListedRaw": 0, "skillsListedUnique": 0, "skillsLoaded": 0,
  "duplicateSkills": [], "contradictions": []
},
"capabilityFit": {
  "instructedToolsMissing": [], "fallbackCalls": [],
  "declaredSurface": {"plugins": [], "skills": [], "tools": []},
  "usedSurface": [],
  "toolset": {"given": 0, "used": 0, "neverUsed": 0, "shellToolUsed": []}
},
"formatCompliance": {
  "artifacts": [{"home": "", "findings": [{"rule": "", "severity": ""}]}],
  "clean": true
},
"conclusion": {
  "outcome": "NOTHING_TO_OPTIMISE",
  "gaps": []
}
```

**The verdict is worst-of.** A run does NOT conclude `NOTHING_TO_OPTIMISE` if any
of these holds: a B0 error-severity format/type finding on a produced artifact
(it shipped a non-contract or mis-typed artifact), a B2 fallback, or a B5 ✗✗
(shell tool used to author/path-hunt). Outcome quality (D) being green does not
override a compliance or capability-fit defect — those become `gaps`. A produced
artifact that violates `skill-format` or is task-shaped, or a driver that
`bash`-`ls`-hunted the repo, is a GAPS verdict with those findings listed, never a
PASS.

The **`conclusion`** block is mandatory and is the ISA outcome (phase 4): either
`outcome: "NOTHING_TO_OPTIMISE"` with an empty `gaps`, OR `outcome: "GAPS"` with
`gaps` = the clean actionable list (each: `{title, category, problem, evidence,
impact, effort, risk, approach}`). No third state — an eval that can't decide
isn't finished.

Validate against `ktw-compose:/reference/artifact/eval/output-schema.md` (plus the
blocks above), write it beside the session, and echo it. A measured field left as
an estimate, a verdict without its number, or a missing/ambiguous `conclusion` is
a failure — fix it before declaring the evaluation done.
