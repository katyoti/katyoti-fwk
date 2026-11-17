---
name: create-artifact
description: "Entry point for creating any files-only ktw artifact (skill, task, agent, reference, config, CWM type); routes to the per-kind skill. Load to author or extend a katyoti solution without writing Go."
iconName: package-plus
icon: "📦"
hashtags: [development, creation, create-artifact, files-only]
category: development
---

# Artifact creator

`ktw-compose` is the **files-only** creator. You stand up or extend a katyoti
solution by authoring files — skills, tasks, agents, personas, references,
configs, CWM type assets, embedded assets — and by scaffolding a **files-only
sidecar** (manifest + `embed/`, no Go business logic) or adding files to an
existing sidecar's `embed/`. You never write Go here.

**If it needs Go — a tool, a service, real domain logic — that is the
code-bearing track (`ktw-code` / `ktw-gocode`), not this surface.** Recognise it
and hand off; do not try to produce Go from here.

**When you are a delegated worker** (an orchestrator handed you ONE artifact to
build): produce that artifact at its `home` and return. Do NOT verify, lint, or
mark anything `done` — you have no such tools, and the orchestrator owns the
gate. The criteria you were given are your TARGET to satisfy in the artifact, not
a checklist for you to run. Never fall back to `scripts-run` / `bash` to invoke a
tool you don't have — report what you produced and let the orchestrator verify.

Three decisions come before any per-kind skill: **extend or create**, **which
artifact**, and **where it lives**. Get those right, then route.

## 1. Extend what exists, or create new?

Before anything, look at what is already there — adding a near-duplicate is the
most common mistake. List the live surface: the available skills, the contributed tasks,
`ktw-univers scan-inventory` for binaries, and every contribution universe-wide.

- **Working from existing material** (an existing skill, docs to convert)? The
  overlap check is mandatory. For each capability decide: **extend** the
  existing artifact (add a sub-skill, widen scope, append a reference) or
  **create new** (different trigger / different job).
- Prefer extending when it is the *same job, more depth*. Create new only when
  the trigger is one no existing artifact claims.
- Record the verdict per item (`extend <uri>` | `create new`) and carry it into
  the layout plan (`ktw-compose:/skill/skill-layout-planning`) for bulk material.

## The files-only artifacts

| Artifact | What it is | The tell |
|---|---|---|
| **skill** | on-demand knowhow: a *procedure with decisions* an LLM loads | "when situation S, here's how to proceed" — no code |
| **task** | a unit of work launched + tracked by an engine (TASK.md) | "run this end-to-end and give me the result" |
| **agent** | a task whose engine is an LLM loop (`type: agent`) | a task that must *reason*, use tools, iterate |
| **persona** | an agent-loop task + binding metadata (CWM persona library) | a reusable agent role you spawn on objects |
| **reference** | passive lookup material: a schema, a table, a catalog | no procedure, no decisions — facts to read |
| **config** | a YAML config file a binary reads | settings, profiles, allow-lists |
| **CWM type** | an object-type / edge-type YAML asset | a new kind of object, or of relation, in the workspace |
| **capability** (and its pack pieces) | a capability, aspect, workflow, gate, policy, hook or agent-context YAML | a new ability objects *have*, rules over them, states they move through |
| **object panel** | an `object-panels/` YAML: a tab, or a Global view | "show this on those objects", a dashboard with acts |
| **embedded asset** | any file shipped under a sidecar's `embed/` | templates, fixtures, data |
| **files-only sidecar** | scaffold a new `ktw-*` binary: manifest + `embed/`, **no Go business logic** | a fresh files-only pack that needs its own binary identity |

**Not here — these need Go → `ktw-code` / `ktw-gocode`:** a **tool** (a service
method), a **service** (domain logic), a **code-bearing sidecar** (Go business
logic). If the request is one of these, say so and route to the code track.

## 2. Pick the artifact — the selection interview

Do not assume from the user's noun ("make me a tool…" often means a skill).
Interview to the real need — apply `ktw-methods:/skill/interview-technique`
(open → probe → confirm), asking only what disambiguates:

- **Knowing or doing?** Knowing *how* (a procedure an LLM follows) → **skill**.
  Just facts to look up → **reference**. *Doing* (compute, fetch, mutate data in
  code) → that is a **tool/service = Go → code track**, not a files-only artifact.
- **Who invokes it, and when?** An LLM mid-task reaching for guidance → skill. A
  user/GUI/agent launching tracked work → **task** (→ **agent** if it must
  reason). A reusable agent role bound to a node → **persona**.
  - Often it is **both**: a task whose how-to is reusable domain knowledge
    splits into a **skill** (the knowledge — reusable, independently
    discoverable) plus a thin **task** that just loads it. Put the knowledge in
    the skill; never duplicate it in the task body. See
    `ktw-compose:/skill/create-task`.
