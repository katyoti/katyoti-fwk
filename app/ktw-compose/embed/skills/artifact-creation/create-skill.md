---
name: create-skill
description: "Drive the full creation of a ktw skill — from a vague request or given input to a validated, registered skill. Load when asked to create, author, or add a skill."
advertised: false
hashtags: [skill, authoring, create-items]
category: guide
---

# Creating a skill

You own the end-to-end path from "I want a skill that…" to a working skill that
loads at the right moment. You do not hold the format or the interview content
yourself — you orchestrate the skills that do. Run these phases in order; do not
skip the clarify or validate phases.

## 1. Clarify the intent

Load `ktw-compose:/skill/skill-intent-interview` and follow it to produce the
**skill intent brief** — one job, a what+when description, anti-triggers, the
procedure with its decision points, the knowledge carried, surfaces, and whether
it is a single leaf or a top skill with sub-cases. Confirm the brief with the
user before writing anything.

If the request already comes with **input material** rather than a bare
intent, this is the from-input path. When the material is an EXISTING skill
in a foreign format (Anthropic/Claude SKILL.md) or raw knowledge documents,
load `ktw-compose:/skill/skill-import` — it normalizes the format, then
returns here. For already-ktw-shaped material, load
`ktw-compose:/skill/skill-layout-planning` to inventory every item to create
before writing. Otherwise continue.

## 2. Decide it is actually a skill — and extend before you create

From the brief, confirm a skill is the right artifact: it teaches a *procedure
with decisions*. If there are no decisions it is a reference; if there is no
procedure it is a tool. The "when NOT to write a skill" table lives in
`ktw-compose:/skill/skill-format`. If it is not a skill, stop and route: a task →
`ktw-compose:/skill/create-task`; a reference → author it per
`ktw-compose:/skill/skill-format`; a **tool/service (Go)** → not a files-only
artifact, hand off to the code track (`ktw-code` / `ktw-gocode`).

Then check for overlap — adding a near-duplicate is the common failure. Scan
`skills list` for a skill that already claims this trigger. If one is close,
**extend it** (add a sub-skill, widen its scope) rather than create a sibling
with a blurred description. Create new only when the trigger is genuinely one no
existing skill owns.

## 3. Plan the shape and the home

**Shape:**
- **Single leaf** — one file.
- **Top + sub-cases** — one advertised top skill that routes to
  `advertised: false` sub-skills, each named from the parent. For more than a
  couple of branches, plan the full inventory with
  `ktw-compose:/skill/skill-layout-planning` first.

**Home** (the create-artifact decided the artifact; you decide the skill's
exact file location):
- **A sidecar owns the capability** → `app/<sidecar>/embed/skills/` — ships in
  the binary, available wherever it runs. When that sidecar's source is in THIS
  workspace, do not pay the rebuild loop while iterating: author into the
  overlay and promote at the end — load `ktw-compose:/skill/overlay-iteration`.
- **Workspace-specific, prototyped, or built from the user's own material** →
  the external `<workspace>/.katyoti/skills/` folder — loaded by the host at
  runtime, no rebuild, scoped to this workspace.
- **If unclear — ask.** The user either states where, or you surface the choice
  (which sidecar vs `.katyoti/skills/`). Do not default silently; placement
  decides who can load the skill and whether a rebuild is needed.

Set the kebab `name` (becomes `<provider>:/skill/<name>` for an embedded skill,
or `<name>` loaded by the host for an external one).

## 4. Write it

For an embedded home, emit the contract-valid stub first with the
**ktw-compose:compose/create-skill** tool
(`{"sidecar": "<name>", "name": "<skill>", "description": "<L0 line>", "advertised": false}`)
— it places the file, sets the strict frontmatter, and reports the rebuild +
go:embed notes. When the sidecar source is in this workspace, add
`"target": "overwrite"` to iterate live without rebuilding (see
`ktw-compose:/skill/overlay-iteration`). For a workspace-folder home, create the
file directly.

Then load `ktw-compose:/skill/skill-format` and author the body to the
contract: the what+when description from the brief, a second-person procedure
naming tools/resources by bare URI, and the checklist satisfied. Write sub-skills the same way; never inline a sub-skill into its
parent — name it and let the LLM descend.

## 5. Validate

- The strict frontmatter parses (no key outside the contract).
- The skill appears in the sidecar's `skills list` — the top skill advertised,
  sub-skills hidden but loadable by name.
- Re-read the description cold: from that one sentence alone, is it clear what
  the skill does and when to load it, and is it distinct from its siblings? If
  not, fix the description before declaring done — it is the highest-leverage
  line in the file.
- Walk the body once as if you were a fresh LLM: every cross-reference is a bare
  URI, no surface-specific loading text, no disk reads, no version framing.

## 6. Smoke

Trigger the skill the way a user would — state the situation its description
claims and confirm the host loads it and the body's procedure runs to a useful
output. For choosing the right test method (CLI probe, run+trace, …) load
`ktw-compose:/skill/test-artifact`. For evaluating quality/efficiency
beyond a single smoke — running the skill autonomously against a simulated
user and scoring the journals — load `ktw-compose:/skill/skill-evaluation`.

## Done

A skill is done when it parses, lists, loads on the right trigger, and its
description alone is enough for an LLM to pick it correctly. Not before.
