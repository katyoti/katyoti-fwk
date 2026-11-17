---
name: audit-artifact
description: "Generic audit entry for any ktw artifact or whole sidecar; load to produce a read-only quality report with ranked change proposals, dispatching by kind and never applying changes."
advertised: true
hashtags: [audit, quality, proposals, dispatcher]
category: development
---

# Audit artifact

The generic **audit** entry, mirroring `compose-artifact`: name a subject, and it
dispatches by kind to produce a **read-only quality report with ranked change
proposals**. `audit` is the top rung of the static ladder —
`lint` (deterministic) → `check` (LLM checklist) → **`audit`** (check + proposals).
It proposes; it never applies. Applying is `improve-artifact` (the user-gated
loop).

The subject can be a single artifact OR a whole **sidecar** — a sidecar is just
the largest artifact.

## 1. Identify the subject and its kind

Name what to audit and determine its kind from location/shape: `skills/*.md` →
skill; `tasks/*.md` → task; `cwm/object-types/*.yaml` → object type;
`data-types/*.dsl.yaml` → data-type; `references/*` → reference; a whole
`app/<name>/` pack → **sidecar**. If the kind is genuinely ambiguous, ask — do
not guess.

## 2. Dispatch

| Subject | Route |
|---|---|
| **sidecar** (whole pack) | `ktw-compose:/skill/audit-sidecar` — the 4-phase whole-unit audit (usage-road trace → per-artifact check → doc reconciliation → Pareto proposals). Return its report; you are done. |
| **any single artifact** (skill / task / type / data-type / reference) | the two steps below |

For a single artifact:

1. **Static quality** — run `ktw-compose:/skill/check-artifact` on it (the static
   dispatcher routes to the per-kind `check-<kind>` checklist). This is the
   evidence base: every finding with a `file:line`.
2. **Behavioral quality, if runnable** — for a skill or task, add an
   `evaluate` summary of how it performs against its stated purpose
   (`ktw-compose:/skill/skill-evaluation` scores a skill; for a task, score the
   run against its goal). Passive kinds (reference, data-type) skip this — there
   is nothing to run.

## 3. Report + propose — never apply

Assemble one report: what was audited, the check findings (evidence), the
evaluate summary (if any), and a **Pareto table of proposals** sorted by
impact/effort — each with its `file:line`, category, effort, and regression risk.
Cross-cutting patterns rank above individual rows; one systemic fix clears many.

Stop there. `audit` is read-only. To apply the proposals, hand the report to the
user and run `ktw-compose:/skill/improve-artifact` — it owns the
"user decides every change" gate and the re-measure loop.

## Rules

- Read-only. No file is edited by an audit — proposals only.
- Separate the ladders: `lint`/`check` gate, `audit` proposes, `improve` applies,
  `test`/`evaluate` judge by running. Don't collapse them.
- Every finding and proposal cites a `file:line`. No verdict without evidence.
- Intent is not a defect: `preview: true`, `advertised: false`, and
  in-process-accessor services are deliberate.
- A whole sidecar is `audit-sidecar`, not a loop over `audit-artifact` per file —
  the sidecar audit adds the usage-road + doc reconciliation a per-artifact pass
  cannot see.
