---
name: cwm-personas
description: >-
  The persona library mechanism — 3-tier resolution (workspace > universe
  > built-in), TASK.md persona format, bind modes (node / global / custom),
  and how PersonaService.Spawn integrates with the agent loop. Substrate-level
  reference; library contents ship with each method pack.
iconName: user-circle
icon: "👥"
hashtags: [cwm, persona, spawn, library]
category: cwm
advertised: true
---

# CWM Persona Library

The persona library is the `personas` extension's `PersonaService` (`persona.Resolve` / `persona.List` / `persona.Spawn`; CLI `ktw-agents persona resolve|list|spawn`). It ships zero personas itself: method packs (ktw-gocode, ktw-methods) ship persona definitions as catalog resources, and the service reads them from the catalog.

> Only the built-in tier is read today: the service resolves personas from the
> catalog. The workspace and universe override tiers below are the design, not
> yet implemented.

## The 3 tiers

| Tier | Path | Wins | Reset by |
|------|------|------|----------|
| Workspace | `<root>/personas/<name>/TASK.md` | Last (overrides universe + built-in) | User commit |
| Universe | `<universe-home>/personas/<name>/TASK.md` | Middle | Universe rebuild |
| Built-in | binary-embedded `personas/<name>/TASK.md` | First (the floor) | Binary release |

The resolver picks the closest match. To customize an inherited persona, drop a `TASK.md` at the workspace tier; do NOT edit the binary copy.

## Persona TASK.md frontmatter

```yaml
---
name: reviewer
type: agent           # persona = agent task
category: specialist  # specialist | manager
description: >-
  Reviews code changes against the workspace's effective decisions + DoD.
icon: 🧐
types: [project, module]   # the object types it may target; empty = any
bind_mode: object     # object | global | custom
default_gate: code-reviewed
skills:
  - cwm-graph-navigation
  - cwm-search
  - safe-code-review     # method-pack skill
---

# Reviewer Persona

You are a code reviewer …
```

## Bind modes

- **`object`**: spawn requires a `target` object. `types` lists the object types the persona is meant for (empty = any); the persona list filters by it (`type`).
- **`global`**: spawn runs without a target (e.g. a system-health auditor).
- **`custom`**: the persona's `TASK.md` body defines its own bind logic.

## Spawning

`persona.Spawn(name, target, gate, inputs)` (CLI `ktw-agents persona spawn`) materializes a sub-agent. The service:

1. Resolves the persona from the catalog.
2. Checks the bind mode: `object` (and `custom`) require a `target`, else the spawn is `rejected` with a reason.
3. Allocates a `spawn_id` (UUID v7).
4. Runs the agent loop through the TaskService with the persona's TASK.md body as the prompt; `inputs` travel as the task input plus `_target`, `_gate`, `_spawn_id`, `_parent_spawn_id`.
5. Returns `running` with the task id, or `queued` when no TaskService is reachable.

## Default gate

When `gate` is omitted, the persona's `default_gate` from frontmatter is used. The gate is a DoD criterion ID (namespaced: `<owner-node-uuid>.<local-id>`); when the persona completes successfully, that criterion is flipped to `checked` in the focused node's `dod-status` item.

## Skills attachment

The persona's `skills:` list determines which skills the agent loop preloads. Skills are NOT loaded automatically based on the persona's category or node type — they must be listed explicitly. Method packs typically expose a base set of skills via their own skill library; the persona picks which subset it needs.

## What to read

- [cwm-knowledge-capture](cwm-knowledge-capture.md) — DoD-status.
- [cwm-onboarding](cwm-onboarding.md) — the substrate mental model that personas operate inside.
- Method packs (e.g. ktw-safe) for the ACTUAL persona TASK.md files agents will encounter.
