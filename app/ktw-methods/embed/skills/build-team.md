---
name: build-team
description: >-
  Assemble a coordinated team from the 6 baseline personas and map it to
  an increment with capacity — the SAFe Release-Train move. Use when the
  user wants to "set up / staff / build a team" for a project or
  increment, or assign who works on what. NOT for delegating one sub-task
  (call delegate / see the delegation-modes skill directly), NOT for
  pulling the backlog (plan-increment), NOT for defining goals.
icon: "👥"
iconName: users-group
hashtags: [team, personas, increment, capacity, delegation, release-train]
category: lifecycle
---

# Build a team for an increment

Stand up a working team from the methods pack's six baseline personas
(specifier / architect / coder / reviewer / tester / documenter) and map it
onto the increment's committed cards with a capacity budget — lean-increment-flow
§5 (the Release-Train assignment step).

> **Boundary:** the *orchestration mechanics* (how agents are spawned, how they
> message, isolation) live in the **ktw-agents engine**, not here. This skill
> REFERENCES them — load `ktw-agents:/skill/delegation-modes` for the 9 spawn
> presets and `ktw-agents:/skill/topic-conventions` for inter-agent messaging.
> Build-team decides *who + on what + how much*; the engine skills decide *how*.

## What a "team" is here
A **parallel work-stream**: an agent-team, a persona-stream, or a focused
you+assistant session. Each team has an **effort capacity** (sum of card
effort it can take this increment). Start with one stream; add teams only when
work genuinely fans out.

## Procedure

1. **Read the increment.** Take the committed cards (from `plan-increment`) and
   their total effort. If no increment is planned yet, run `plan-increment` first.
2. **Pick the roles needed.** From the 6 baselines, select the ones this work
   needs (a spec-heavy increment leans specifier+architect; an implementation
   increment leans coder+tester+reviewer). Each persona keeps its default gate.
3. **Choose the delegation mode per role** — from `ktw-agents:/skill/delegation-modes`:
   typically a `coordinator` (foreground) driving `worker`/`teammate` streams, or
   a `team-leader` with `spawn-teammate` + group messaging for peer collaboration.
   Don't re-derive the modes — cite the one that fits.
4. **Assign cards to teams within capacity.** Map each committed card → a team;
   keep each team's assigned effort ≤ its capacity. Surface team-load vs capacity.
5. **Wire coordination** (when >1 stream) via `ktw-agents:/skill/topic-conventions`
   (direct `to.<name>` / team `group.<team>` channels). Reference, don't reinvent.
6. **Confirm + record.** Present the team→cards→capacity map; on confirm, record
   it on the increment (team assignment is increment-scoped, not a backlog field).

## Done when
- Each committed card has an owning team within that team's capacity.
- Each team's roles + delegation mode are named; coordination channels are cited
  for multi-stream teams.

## Don't
- Don't re-implement delegation modes / topics in this skill — reference the
  ktw-agents engine skills (they own the mechanics).
- Don't assign past a team's capacity, or assign cards that weren't pulled into
  the increment.
- Don't use this to delegate a single sub-task — that's the `delegate` tool +
  `delegation-modes` directly.