- **Is it a shape in the workspace graph?** → a **CWM type**; an ability
  several kinds of object share → a **capability**; something to see on some
  objects → an **object panel**. The reference extension, ktw-demo
  (`app/ktw-demo/embed/cwm/`), shows each one.
- **Does this files-only pack need its own binary identity?** → scaffold a
  **files-only sidecar**; otherwise add the files to an existing sidecar's
  `embed/` or the workspace folder.

Confirm the chosen kind with the user before creating. If it turns out to need
Go, stop and hand off to the code track — do not improvise Go here.

## 3. Decide where it lives

Every artifact has a home; choose deliberately.

- **The user said where** → honor it.
- **A sidecar owns the capability** → author it in that sidecar's
  `app/<sidecar>/embed/skills|tasks|references|...`. It ships in the binary,
  available wherever it runs, but needs a **rebuild** (`make <sidecar>`). Durable,
  product-level artifacts. When that sidecar's source is in THIS workspace,
  iterate without rebuilding: author into the overlay, test live, promote when
  validated — load `ktw-compose:/skill/overlay-iteration`.
- **Workspace-specific, prototyped, or built from the user's own material** →
  the external `<workspace>/.katyoti/skills/` (tasks `.katyoti/tasks/`). Loaded
  by the host at **runtime, no rebuild** — fast to iterate, scoped to the
  workspace. Default while shaping, or when no sidecar owns it.
- **Especially from existing material**, placement is rarely obvious — surface
  the choice and ask: which sidecar's `embed/`, or `.katyoti/skills/`.

## 3b. The executable surface

The scaffold/emission ACTIONS are compose tools (`ktw-compose:compose/…`:
create-sidecar, create-skill, create-task, create-persona, create-reference,
promote, the universe/membership verbs, apply-universe). The per-kind skills
below tell you WHEN and WITH WHAT content to call them — stub via the tool,
author via the skill. The create verbs take `target: "overwrite"` to write into
the live workspace overlay instead of the source tree (no rebuild); `promote`
folds an overlay artifact back into source. See
`ktw-compose:/skill/overlay-iteration`.

## 4. Route to the per-kind skill

| Creating a… | Load |
|---|---|
| **skill** | `ktw-compose:/skill/create-skill` (foreign-format or raw-knowledge sources route through `ktw-compose:/skill/skill-import`; ktw-shaped material through `ktw-compose:/skill/skill-layout-planning`) |
| **task** | `ktw-compose:/skill/create-task` |
| **agent** | `ktw-compose:/skill/create-agent` |
| **files-only sidecar** | `ktw-compose:/skill/create-sidecar` (scaffold only — manifest + `embed/`, no Go logic) |
| **reference** | no separate skill — author it alongside the skill that names it, per `ktw-compose:/skill/skill-format` |
| **CWM object type** | `ktw-compose:/skill/create-object-type` |
| **capability**, aspect, workflow, gate, policy, hook, agent-context | `ktw-compose:/skill/create-capability` |
| **object panel** (a tab, a page with acts, a Global view) | `ktw-compose:/skill/create-object-panel` |
| **translations** of any of these (`<name>.i18n.yaml`) | `ktw-compose:/skill/translate-contributions` |
| **CWM edge type** | `ktw-compose:/skill/create-edge-type` |
| **persona** | `ktw-compose:/skill/create-persona` |
| **data-type** (schema DSL) | `ktw-methods:/skill/author-schema` |
| **UI form / view / table** of a data-type | `ktw-methods:/skill/drive-method-ui` (the UI methodology; `author-ui-form` / `author-ui-view` / `author-ui-table` author each surface) |
| **config** | no dedicated skill yet; follow the closest existing config in a neighbouring sidecar's `embed/config/` |

Each per-kind skill owns its own references and its own run/check/fix loop — the
dispatcher only gets you to the right one. Hand off and follow it. For the
**check** step of that loop — is what I authored well-formed against its
contract? — route to `ktw-compose:/skill/check-artifact` (the single owner of the
static per-kind checks); it is the same gate `audit-sidecar` uses on shipped
artifacts.

## Rules

- ktw-compose writes **files, never Go**. A tool / service / Go-bearing sidecar
  is the code track — recognise it and hand off.
- Decide **extend-or-create**, **which artifact**, and **where it lives** before
  writing any file.
- When working from existing material, the overlap check is mandatory: extend
  before you duplicate.
- Read the per-kind skill before creating; follow existing patterns, don't invent.
- Test every creation with a real run (or, for a skill, a real trigger) before
  calling it done.
