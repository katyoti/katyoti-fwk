---
name: skill-layout-planning
description: "Plan which skills, sub-skills, references, and templates to create from source material before writing any file. Load when turning given input into one or more skills."
advertised: false
hashtags: [skill, authoring, planning, create-items]
category: guide
---

# Planning a skill layout from input

When the source is **given material** — an existing skill, an external skill, or
a set of documents — the hard part is not writing files, it is deciding *what
set of files* to write and how they nest. Do that here, get it approved, and
only then create. Writing first and structuring later produces flat dumps that
no LLM can navigate.

The discovery model you are planning against — L0 / L1 / L2, sub-skill vs
reference, only-top-advertised — is `ktw-compose:/skill/skill-format`. Load it
first; this skill applies it to a body of source material.

## Read the material for capabilities, not chapters

Do not mirror the document's structure. Extract the **capabilities** it
contains — each distinct *procedure with decisions* is a candidate skill. A
document's headings are how a human organised prose; skills are organised by
"when would an LLM reach for this". Group by trigger, not by table of contents.

## Check each capability against what exists — extend or create

Before planning a single new file, scan the live surface (the advertised
skills, and the full skill-type contribution listing) for skills that already
cover each extracted capability. This is the highest-leverage step when the source is existing
material: the goal is usually to **fold the material into the right skills**, not
to mint a parallel set. Per capability decide:

- **Extend** an existing skill — the same job, more depth or a new sub-case →
  add a sub-skill under it or widen it. Record `extend <uri>`.
- **Create** new — a trigger no existing skill claims. Record `create new`.

A capability that maps onto an existing skill is NOT a new tree node; it is an
edit to that skill. Plan those as edits, separately from the create list.

## Decide the tree

Produce a tree, top-down:

- **L0 — top skill(s).** The advertised entry points. Usually one per coherent
  capability area. Each gets a **what+when description** (its trigger) — "skill
  L0 triggers this". If the material covers two unrelated areas, that is two
  top skills, not one with an "and".
- **L1 — sub-skills.** The branches a top skill routes to (per kind, per mode,
  per phase). `advertised: false`, named from the parent. **Prefer a sub-skill
  over a reference**: a sub-skill is itself an md file with its own L0
  description, so the LLM can judge whether to descend; a reference is opaque
  until fully read. Reach for a sub-skill whenever the branch contains a
  *procedure or decisions*.
- **L2+ — deeper sub-skills.** Recurse where a branch itself splits.
- **References** — only for **passive** material: a schema, a lookup table, a
  catalog, a long example corpus. No procedure, no decisions. If you are
  tempted to make a reference that says "to do X, first…", it is a sub-skill.
- **Templates** — fill-in scaffolds the creation step copies (a skeleton file,
  a frontmatter stub). List them separately from skills.

## For every item, fix three things

1. **Verdict** — extend `<existing-uri>` or create new (from the overlap check
   above). Extends are edits, not new tree nodes.
2. **Trigger mode** — what causes it to load. For a top skill: the user/agent
   situation in its description. For a sub-skill: which parent routes to it and
   on what condition ("the task is long-running → load …"). For a reference:
   which skill body names it.
3. **Knowledge placement** — which slice of the source knowledge goes into this
   item, and — for a skill — what of that becomes the **description** (the
   trigger) vs the body (the procedure). Knowledge with no home is cut, not
   parked; knowledge in two homes is a link, not a copy.

Plus one decision for the whole set: **home** — a sidecar's `embed/skills/`
(which sidecar; needs a rebuild) or the external `<workspace>/.katyoti/skills/`
(runtime, no rebuild). Ask if the user hasn't said; default to `.katyoti/skills/`
when absorbing the user's own material.

## The layout plan

Produce one approvable document before any file is written:

- the tree (L0 → L1 → L2…), each node with its `name` + draft description
- the reference list, each with the skill that names it
- the template list
- a per-item table: item · kind (top-skill / sub-skill / reference / template) ·
  verdict (extend/create) · trigger mode · knowledge placed
- the **home** for the set (sidecar embed / `.katyoti/skills/`)
- the **coverage check**: every capability in the source maps to exactly one
  item (or an extend of an existing one); nothing is dropped silently and
  nothing is invented

Present the plan for approval (`request-validation` kind `approve` when it is offered, else `ask-user` confirm). On approval, create each item with
`ktw-compose:/skill/create-skill` (which loads `skill-format` per file).

## When the source is one existing skill

The job is usually **decomposition**: a fat skill becomes a top skill + the
sub-skills its sections imply, plus references for any passive tables it inlined.
Preserve its working description as the top skill's; split the body by trigger.
