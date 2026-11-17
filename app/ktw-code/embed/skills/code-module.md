---
name: code-module
description: >-
  Bring a code module to production-grade quality through a reproducible
  per-module workflow: bootstrap → specify → plan-tier → execute → verify →
  audit → promote → retrospective. Use when bootstrapping a charter for a
  new module, advancing an existing module toward a tier promotion, running
  audits or UC verification, or producing the cross-module factory rollup.
  Wraps the existing `ktw-code:code-review` skill for the audit step.
iconName: file-shield
icon: "📋"
hashtags: [module, charter, quality, production, factory, rock-solid, tier]
category: lifecycle
provides: [task]
---

# Code Module — Rock-Solid Method

Drive a module from "no charter" to "production-grade tier N" through a reproducible workflow. The method sits **one layer above** the work-management primitives (requests filed in the module with `ktw-agents requests file`, planned into to-dos) — it owns the per-module quality arc, plans waves of requests, runs audits, gates tier promotions.

This skill never executes work directly. It plans, audits, and reports. Execution is delegated to `module-next` (or the planner).

> **Progressive discovery**: this skill is intentionally lean. Per-verb procedures, the charter template, the workflow loop, the test-authoring guide, and all rules live in references loaded on demand. See "References" below.

## Verbs

| Verb | One-line | Reference |
|---|---|---|
| `charter` | Bootstrap or refresh a module's charter | [`verbs/charter.md`](#) |
| `status` | Print where one module stands (read-only) | [`verbs/status.md`](#) |
| `lint` | Run rules + alignment checks (read-only by default) | [`verbs/lint.md`](#) |
| `plan-tier` | Compute tier-N gaps; file one request per gap (`ktw-agents requests file`) | [`verbs/plan-tier.md`](#) |
| `verify` | Run UC test layers (L1+L2+L3); produce green/red board | [`verbs/verify.md`](#) |
| `audit` | Run a fresh 13-agent code review; archive bundle | [`verbs/audit.md`](#) |
| `promote` | Move module up a tier (gated; updates charter) | [`verbs/promote.md`](#) |
| `factory-status` | Portfolio rollup across all bootstrapped modules (read-only) | [`verbs/factory-status.md`](#) |

Load any verb's full detail via:

Read the `ktw-code:/reference/modules/verbs/<verb>.md` reference.

## High-level workflow

```
┌──────────────┐   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│  Bootstrap   │ → │   Specify    │ → │  Plan-tier   │ → │   Execute    │
│  (charter)   │   │  (DoD/UCs)   │   │  (requests)  │   │ module-next  │
└──────────────┘   └──────────────┘   └──────┬───────┘   └──────┬───────┘
                                             ▲                  │
                                             │                  ▼
                          ┌──────────────────┴──────┐   ┌──────────────┐
                          │  audit-loop / verify-   │ ← │   Verify     │
                          │  fail-loop (re-plan)    │   │   (testing)  │
                          └──────────────┬──────────┘   └──────┬───────┘
                                         │                     ▼
                                         │              ┌──────────────┐
                                         │              │    Audit     │
                                         │              └──────┬───────┘
                                         │                     ▼ clean
                                         │              ┌──────────────┐
                                         │              │   Promote    │
                                         │              └──────┬───────┘
                                         │                     ▼
                                         │              ┌──────────────┐
                                         │              │ Retrospective│
                                         │              └──────────────┘
                                         │                     │
                                         └─────────────────────┘
                                            (next tier or
                                             next module)
```

For the full lifecycle including loop-backs and cadence rules, see [`workflow.md`](#) (`ktw-code:/reference/modules/workflow.md`).

## Rules (always applicable)

- **Method is on-request, not on-commit**. Heavy gates (verify, audit, lint with race detector) run via local invocation or manual CI trigger — never on push.
- **Charter is the single source of truth**. Tasks reference UC IDs from the charter; tasks never define new UCs.
- **Stable UC IDs from day 1, with gaps between groups** so future expansion never requires renumbering.
- **`(gap)` marker for missing tests** in DoD-per-UC bodies. Each `(gap)` becomes a tier-N task.
- **Negative DoD is required for `security` UCs**. Positive-only DoDs miss bypass paths.
- **L2 scenarios default to `type: simulator`** unless the UC intrinsically requires a real LLM.
- **Resolved OD → exactly one of**: tier-N task, `fail-deferred` UC, charter update. Never silently dropped.
- **Audit bundles are immutable**. Re-audits land in dated sibling folders.
- **Manifest is the writer's responsibility**. Every state-changing verb writes `manifest.json`.
- **Promote must be deliberate**. The verb refuses unmet gates; `--force` is auditable.
- **A charter that lies is worse than no charter**. If `lint` reports drift, reconcile before promote.
- **Test-creation tasks must follow the universal authoring procedure** (per Lesson 2 — see `lessons-from-tasks-bootstrap.md`). Cite [`e2e-test-authoring.md`](#); copy `_template.tasktest`; reference fixtures from `fixtures/`; include negative DoD for security.
- **Plan-tier has hard preconditions**. Refuses if charter < v0.1, e2e authoring guide missing, or L2 template missing. `--force` available with audit trail.

## When to use which verb

| If you want to... | Use | Reference |
|---|---|---|
| Start a new module's quality program | `charter <module>` | [verbs/charter.md](#) |
| Check current state of a module | `status <module>` | [verbs/status.md](#) |
| Find drift / rule violations | `lint <module>` | [verbs/lint.md](#) |
| Generate the next wave of tasks | `plan-tier <n> <module>` | [verbs/plan-tier.md](#) |
| See pass/fail per UC right now | `verify <module>` | [verbs/verify.md](#) |
| Get a fresh independent assessment | `audit <module>` | [verbs/audit.md](#) |
| Move tier 0 → 1 (or 1 → 2, etc.) | `promote <module>` | [verbs/promote.md](#) |
| See the whole portfolio | `factory-status` | [verbs/factory-status.md](#) |

## References (load on demand)

### Method-level (universal)

| Topic | Reference |
|---|---|
| Per-module charter format | `ktw-code/modules/charter-template.md` |
| Lifecycle loop (Bootstrap → Specify → … → Retrospective) | `ktw-code/modules/workflow.md` |
| L1/L2/L3 testing strategy (what + why per layer) | `ktw-code/modules/test-layers.md` |
| **HOW** to write a test from a DoD (`.tasktest` schema, DSL, worked example) | `ktw-code/modules/e2e-test-authoring.md` |
| Bundle inventory + versioning per module | `ktw-code/modules/deliverables.md` |
| Portfolio view design | `ktw-code/modules/factory-rollup.md` |
| Rule format + lint integration | `ktw-code/modules/standards-enforcement.md` |
| Charter-to-code drift detection | `ktw-code/modules/alignment.md` |
| Go tooling stack (golangci-lint, race tests, on-request philosophy) | `ktw-code/modules/quality-tooling.md` |
| Inventory of candidate modules in this codebase | `ktw-code/modules/module-catalog.md` |
| Layering over planification (requests, to-dos, milestones) | `ktw-code/modules/relationship-with-planification.md` |
| **Lessons from real runs** (heuristics, gaps, generalizations) | `ktw-code/modules/lessons-from-tasks-bootstrap.md` |

### Templates

| Template | Reference |
|---|---|
| L2 scenario starter | `ktw-code/modules/templates/template.tasktest` |

### Per-verb (load when invoking the verb)

| Verb | Reference |
|---|---|
| All verbs index | `ktw-code/modules/verbs/README.md` |
| `charter` | `ktw-code/modules/verbs/charter.md` |
| `status` | `ktw-code/modules/verbs/status.md` |
| `lint` | `ktw-code/modules/verbs/lint.md` |
| `plan-tier` | `ktw-code/modules/verbs/plan-tier.md` |
| `verify` | `ktw-code/modules/verbs/verify.md` |
| `audit` | `ktw-code/modules/verbs/audit.md` |
| `promote` | `ktw-code/modules/verbs/promote.md` |
| `factory-status` | `ktw-code/modules/verbs/factory-status.md` |

## Status

**Phase A.5** — first runtime version of the skill. Authored after the `tasks` module bootstrap completed by hand on 2026-05-06. The skill is a procedural runbook an LLM can follow today; verb implementations are not yet wired into Go code. As verbs gain Go implementations, this file becomes the spec they realize.
