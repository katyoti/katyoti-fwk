# Factory Rollup — Portfolio View Across Modules

The single per-module charter answers "is this module rock-solid?" The factory rollup answers **"how is the whole portfolio doing?"** — across all modules at once.

This is the missing aggregation layer. It reads each module's `manifest.json`, joins with the latest audit/results, and presents a single status board for the whole `/modules/` tree.

## Verb: `code-module factory-status`

Read-only aggregator. No flags = produce the default factory view.

### Inputs

- (no `<module>` arg) — verb operates on the whole `/modules/` tree
- `--format text|md|json` (default text) — output shape
- `--tier <n>` — filter to modules at a specific tier
- `--blocker <OD-ID>` — filter to modules blocked on a specific open decision
- `--stale <days>` — filter to modules whose last audit is older than N days
- `--bare` — drop columns with no signal (compact view for terminals)

### Behavior

```
for each subfolder under /modules/ (excluding docs/, skills/):
  if /modules/<name>/manifest.json exists:
    parse manifest
    optionally read charter Status board
    optionally read latest results.json
  else:
    record as "not bootstrapped"

aggregate into rows
sort by:
  1. tier ascending (tier 0 / unbootstrapped first — needs attention)
  2. last audit date ascending (stalest first)
filter by flags
print
```

### Output example (text format, default)

```
Factory rollup — 2026-05-06 12:30Z

Module          Tier    Charter   Last audit          UCs (pass/total)   Active milestone           Blockers / notes
─────────────── ─────── ───────── ─────────────────── ────────────────── ────────────────────────── ─────────────────────────────────
tasks           0 → 1   v0.1      2026-05-06 (3C 6H)  8 / 47             ms-tasks-rs-tier-1         OD-001 OD-002; 14 tier-1 tasks
agent           —       —         —                   —                  —                          not bootstrapped (Phase C)
orchestration   —       —         —                   —                  —                          not bootstrapped (Phase C)
tools (group)   —       —         —                   —                  —                          not bootstrapped (Phase D)
skills (group)  —       —         —                   —                  —                          not bootstrapped (Phase D)
kts             —       —         —                   —                  —                          not bootstrapped (later)

Summary
  Modules total:           ~30 (per ktw-code/modules/module-catalog.md)
  Modules bootstrapped:    1
  Modules at tier 1:       0
  Modules with criticals:  1
  Modules with stale audit (>90d): 0
  Open decisions across portfolio: 2 (OD-001, OD-002)
```

### Output example (JSON format)

For dashboards / external tooling:

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
      "ucs": {"total": 47, "with_dod": 12, "passing": 8},
      "active_milestone": "ms-tasks-rs-tier-1",
      "open_decisions": ["OD-001", "OD-002"],
      "manifest": "/modules/tasks/manifest.json"
    },
    "agent": {"status": "not_bootstrapped", "phase": "C"},
    "orchestration": {"status": "not_bootstrapped", "phase": "C"}
  },
  "summary": {
    "modules_total": 3,
    "bootstrapped": 1,
    "at_tier_1_or_above": 0,
    "with_open_criticals": 1,
    "stale_audit_count": 0
  }
}
```

The JSON view is what powers any external dashboard (Grafana panel, internal status page, Slack daily-summary bot, etc.).

### Output example (markdown format)

For pasting into PR descriptions, weekly status emails, etc. Same data as the text view but rendered as a GitHub-flavored table.

## When to invoke

- **Daily** by a factory owner who wants the morning status snapshot
- **Weekly** by stakeholders who want a status email
- **Pre-promotion** to confirm no other module is blocked on what's about to ship
- **Before any plan-tier** to see if there are cross-module dependencies to resolve first
- **In CI** as `code-module factory-status --format json --stale 90` to fail the build if any module's audit is too old

## Filters in detail

### `--tier <n>`

Show only modules currently at tier `<n>`. `--tier 0` → modules that haven't promoted yet. `--tier 1` → modules at tier 1 (the "rock solid" floor). `--tier none` → only bootstrapped-but-no-tier-yet modules (rare; should not exist long).

### `--blocker <OD-ID>`

Show only modules whose `open_decisions[]` includes the given OD ID. Useful when an OD is about to be resolved and the factory owner wants to know which modules unblock as a result.

### `--stale <days>`

Show only modules whose `last_audit` is older than N days. The default factory cadence (per [`workflow.md`](workflow.md)) recommends a re-audit at least every 90 days; `--stale 90` is the typical staleness check.

### `--bare`

Drop the columns whose entire visible set is empty/dash. Useful for terminals where the wide table wraps badly.

## Composition with other verbs

| Pattern | What it does |
|---|---|
| `code-module factory-status \| jq '.modules \| to_entries[] \| select(.value.tier < 1) \| .key'` | Module names of everything not yet at tier 1 |
| `code-module factory-status --format json --stale 90 \| jq '.summary.stale_audit_count'` | One number for a CI gate |
| `for m in $(code-module factory-status --format json --tier 0 \| jq -r 'keys[]'); do code-module status $m; done` | Drill-down into every tier-0 module |

## Aggregation gotchas (and how this design avoids them)

- **Stale `manifest.json`**: if a module's manifest isn't updated by every workflow step, the rollup lies. Mitigation: `manifest.json` is updated by *every* skill verb invocation that affects state (per the verb references in [`verbs/`](verbs/README.md)). Manual edits to `manifest.json` are discouraged; the skill is the writer.
- **Inconsistent fields across modules**: if module A's manifest reports `ucs.total` and module B doesn't, the aggregation breaks. Mitigation: the manifest schema is fixed (defined in [`deliverables.md`](deliverables.md)); the skill's `charter` verb scaffolds the full schema on bootstrap.
- **Modules outside `/modules/`**: bundles only count if they live under `/modules/<name>/`. Sidecar binaries that haven't been bootstrapped show as "not bootstrapped" (the catalog has the inventory; the rollup shows the active subset).
- **Race between two skill invocations**: two `code-module verify` runs writing `results/latest/` simultaneously could produce inconsistent state. Mitigation: rare in practice (single-user) but documented as a known limitation; advisory file lock recommended in the skill's implementation.

## Future: dashboard

`factory-rollup.md` describes the data layer. A dashboard layer is out of scope for the spec but well-positioned:

- The JSON output is dashboard-ready.
- A simple HTML page that fetches `/modules/factory-status.json` on a schedule renders a portfolio matrix.
- A SonarCloud / Grafana / internal tool can ingest the same JSON.

Recommendation: ship the verb first, defer the dashboard until the rollup proves valuable in raw form.

## Relationship to the module catalog

[`module-catalog.md`](module-catalog.md) is the **inventory of candidate modules** (what could be charterized). `factory-rollup.md` (this doc) is the **portfolio of actually-charterized modules** (what is being managed).

The catalog is updated by humans (or future tooling) when a new extension is added to the codebase. The rollup is updated automatically on every skill invocation. They never disagree on the bootstrapped subset, but the catalog usually lists more entries than the rollup.
