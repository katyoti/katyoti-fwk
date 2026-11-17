---
name: new-functionality
description: >-
  Help a user (or persona) shape a new functionality idea into a
  properly-structured feature tree (feature → sub-features → tasks),
  driven by Socratic questioning to surface real goals (not pre-baked
  solutions), and grounded in what already exists in the workspace.
  Use when the user says "I want to add X" or "we need a feature
  for Y." Refuses to jump to implementation until goals are explicit.
icon: "💡"
iconName: lightbulb
hashtags: [feature, goals, specifier, bootstrap, ideation]
category: lifecycle
provides: [task]
---

# New Functionality — Goal-Driven Feature Bootstrap

This skill drives a user from a raw idea ("I want X") to a CWM-shaped
feature tree (a `feature` object + child features + initial
tasks), with goals made explicit before any solution is proposed.

It is a **conversational skill**, not a verb. It loads when invoked
and walks the user through a sequence of questions. It writes nothing
until the user confirms. It never proposes a code change.

> **The hard rule**: this skill REFUSES to discuss "how" until "why"
> is recorded. If the user pre-baked a solution, the skill asks
> "what would happen if you didn't do that?" and records the actual
> goal underneath.

## Procedure

### Phase 1 — Listen and surface the real goal

Open with:

> "Tell me what you want to add or change. Don't explain how — just
> what you want the outcome to be."

Then probe with **goal-extractor questions** (use 2-3 from the list,
not all):

- "Who would notice the change? What would they see?"
- "What would be different the day after it's done?"
- "If we only built half of it, which half is non-negotiable?"
- "Has someone hit a wall today that this would unblock?"
- "Is this 'we should have it' or 'we're losing something without it'?"

Capture the answers verbatim. Do NOT paraphrase yet.

### Phase 2 — Scan what already exists

**Before proposing any new objects**, query CWM for relevant context:

1. Call `cwm-object-list(parent=<the target object>, type=feature)` (or
   `cwm-object-tree` from it) for existing features near the target.
2. Call `cwm-object-list(module=<the module>, type=goal)` — get the module's goals.
3. Call `object-effective(id=<the focused object>, type=rule)` — get applicable
   constraints.
4. Call `cwm-object-search(query=<keywords from the user's input>,
   types=[feature, goal, decision])`.

Report back:

> "Before I propose anything new, here's what already touches this
> area: [list]. Does your idea overlap with any of these, or is it
> genuinely new?"

If overlap: walk the user through whether to **extend an existing
object** (preferred) vs **create a new sibling**. Update wins over
proliferation.

### Phase 3 — Distinguish goal from solution

The user's phrasing usually mixes them. Examples to disambiguate:

| User says | Goal (the why) | Solution (the how) |
|---|---|---|
| "We need a button to export PDF" | Users need to share results with people outside the system | A button on the report page |
| "Add a cache" | The N-second wait makes the dashboard unusable | LRU cache on the query layer |
| "Migrate to NATS" | We can't scale to 5 instances; one process binds queues | NATS-based event bus |

Restate the goal back to the user in plain prose, and ask:

> "Is THIS the goal, and the rest is one possible solution?
> Or is the implementation itself the goal because of [X constraint]?"

### Phase 4 — Propose CWM objects (PROPOSE, don't write)

Build a draft list:

- **0 or 1 `goal` object** — added at module or top-feature level if no
  existing goal covers it
- **1 `feature` object** — the top-level feature for this
  functionality
- **2-N child `feature` objects** under it (`parent`
  = the feature), each independently testable
- **0+ `task` objects** under each feature, only if the user has them in
  mind already (otherwise the planner / specifier fills these in)
- **1 `serves` edge per feature → the goal(s) it serves**
  (method-core alignment, M:N). Delivery links (`wants` a use-case, which
  `lives-in` a `module`) come LATER — when the feature is broken into its
  checks — NOT here. There is no `realized-by` edge (removed; use `wants`).

Present the draft as a single block. Ask:

> "Here's what I'd propose to create. Walk me through anything that
> feels wrong: missing context, wrong scope, something that should already
> exist somewhere."

### Phase 5 — Write objects, OR exit unwritten

After the user confirms (or modifies):

- Call `cwm-object-create` for each accepted object
  (goal → feature → child features → tasks in that order, so
  parents and edges resolve), e.g.
  `cwm-object-create({ module: <the module>, object: { type: "feature", name: <title>, parent: <parent object id>, description: <the goal-framed outcome> } })`
  — omit `id` to have one minted.
- Add `serves` edges: `cwm-edge-create({ kind: "serves", from: <feature id>, to: <goal id> })`
  for each feature → the goal it serves.
- Print the list of created ids and their paths.
- Stop. The skill's job is done. Hand off to:
  - `specifier` persona (refine features into DoD)
  - or `architect` persona (decide which `module` will own the feature's
    use-cases, via `wants` / `lives-in`, if not already decided)

If the user is NOT ready to commit: print the proposal as a markdown
block they can save manually, and exit without writing.

## Rules this skill follows

- **Goal before solution** — always. If the user's input is
  solution-shaped, surface the underlying goal explicitly.
- **Scan before propose** — never propose a new object without checking
  what already exists. Update wins over proliferation.
- **One goal, one feature** — if the user wants two things, the skill
  asks them to pick one (or runs twice). No multi-goal features.
- **Atomic features** — each feature must be independently testable
  and deliverable. If one "depends on the previous one to make sense,"
  split it differently.
- **No implementation commitments** — the skill never agrees to a tech
  choice, placement, or implementation detail. Those are the
  architect/coder personas' job.

## When NOT to use this skill

- The user has already filed the feature and wants to refine it
  → use the `specifier` persona instead.
- The user wants to **retro-engineer existing code** into objects → use
  the language pack's onboarding skill instead.
- The user is debugging or hotfixing → this is a `task` against an
  existing UC, not a new feature.
- The user wants to decompose a delivered thing → use the `architect`
  persona; this skill operates on the souhait (backlog) side.

## Output format

When the skill writes objects, it prints a confirmation block:

```
Created in module reports:
  goal:     reports.G-003-shareability          (new)
  feature:  reports.F-012-pdf-export
  feature:  reports.F-013-pdf-export-button     (under reports.F-012-pdf-export)
  feature:  reports.F-014-pdf-export-cli        (under reports.F-012-pdf-export)
  serves:   reports.F-012-pdf-export → reports.G-003-shareability

Delivery edges (`wants` use-cases) come later, when the feature is broken into checks.

Next step: spawn the `specifier` persona on reports.F-013-pdf-export-button
to write its DoD-per-UC.
```
