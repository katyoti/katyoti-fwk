---
name: create-task
description: "Drive the full creation of a ktw task (a free-named `<name>.md` file) — from a vague request to a validated, runnable task. Load when asked to create, author, or add a task."
advertised: false
hashtags: [task, authoring, create-items]
category: guide
---

# Creating a task

You own the path from "I want a task that…" to a task that runs and returns a
useful result. You orchestrate the skills that hold the detail; run these phases
in order, and do not skip clarify or test.

## 1. Clarify the intent

Load `ktw-compose:/skill/task-intent-interview` and follow it to produce the
**task intent brief** — the job + its result, who launches it, the input
contract, the engine `type`, the tools/plugins, the extend-or-create verdict,
and the home. Confirm the brief before writing anything.

## 2. Decide it is actually a task

A task is *launched and tracked, returns a result*. If the value is in-the-
moment knowhow an LLM follows → that is a **skill**
(`ktw-compose:/skill/create-skill`). If it is deterministic logic returning
data → that is a **tool/service = Go**, which is the code-bearing track
(`ktw-code` / `ktw-gocode`), not here.

Then check `tasks list` for overlap — extend an existing task before adding a
near-duplicate.

## 3. Pick the type and the home

- **type** — `agent` (reasons + uses tools, the common case), `workflow` (fixed
  multi-step orchestration), or `task` (single deterministic dispatch). For an
  agent, load `ktw-compose:/skill/create-agent` for the prompt + plugin
  specifics; for a workflow, load `ktw-compose:/skill/create-workflow`.
- **home** — a sidecar's `embed/tasks/<name>.md` (rebuild) or
  `<workspace>/.katyoti/tasks/<name>.md` (runtime, no rebuild). Ask if unstated.
  When the sidecar source is in this workspace, iterate via the overlay (no
  rebuild) and promote at the end — load `ktw-compose:/skill/overlay-iteration`.

## 4. Write it

For an embedded home, emit the stub first with the
**ktw-compose:compose/create-task** tool
(`{"sidecar": "<name>", "name": "<task>", "type": "agent"}`); add
`"target": "overwrite"` to iterate live when the sidecar source is in this
workspace (`ktw-compose:/skill/overlay-iteration`). For a workspace tasks
folder, create the file directly. Then load
`ktw-compose:/skill/task-format` and author the task's `<name>.md` to the contract:
strict frontmatter, the chosen `type`, the `plugins` the brief named, and a body
that fits the type (a usable system prompt for `agent`). For an agent body,
follow the prompt-writing rules in task-format — name tools in bold, mandate
their use, and order the **complete** tool by name. To sharpen a draft
body, `ktw-compose:/skill/prompt-improvement` applies the prompt-engineering
techniques to it.

> ⚠️ **Set visibility + a friendly title.** A top-level, user-launched task
> gets a `title:` (the gallery headline; `name` stays the runnable id). A
> **worker/sub-task** that a top-level task delegates to gets `advertised: false`
> so it stays callable by name but does not clutter the gallery. See
> `ktw-compose:/skill/task-format` (Visibility ≠ callability).

> ⚠️ **Scope `plugins` to what the task actually uses — do NOT blanket
> `augmented`.** `augmented` = standard + web + scripting + skills; it hands
> the agent tools it never needs (web-fetch, `scripts-list`, …) and every
> extra tool is a chance for a stray, malformed call. List only what the job
> needs, e.g. an interview/CWM wizard wants `[standard, skill, cwm]`
> (file tools + ask-user + delegate, skills, the graph) — not scripting or
> web. Fewer tools = a tighter, more reliable agent.

> ⚠️ **Don't duplicate a skill's knowledge in the task body — link to it.**
> When the task's how-to is reusable domain knowledge (other tools would want
> it too), the knowledge belongs in a **skill**, and the task is a thin
> *launcher*: list the skill in `skills:` and let the body say "load
> `<provider>:/skill/<name>` and follow it." The task then carries only
> launch-specifics — `plugins` / `tools` / its input contract file / the engine `type`.
> A task and a skill that restate the same procedure is the smell: the skill is
> the single source of truth (reusable, independently discoverable), the task
> just points at it. Author the skill first (`ktw-compose:/skill/create-skill`),
> then the task. (Worked example: `ktw-weather`'s `forecast-page` skill holds the
> how-to; its `weather-forecast-page` task is a 2-line launcher over it.)
>
> Keep the launcher body to the **load-and-follow instruction only** — the body
> IS the executing agent's prompt, so it gets actionable instruction, never
> authoring rationale ("this task stays thin so the knowledge is reusable" is for
> you, not the agent). Write `Load X and follow it.`, not a paragraph about why.

## 5. Test

Run it and inspect the session — do not declare done on a clean build:

- run it (foreground, or piped input for a quick check)
- inspect with `ktw-utils session show <id>` — did it call the right tools, did
  delegation resolve, did `ask-user` get answered, did it call **complete**, was
  a result returned
- the catalog of common first-run failures is
  `ktw-compose:/skill/failure-patterns`; hard-won authoring
  lessons (each from a real failure) are
  `ktw-compose:/skill/authoring-lessons`

## Done

A task is done when it runs, returns the result its brief promised, and an
inspection shows it used its tools and completed cleanly. Not before.
