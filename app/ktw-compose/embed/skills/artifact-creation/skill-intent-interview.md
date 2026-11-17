---
name: skill-intent-interview
description: "Clarify what a new skill must be before writing it: its single job, its trigger, its anti-triggers, and the knowledge it carries. Load at the start of creating a skill from a vague request."
advertised: false
hashtags: [skill, authoring, interview, create-items]
category: guide
---

# Clarifying a skill's intent

Before a skill is written, its **identity** must be sharp. A skill lives or dies
on one thing: whether an LLM, seeing only its one-line description, can decide
to load it at the right moment and skip it otherwise. Everything else is
recoverable by editing the body; a fuzzy identity is not. So the interview's
job is to extract a sharp identity, and the single most important output is a
**description that says what the skill does AND when to use it**.

Interview methodology — open before closed, one question per round, summarize
before confirming, 3-5 rounds — is the `ktw-methods:/skill/interview-technique`
skill. Load it and apply it. This skill adds **what to ask about for a skill**.

## What we must know (in priority order)

1. **The one job.** What single class of task does this skill make an LLM able
   to do? If the answer needs "and" for two unrelated jobs, it is two skills —
   surface that and split.
2. **The trigger.** In what situation should the LLM reach for it? Capture the
   real phrasings a user/agent would be in the middle of ("use WHEN the user
   asks to …", "WHEN about to edit a task file"). The trigger is the seed of the
   description.
3. **The anti-trigger.** What nearby situation looks similar but should NOT load
   it? This is what keeps it distinct from sibling skills. Probe for the
   confusable neighbor.
4. **The procedure + its decisions.** What are the steps, and where does the LLM
   have to *choose*? No decision point → it is a reference or a tool, not a
   skill. Confirm a skill is even the right artifact (the `skill-format` skill
   has the "when NOT to write a skill" table).
5. **The knowledge carried.** What does the LLM not already know that this skill
   supplies? Names of tools/resources to call, limits, gotchas, the shape of a
   good output. Thin is fine; padding is not.
6. **Surfaces + exposure.** Does it need to run on the agent loop, the `llm`
   CLI, or be exposed externally (`provides`)? Default: both internal surfaces,
   no external exposure.
7. **Depth.** Is this one leaf skill, or a top skill that routes to sub-skills?
   If the job has distinct sub-cases (per kind, per mode), it is multi-level —
   note the branches; the layout is planned in
   `ktw-compose:/skill/skill-layout-planning`.
8. **Existing overlap.** Scan `skills list` — does a skill already claim this
   trigger? If close, the answer may be *extend it*, not create new. Capture the
   verdict (extend `<uri>` | create new) so the driver doesn't add a duplicate.
9. **Home.** Where does it live — a sidecar's `embed/skills/` (ships in the
   binary, needs a rebuild) or the external `<workspace>/.katyoti/skills/`
   (runtime-loaded, no rebuild, workspace-scoped)? The user states it or you
   ask — never assume. Default lean: `.katyoti/skills/` while prototyping or
   when built from the user's own material; a sidecar embed when a capability
   clearly owns it.

## Description-first

Draft the description **during** the interview, not after. Read it back to the
user as part of the round-4 summary — if they can't tell from one sentence what
it does and when they'd use it, the identity isn't sharp yet, so probe more.
A good description is: short (one sentence where possible), **what + when**
("Does X. Use when Y."), and distinct from siblings (no shared vocabulary, or
the LLM can't pick between them).

## Output — the skill intent brief

Produce a structured brief the creation driver consumes:

- **name** — kebab-case, candidate
- **description** — the what+when sentence (the deliverable)
- **anti-triggers** — situations that must NOT load it
- **procedure** — the steps + where the LLM decides
- **knowledge** — what the body must supply (tools/resources by URI, limits)
- **surfaces / provides** — exposure needs
- **shape** — single leaf, or top + named sub-cases
- **verdict** — extend `<existing-uri>` or create new
- **home** — a sidecar's `embed/skills/` (which?) or `.katyoti/skills/`

Confirm the brief with the user (`request-validation` kind `approve` when it is offered, else `ask-user` confirm) before any file is written.
The driver — `ktw-compose:/skill/create-skill` — takes it from here.
