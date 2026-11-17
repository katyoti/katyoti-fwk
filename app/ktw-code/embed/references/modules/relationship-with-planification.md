# Relationship with planification (requests, to-dos, milestones)

This document defines how the **module-rock-solid method** layers above the work-management primitives. Read this first — it sets the conceptual frame that everything else builds on.

> **Transport note**: the layer below this method is the **planification** module: work is a **request** filed in the module it concerns (`ktw-agents requests file`, skill `request-file`), planned into **to-dos**, and taken by a person or a session (`module-next`, `work-todo`). Delivery stages are **`milestone`** objects.

## TL;DR

Three layers. Each layer has one job. Higher layers compose lower layers; lower layers don't know the higher layers exist.

| Layer | Skill / artifact | One-line job |
|---|---|---|
| **L1** | `module-next` / `work-todo` | Take a module's next to-do and do it (the worker contract: `todos pull`, `renew`, `report`) |
| **L2** | `request-file` (`ktw-agents requests …`) | File work as requests in their module, planned into to-dos; split, order, depend |
| **L3** | `module-rock-solid` *(this method)* | Own a module's quality arc — specify, audit, plan waves of requests, promote tiers |

L3 *uses* L2 to file requests (with all info pre-filled, no user prompts) and *defers execution* to L1. L3 never executes work directly. L3 is durable; L1/L2 state is per-request.

## Why a third layer

The two lower layers are excellent at "what's the next thing to do?" but they have no concept of:

- A **module** as a coherent unit of code with its own quality bar
- A **specification** with stable UC IDs that tests reference
- A **DoR/DoD** gating model that says "we cannot start UC-12 until X is true; we cannot promote until 80% of UCs pass"
- An **audit ledger** that records repeated reviews of the same module over time
- A **tier system** for "rock solid enough to ship Tier 1; can grow to Tier 2 later"
- The **loop-back path**: when an audit finds new criticals, mechanically re-plan the milestone instead of relying on memory

L3 owns these. L3 holds the durable charter; L1/L2 stay lightweight, transactional, per-request.

## The flow

A typical pass through the method, end-to-end:

```
user: "make the tasks module rock-solid"
  │
  ▼
L3: module-rock-solid status tasks
  → reads /modules/tasks/charter.md, prints the status board
  │
  ▼
L3: module-rock-solid plan-tier 1 tasks
  → identifies tier-1 DoD gaps from the charter
  → for each gap: title, kind, details, deliverables (checks), the UC it is on
  → L2: ktw-agents requests file --module tasks … (N times, non-interactive)
  → release tasks-rs-tier-1 (a project release) that the N requests target
  │
  ▼
L1: module-next tasks  (the planner, or a session by hand)
  → takes one to-do at a time until the milestone's requests are done
  │
  ▼
L3: module-rock-solid audit tasks
  → parallel review (e.g. ktw-code:code-review), audit bundle + findings
  → updates the charter's audit ledger
  │
  ├── new critical/high findings → plan-tier 1 again (only the new gaps)
  └── none → promote tasks tier 1 (charter status; next: tier 2 or another module)
```

## What L3 files with L2 (non-interactive contract)

To file a request **without prompting the user**, L3 passes everything `ktw-agents requests file` takes:

| Flag | Source |
|---|---|
| `--module` | the module being made rock-solid |
| `--kind` | from the gap's classification (security-fix → `fix`, missing behaviour → `build`, decision → `question`, audit-only → `review`) |
| `--priority` | critical → `P0`, high → `P1`, the rest `P2` |
| `--by` | `{"id":"<module>","kind":"module"}` (L3 asks on the module's behalf) |
| `--title` | the outcome, one sentence |
| `--details` | the gap description with file:line references from the audit, the affected UCs, related rules and decisions |
| `--deliverables` | the checks lifted from the charter's DoD-per-UC for the affected UCs (`[{"title":…,"check":"<a command or an action URI>"}]`) |
| `--on` | the use case (or feature) the request carries |
| `--depends_on` | requests it must follow (an open decision's question request, a foundation fix) |

The tier's release is a project release in `sys-planification` (version `<module>-rs-tier-<n>`) that the requests target; its grade is the rollup over its members (`judged-by` when given).

## What L3 does NOT do

- **L3 does not run code, edit source files, or commit.** Those are downstream effects of L1 working the to-dos L2 planned.
- **L3 does not re-implement work management.** `ktw-agents requests file` is the only path to new work; never a hand-written task file.
- **L3 does not own the release format.** It plans a release like any other, in the `module-rock-solid` naming.
- **L3 does not replace `module-next`.** The planner, the user or a session still takes the to-dos.

## Naming convention for L3 milestones

| Pattern | Example | Meaning |
|---|---|---|
| `ms-<module>-rs-tier-<n>` | `ms-tasks-rs-tier-1` | Every request needed for module `<module>` to clear tier `<n>` |

This pattern keeps L3 milestones distinguishable from domain milestones and groups them by module + tier.

## Open contract questions

Listed for the eventual skill spec, not blocking now:

1. **Requests already in flight.** If L3 re-runs `plan-tier` while requests are open, does it diff against them (skip duplicates) or refuse? Default proposal: diff (`requests list --module <m>`) + skip + report.
2. **Audit-driven re-planning vs user-initiated.** Should the audit automatically trigger `plan-tier` for new findings, or always require explicit user invocation? Default: explicit only — keeps the user in control.
3. **Cross-module dependencies.** `tasks` charter may reference UCs from `agent`. Default: L3 files the request in the module that answers (a module asking another, `request-file`) and depends on it.

These are decisions to open in the module (`ktw-methods:decisions/Open`) when the skill is built.

## The contract the skill depends on

| Operation | Through |
|---|---|
| File a request with a full payload | `ktw-agents requests file …` |
| List a module's open requests | `ktw-agents requests list --module <m>` |
| Order and depend | `ktw-agents requests depend --id <waiting> --on <id>` |
| Check the tier's release exists / plan it | `ktw-agents releases list --module sys-planification` / `releases plan --module sys-planification --version <module>-rs-tier-<n>` |
| Put a request in it | `ktw-agents releases target --id <request> --release <release>` |
| Grade it | `ktw-agents releases grade --id <release>` |
| Where a request stands | `ktw-agents requests get --id <id>` (its plan and to-dos) |
