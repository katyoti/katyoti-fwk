# Verb — `code-module status <module>`

Read-only. Print where one module stands.

## Inputs

- `<module>` — module name
- `--format text|json|md` (default: text)

## When to use

- Daily check on a module's tier-1 push progress
- Pre-meeting status snapshot
- Quick confirmation that recent work moved a metric (e.g. UC count, audit findings)

## Behavior

```
read /modules/<module>/manifest.json
read /modules/<module>/charter.md (status board, audit ledger, open decisions)
read the active milestone's requests (its includes edges; ktw-agents requests get) — count requests by plan state
read /modules/<module>/results/latest/results.json (if exists)
print:
  - module name + tier + charter version
  - status board (UCs / DoD / audit findings / tests)
  - active milestone name and request counts
  - blockers (open decisions assigned to current tier; failing UCs)
  - "next step" suggestion
```

## Output example (text)

```
Module: tasks                        Tier: 0 → 1 (in progress)
Charter: v0.1                         Last audit: 2026-05-06 (3 critical, 6 high)

UCs:
  Total: 54
  With DoD-per-UC: 51 / 54
  Passing: 8 / 54

Tests:
  L1: 12 / 54 covered
  L2: 0 / 54 covered            ← needs ms-tasks-rs-tier-1.3
  L3: 0 / 54 covered

Audit (2026-05-06):
  Critical: 3                   ✗ tier-1 requires 0
  High: 6                       ✗
  Medium: 8

Active milestone: ms-tasks-rs-tier-1
  Sub-milestones: 1.1 (3 requests), 1.2 (9 requests), 1.3 (18 requests)
  Total: 30  (running: 0, planned: 30, done: 0)

Open decisions blocking tier-1: none (all 6 resolved)
Deferred-red UCs: 2 (UC-073 / OD-004, UC-094 / OD-005)

Next: module-next tasks takes the first to-do of ms-tasks-rs-tier-1.1
       (recommend: tier-1-nfr-wiring — smallest scope)
```

## Output example (JSON)

Suitable for dashboards / CI gates:

```json
{
  "module": "tasks",
  "tier": 0,
  "tier_target": 1,
  "charter_version": "0.1",
  "last_audit": "2026-05-06_07-56",
  "audit_findings_open": {"critical": 3, "high": 6, "medium": 8},
  "uc_total": 54,
  "uc_with_dod": 51,
  "uc_passing": 8,
  "uc_fail_deferred": 2,
  "active_milestone": "ms-tasks-rs-tier-1",
  "open_decisions": [],
  "tier_1_blockers": ["3 critical findings open", "test gaps"]
}
```

## Composition

- `code-module status <m> --format json | jq '.uc_passing'` — single number for CI
- `for m in tasks agent; do code-module status $m; done` — drill into multiple modules
- See also `code-module factory-status` for the full portfolio view

## See also

- [`verbs/factory-status.md`](factory-status.md) — portfolio-wide rollup
- [`charter-template.md`](../charter-template.md) — what the status board displays
- [`workflow.md`](../workflow.md) — where status fits in the lifecycle
