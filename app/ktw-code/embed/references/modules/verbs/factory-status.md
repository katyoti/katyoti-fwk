# Verb — `code-module factory-status`

Read-only portfolio rollup across all bootstrapped modules. The cross-module view that complements per-module `status`.

## Inputs

- (no `<module>` arg) — operates on the whole `/modules/` tree
- `--format text|md|json` (default: `text`)
- `--tier <n>` — filter to modules at tier N
- `--blocker <OD-ID>` — filter to modules blocked on a specific open decision
- `--stale <days>` — filter to modules whose last audit is older than N days (90 = staleness gate)
- `--bare` — drop empty columns (compact terminal view)

## When to use

- **Daily** by a factory owner who wants a morning snapshot
- **Weekly** for stakeholder status emails
- **Pre-promotion** to confirm no other module is blocked on what's about to ship
- **Before any plan-tier** to catch cross-module dependencies
- **In CI** as `code-module factory-status --format json --stale 90` for staleness gating

## Behavior

```
for each subfolder under /modules/ (excluding docs/, skills/):
  if /modules/<name>/manifest.json exists:
    parse manifest
    optionally read charter Status board
    optionally read latest results.json
  else:
    record as "not bootstrapped"

aggregate into rows
sort:
  1. tier ascending (tier 0 / unbootstrapped first — needs attention)
  2. last audit date ascending (stalest first)
filter by flags
print
```

## Output example (text)

```
Factory rollup — 2026-05-06 12:30Z

Module          Tier    Charter   Last audit          UCs (pass/total)   Active milestone           Blockers / notes
─────────────── ─────── ───────── ─────────────────── ────────────────── ────────────────────────── ─────────────────────────────────
tasks           0 → 1   v0.1      2026-05-06 (3C 6H)  8 / 54             ms-tasks-rs-tier-1         all 6 ODs resolved; 30 tasks
agent           —       —         —                   —                  —                          not bootstrapped (Phase C)
orchestration   —       —         —                   —                  —                          not bootstrapped (Phase C)

Summary
  Modules total:           ~30 (per /ktw-code/modules/module-catalog.md)
  Modules bootstrapped:    1
  Modules at tier 1:       0
  Modules with criticals:  1
  Modules with stale audit (>90d): 0
```

## Output example (JSON)

For dashboards / external tooling. Schema:

```json
{
  "generated_at": "2026-05-06T12:30:00Z",
  "modules": {
    "tasks": {
      "tier": 0,
      "tier_target": 1,
      "charter_version": "0.1",
      "last_audit": "2026-05-06T07:56:00Z",
      "audit_findings": {"critical": 3, "high": 6, "medium": 8},
      "ucs": {"total": 54, "with_dod": 51, "passing": 8, "fail_deferred": 2},
      "active_milestone": "ms-tasks-rs-tier-1",
      "open_decisions": [],
      "manifest": "/modules/tasks/manifest.json"
    }
  },
  "summary": {
    "modules_total": 1,
    "bootstrapped": 1,
    "at_tier_1_or_above": 0,
    "with_open_criticals": 1,
    "stale_audit_count": 0
  }
}
```

## Composition

| Pattern | What it does |
|---|---|
| `... --format json \| jq '.modules \| to_entries[] \| select(.value.tier < 1) \| .key'` | Names of modules not at tier 1 |
| `... --format json --stale 90 \| jq '.summary.stale_audit_count'` | One number for a CI gate |
| `for m in $(... --format json --tier 0 \| jq -r 'keys[]'); do code-module status $m; done` | Drill-down |

## Aggregation gotchas

- **Stale `manifest.json`**: rollup is only as current as each module's manifest. The skill writes manifest on every state-changing verb invocation; manual edits are discouraged.
- **Inconsistent fields**: schema is fixed (per [`deliverables.md`](../deliverables.md)); `charter` verb scaffolds the full schema.
- **Modules outside `/modules/`**: don't count. Sidecar binaries that haven't been bootstrapped show as "not bootstrapped" (the catalog has the inventory; the rollup shows the active subset).

## Future: dashboard

The JSON output is dashboard-ready. A simple HTML page that fetches `/modules/factory-status.json` on a schedule renders a portfolio matrix. SonarCloud / Grafana / internal tools can ingest the same JSON.

## See also

- [`factory-rollup.md`](../factory-rollup.md) — the design behind this verb
- [`module-catalog.md`](../module-catalog.md) — the inventory of all candidate modules in this codebase
- [`verbs/status.md`](status.md) — single-module view
