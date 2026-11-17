---
name: check-skill
description: "Static checklist for one skill; load to validate its frontmatter, resolvable references, hygiene/no-legacy, and naming before shipping or when auditing for drift."
advertised: false
hashtags: [check, skill, lint, audit, hygiene]
category: development
---

# Skill check

A static checklist for ONE skill artifact. It runs without executing anything —
read the file, resolve what it points at, report pass/fail per item with a
`file:line` for every finding. Works the same on a skill you just authored and
on one that shipped months ago (audit mode): the checklist is the contract, the
artifact either meets it or it doesn't.

This is the cheapest gate. It does NOT run the skill (that is
`ktw-compose:/skill/test-artifact`) and does NOT score quality
(`ktw-compose:/skill/skill-evaluation`). It checks the artifact is *well-formed,
resolvable, and clean* — the things that are true or false by reading, not by
running. The contract it checks against is `ktw-compose:/skill/skill-format`.

## The checklist

### 1 — Frontmatter contract
- [ ] Only the allowed keys are present (per `skill-format`); no stray keys.
- [ ] `name:` is kebab-case and **equals the file stem and the URI segment**
      (`<provider>:/skill/<name>`). File `foo-bar.md` ⇒ `name: foo-bar`.
- [ ] `description:` states **what it does AND when to load it** — a reader can
      decide relevance from the description alone. Not just a title restated.
- [ ] `advertised:` is correct for the role: a top-level user-facing skill is
      advertised (or omits it, default true); a sub-skill routed only by its
      parent is `advertised: false`.

### 2 — Structure
- [ ] Exactly one H1, naming the skill.
- [ ] Progressive disclosure holds: L1 body is self-contained; deeper material
      is split into references the body links, not inlined into a wall.
- [ ] If it is a dispatcher, every branch it lists routes to a sub-skill that
      **exists**; if it is a sub-skill, its parent actually links it.

### 3 — Everything it references resolves
- [ ] Every `<provider>:/skill|reference|data-type/...` URI in the body
      resolves to a real, advertised-or-loadable artifact (no dangling URI).
- [ ] Every `home` / file path it names exists (or is clearly a template
      placeholder, not a claim).
- [ ] Every tool it names in bold is a real tool in the catalog for its mode —
      a skill that says "call **get-forecast**" when no such tool exists sends
      the model flailing.

### 4 — Hygiene: no legacy, no tracking cruft
- [ ] **No task-tracking anywhere in the body** — no `US-N`, `Phase X`,
      `ms-NNNN`, ticket ids, or `added YYYY-MM-DD`. Tracking lives in commits and
      the module's requests, never in a shipped artifact.
- [ ] **No history / provenance prose that has no functional effect** — "this
      was renamed from X", "previously did Y", "kept for the old flow". A skill
      describes what to do NOW; the why belongs only where it changes behavior.
- [ ] No dead cross-references to deleted artifacts, no "TODO"/"FIXME" left in
      the shipped body, no commented-out alternate instructions.
- [ ] Comments/notes describe **what / why**, not **when / who** — same rule as
      code.

### 5 — Naming convention
- [ ] If this is an **action** skill, its name follows
      `<targeted-type>-<action>` (e.g. `create-skill`, `task-testing`,
      `<type>-check`). If it is **knowledge** (a patterns/lessons reference),
      it is named for its topic and is `advertised: false` — and ask whether it
      should be a `reference` contribution rather than a skill at all.

## Output

Report the checklist with each item ✅/❌ and, for every ❌, the `file:line` and
the one-line fix. End with a verdict: **clean** (ship/keep), or **N findings**
(fix these first). Never soften a ❌ into a maybe — the checklist items are
binary by construction.
