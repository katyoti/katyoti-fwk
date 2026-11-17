# Verb — `code-module audit <module>`

Run a fresh independent assessment via 13 parallel review agents. Wraps the existing `ktw-code:code-review` skill.

**Blocks on user confirmation by default** (cost-aware — ~13 parallel agents, ~2M+ tokens for a typical module). `--yes` skips for CI.

## Inputs

- `<module>` — module name
- `--reviews <list>` (optional) — subset of audit kinds (default: all 13)
- `--reuse-baseline` — reuse previous audit folder structure for cheap rerun
- `--yes` — skip confirmation (CI / scripted invocations)

## When to use

- After a milestone completes; before `promote`
- Quarterly cadence per [`workflow.md`](../workflow.md) (90-day staleness gate)
- After a major refactor; confirm no regressions in earlier-clean dimensions
- Independently of milestone progress when the team wants a fresh look

## Behavior

```
read manifest.json for source path, charter version, git SHA
read review subset (default = all 13 from ktw-code:code-review)
estimate cost: print "About to run N parallel audit agents against <source>.
                Estimated tokens: ~X. Continue? [y/N]"
if user does not confirm AND not --yes: exit; no charter changes

date = now (YYYY-MM-DD_HH-MM)
audit_dir = /modules/<module>/audits/<date>/

invoke ktw-code:code-review against the source folder, output to audit_dir
write audit_dir/manifest.json with audit metadata
add a row to charter audit ledger
update manifest.json.last_audit

compute closed/new/persisting findings vs previous audit
print summary; suggest next:
  if new criticals/highs → code-module plan-tier <current+1> <module>
  else if DoD met        → code-module promote <module>
  else                   → code-module status <module>
```

## Output

- `/modules/<module>/audits/<date>/` — full bundle (~13 reports + summary + manifest.json)
- Updated `/modules/<module>/charter.md` audit ledger row
- Updated `/modules/<module>/manifest.json.last_audit`

The audit subfolder is **immutable**. Never overwritten or edited. Re-audits land in dated sibling folders.

## The 13 review kinds (default subset)

1. architecture
2. correctness
3. security
4. quality
5. complexity
6. library-audit
7. performance
8. test-coverage
9. cli-ux
10. data-model
11. package-layout
12. threat-model
13. red-team-architecture

A 14th, `dead-code-audit`, may be added as a follow-up after the main run if dead-code findings are flagged.

## Cost discipline

The 2026-05-06 audit on the `tasks` module consumed ~2.1M total tokens across 13 agents (~13 minutes wallclock). Typical cost factors:

- Source size (LOC × file count)
- Review subset breadth
- Agent depth (governed by review-guide complexity)

For modules <5kLOC, expect ~1.2M tokens. For >15kLOC, expect ~3M+. Use `--reviews <subset>` to scope down for spot checks.

## Comparison with previous audit

The summary printed after the run highlights:

- **Closed**: findings present last audit, not in this one
- **New**: findings present in this audit, not the last
- **Persisting**: same finding in both (oldest persisting flagged if older than tier-promotion target)

This is half the value of audit-ledger discipline. Re-running an audit without comparing dilutes the signal.

## Failure modes

- Cost confirmation declined → exit 0; no state change
- `ktw-code:code-review` itself errors mid-run → partial bundle; ledger row marked `incomplete`; user must re-run
- Source path missing → refuse with error

## See also

- `ktw-code:code-review` — the underlying skill the audit verb wraps
- [`workflow.md`](../workflow.md) — Audit phase + 90-day staleness gate
- [`deliverables.md`](../deliverables.md) — audit bundle structure (`audits/<date>/manifest.json` schema)
- [`verbs/promote.md`](promote.md) — audit cleanliness is a promote gate
