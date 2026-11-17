# Verb — `code-module lint <module>`

Run rules + alignment checks against a module. Read-only by default.

Two categories of check:

1. **Rules** — cross-module conventions enforced via [`standards-enforcement.md`](../standards-enforcement.md)
2. **Alignment** — charter-to-code drift detection per [`alignment.md`](../alignment.md)

## Inputs

- `<module>` (optional) — lint a specific module. Without it, lint cross-module rules portfolio-wide.
- `--rules <list>` — subset of rule IDs to run (default: all applicable)
- `--severity error|warning|info|all` (default: `warning`)
- `--format text|md|json`
- `--non-interactive` — skip rules with `human-assert` checks (CI mode)
- `--write` — write lint report to `results/latest/lint.json`

## When to use

- During Specify (alignment checks help find inventory gaps)
- Before `audit` (lint output included in audit bundle)
- Before `promote` (mandatory; lint must be clean)
- Ad-hoc during refactor (catch drift early)

## Behavior

```
read /modules/<module>/manifest.json (if module specified)
read /modules/<module>/charter.md
load applicable rules from /ktw-code/modules/rules/
load alignment checks from charter (Concepts, Function inventory, UCs, Test plan)
filter by --rules / --severity / module-kind / paths

for each applicable rule, sorted by severity desc:
  for each check in rule.checks:
    run the check (file-exists, grep-must-not-match, go-symbol-exists, etc.)
    record result (pass / fail / skipped)
  if rule has human-assert AND --non-interactive: skip with notice
  else if human-assert: prompt user

aggregate results
print: errors (blocking) / warnings (visible) / infos (advisory) / skipped
return non-zero if any errors and --non-interactive, else zero
```

## Check kinds (from standards-enforcement.md)

| Kind | Purpose |
|---|---|
| `file-exists` / `file-not-exists` | A path must / must not exist |
| `file-list-exact` | A directory contains exactly the listed files |
| `grep-must-not-match` / `grep-must-match` | Regex over file globs |
| `go-symbol-exists` / `go-symbol-not-exists` | A Go symbol must (not) be exported |
| `go-test-exists` | Go test by name resolves |
| `import-must-not-include` | Forbid an import edge |
| `manifest-field` | Manifest JSON field meets predicate |
| `human-assert` | Checklist item; not mechanizable |

## Initial rule catalog (seeded from CLAUDE.md)

| Rule ID | Severity | Tier |
|---|---|---|
| `rule-sidecar-cmd-layout` | error | T2 |
| `rule-module-public-private-split` | error | T2 |
| `rule-publish-service-pattern` | error | T2 |
| `rule-naming-typename-configname` | warning | T2 |
| `rule-topic-constructors-only` | error | T2 (grep) |
| `rule-no-yaml-unmarshal` | error | T2 (grep) |
| `rule-no-services-em-direct` | error | T2 |
| `rule-xdg-directory-usage` | warning | T2 |
| `rule-godoc-on-exported` | info | T2 |
| `rule-charter-meta-complete` | error | T2 |
| `rule-charter-uc-ids-stable` | error | T2 |
| `rule-test-references-real` | error | T2 (alignment) |
| `rule-permission-enforcement-opt-out` | error | T1 (post-audit) |
| `rule-tasktest-uc-frontmatter` | error | T2 (per Lesson 2) |

## Output example (text)

```
Linting module: tasks (path: internal/extensions/task/, kind: extension)
Rules applied: 14 / 38 in catalog

ERRORS (must-fix before promote)
  ✗ rule-topic-constructors-only (severity: error)
    Found inline topic strings:
      api/engine/spawner.go:127:  "child."+ name + ".started"
      api/engine/liveness.go:48:  HeartbeatTopicPrefix + runID

WARNINGS
  ⚠ rule-charter-uc-ids-stable
    UC-150 referenced in test catalog but not in charter inventory

INFOS
  ℹ rule-godoc-coverage
    18 / 47 exported symbols lack a doc comment

Result: 1 error, 1 warning, 1 info. Promote blocked.
```

## Integration with workflow

| Phase | When `lint` runs | Failure action |
|---|---|---|
| Bootstrap | After charter scaffold | Errors block bootstrap; warnings logged |
| Specify | Manual; surfaces inventory gaps | Advisory only |
| Plan-tier | Auto-runs at start; lint errors → tasks | Errors → mandatory tier-N tasks |
| Verify | Before tests run; fast feedback | Reported alongside test results |
| Audit | Lint output included in audit bundle | No direct gating |
| Promote | **Required clean** | Errors block; `--force` available |

## Authoring a new rule

See [`standards-enforcement.md`](../standards-enforcement.md) for the rule frontmatter format and step-by-step authoring procedure.

## See also

- [`standards-enforcement.md`](../standards-enforcement.md) — rule format and full check-kind reference
- [`alignment.md`](../alignment.md) — charter-to-code drift checks
- [`quality-tooling.md`](../quality-tooling.md) — `golangci-lint` integration as one rule kind
