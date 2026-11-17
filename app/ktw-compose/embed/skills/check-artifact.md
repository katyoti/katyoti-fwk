---
name: check-artifact
description: "Static-quality dispatcher for any ktw artifact; load to check, lint, or validate that a skill, task, type, data-type, or reference is well-formed and clean without running it."
advertised: true
hashtags: [check, lint, quality, static, dispatcher]
category: development
---

# Artifact check

The single entry for **static** quality: does an artifact meet its contract by
*reading* it — well-formed, every reference resolves, no legacy or tracking
cruft? It never executes anything. This is the cheapest gate; run it before
spending a token on behavior.

Two siblings sit next to it — keep the boundary crisp:
- **Behavioral** verification (does it WORK when run) → `ktw-compose:/skill/test-artifact`.
- **Whole-sidecar** audit (usage road + quality + docs + proposals) →
  `ktw-compose:/skill/audit-sidecar` (it calls THIS skill for its quality phase).

If the ask is "run it / smoke it / does it behave", stop and go to
`test-artifact`. This skill only answers "is it *shaped* right".

## 1. Identify what to check

Name the target. It is one of:
- **a single file** — check just it.
- **a set** (several files, or "the skills that changed") — check each.
- **a whole folder** (`embed/skills/`, a sidecar's `embed/`) — check every
  artifact in it, grouped by kind.

Determine each artifact's **kind** from its location and shape: a
`skills/*.md` with skill frontmatter is a skill; `tasks/*.md` a task;
`cwm/object-types/*.yaml` a CWM object type; `cwm/<folder>/*.yaml` another
pack declaration (capability, aspect, workflow, gate, policy, hook,
object-panel, agent-context); `data-types/*.dsl.yaml` a data-type; a
`*.i18n.yaml` the translations of the file beside it; `references/*` a
reference. A `type: agent` task is still a task
(check it as one). **If the kind is genuinely ambiguous, ask** — do not guess and
run the wrong checklist.

## 2. Route to the specialized check

One check owns each kind. Get-instruction it and run its checklist against the
artifact:

| Artifact kind | Check |
|---|---|
| **skill** (incl. agent-design/knowledge skills) | `ktw-compose:/skill/check-skill` |
| **task / agent** (any TASK file) | `ktw-compose:/skill/check-task` |
| **persona** (agent-loop TASK.md bound to objects) | `ktw-compose:/skill/check-persona` |
| **object type** (CWM YAML) | `ktw-compose:/skill/check-object-type` |
| **edge-type** (CWM YAML) | `ktw-compose:/skill/check-edge-type` |
| **data-type** (schema DSL) | `ktw-compose:/skill/check-data-type` |
| **reference** (passive lookup doc) | `ktw-compose:/skill/check-reference` |
| **capability** and its pack pieces (aspect, workflow, gate, policy, hook, agent-context) | the rules of `ktw-compose:/skill/create-capability` §3–§5, plus `lint-universe` for scope |
| **object panel** | the rules of `ktw-compose:/skill/create-object-panel` §2–§6 |
| **guide recipe** (`embed/guide/*.yaml`) | `lint-guide-recipe --file <path>` clean; a step names each control `{control:<anchor>}` (`{control:<anchor>\|<name>}` for an instance), the anchor taken from the GUI's registry `app/ktw-gui/frontend/src/guide/anchors.ts` — never its label, never in bold (the GUI says the label in the person's language) |
| **translations** (`*.i18n.yaml`) | `ktw-compose i18n <pack> --lang <l> --missing`: `complete`, no `errors` |

The mechanical part of several checks is a CLI lint — run it first and read
its findings into the checklist: `ktw-compose lint-skill | lint-task |
lint-reference | lint-persona | lint-guide-recipe --file <path>`,
`lint-sidecar --exe <bin>` for a compiled sidecar, `lint-universe --bin-dir
<dir>` for references and gates across sidecars.

A **config** file has no dedicated check yet: verify inline that it is valid YAML,
its type name matches the file name (`config/<ConfigName>.yaml`), and it carries
no tracking cruft. A **files-only sidecar as a whole** is not one artifact — that
is `audit-sidecar`.

## 3. Aggregate — and separate intent from defect

For a set or a folder, collect every per-kind verdict into ONE findings table:
`| Artifact | Kind | Verdict (clean / N findings) | Findings (each with file:line + fix) |`.
Then add a **cross-cutting patterns** section — issues that repeat across many
artifacts (the same stray frontmatter key across a family, a whole cluster
carrying task-tracker/`US-N` cruft, a staging folder every consumer points at).
One systemic fix clears many rows, so patterns rank above individual findings.

**Never flag a deliberate choice as a defect.** `advertised: false` on a
routed sub-artifact, `preview: true` on a not-yet-built action target, and a
zero-method in-process-accessor service are intentional. Only unmarked breakage
is a finding.

## Rules

- Static only. If answering the question needs a run, it is `test-artifact`,
  not this skill.
- Every finding cites a `file:line`. Verdicts are binary — never soften a ❌ to a
  maybe.
- One check per kind — route, never re-describe a per-kind checklist here.
- When checking a whole folder, this is a lint sweep, not an audit: it does NOT
  trace the usage road or reconcile docs. Reach for `audit-sidecar` when the ask
  is "is this sidecar any good", not just "is it well-formed".
