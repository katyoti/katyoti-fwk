# Verb — `code-module promote <module>`

Move module up a tier. Gated; updates charter; triggers retrospective.

## Inputs

- `<module>` — module name
- `--to <tier>` (default: current+1)
- `--force` (**discouraged**) — promote despite failing gates; recorded in `manifest.json.forced_promotions[]`

## When to use

- All tier-N gates met (per charter); ready for promotion
- Audit clean, lint clean, race detector green, ≥80% UCs passing or fail-deferred

## Gates (must pass — `--force` overrides)

- All `core` and `security` UCs pass OR are `fail-deferred` with cited resolved-OD
- ≥80% (tier 1) / 100% (tier 2+) of non-deferred UCs pass
- Latest audit zero unmitigated critical/high findings
- `code-module lint` zero errors
- Race detector clean on full L1 suite
- L2 covers full dimension matrix per charter
- Public API contract documented and respected for past 30 days (apidiff clean)

If any gate fails AND not `--force`: refuse with a list of which gates failed and how to close them.

## Behavior

```
read charter, manifest, latest results, latest audit, latest lint report
target_tier = --to or current+1

check tier-target DoD gates (above)
if any gate fails AND not --force:
  print which gates failed and how to close them
  exit nonzero, no charter changes
else:
  snapshot current charter → charter-archive/v<old_version>.md
  update charter:
    Meta.Charter version: bump
    Meta.Current tier: target_tier
    Status board: targets shifted to next tier
    Audit ledger: row for the audit that justified promotion
  update manifest.json
  release the tier's release <module>-rs-tier-<old> (its grade green: every request targeting it done)
  print "Promoted to tier <target_tier>. Charter version <new>."
  prompt: "Run retrospective now? [Y/n]"  ← per workflow.md Retrospective phase
  if yes: scaffold /modules/docs/lessons/<date>-<module>-tier-<n>.md
          with the four-question template
```

## Output

- Updated `/modules/<module>/charter.md` (tier bump, version bump)
- New `/modules/<module>/charter-archive/v<old>.md` snapshot
- The tier's release graded green (`ktw-agents releases grade`) and released
- Updated `/modules/<module>/manifest.json`
- Optional new lessons file under `/modules/docs/lessons/`

## Retrospective trigger

After successful promote, the verb prompts the user to scaffold a retrospective lesson. **Default: yes**. If the user defers, a follow-up reminder appears in `code-module status <module>` until the lesson file exists.

The lesson template uses the four-question format from [`workflow.md`](../workflow.md):

- What worked well?
- What didn't?
- What surprised us?
- What's still missing?

Plus actionable method updates per [`lessons-from-tasks-bootstrap.md`](../lessons-from-tasks-bootstrap.md)'s "Pattern for filing future lessons."

## `--force` is auditable

Forced promotions get an entry in `manifest.json.forced_promotions[]` with:

- `tier`: target tier
- `at`: timestamp
- `reason`: why force was used (required; refuses without)
- `gates_unmet`: which gates were bypassed

This lets future audits and retrospectives identify modules that promoted prematurely.

## Demotion

A future audit that finds new criticals on a tier-1 module **does NOT auto-demote**. Instead:

1. Promote-was-wrong scenario: spawn a tier-1 follow-up milestone (`ms-<module>-rs-tier-1-followup`).
2. Charter audit ledger records the regression.
3. Status board reflects the gap until tier-1 is re-validated.

Explicit demote (`code-module promote --to <lower-tier>`) is allowed but rare and discouraged. Most "demotions" are de-facto via the audit-loop adding tasks to a tier-1-followup milestone.

## See also

- [`workflow.md`](../workflow.md) — Promote and Retrospective phases
- [`charter-template.md`](../charter-template.md) — DoD per module / tier
- [`lessons-from-tasks-bootstrap.md`](../lessons-from-tasks-bootstrap.md) — retrospective lesson format
- [`verbs/audit.md`](audit.md), [`verbs/verify.md`](verify.md), [`verbs/lint.md`](lint.md) — pre-promote gates
