# Verb — `code-module verify <module>`

UC-driven E2E with green/red output. **Distinct from `go test`**: outer-loop UC gate, on-request, UC-keyed, simulator-backed for token-free determinism.

## Inputs

- `<module>` — module name
- `--layer L1|L2|L3|all` (default: `all`)
- `--uc <id-or-pattern>` — run only specified UCs (e.g. `UC-040`, `UC-04*`, `UC-053..UC-058`)
- `--severity core|security|core+security|all` (default: `all`)
- `--archive` — snapshot results to `results/<date>/`
- `--format text|md|json` (default: `text`)
- `--bail-on-first-fail` (CI-friendly)

## When to use

- Before opening a PR that touches the module
- Before promoting (mandatory)
- After landing a fix; verify the targeted UC turned green
- In CI via `workflow_dispatch` (manual trigger; not on every push — see [`quality-tooling.md`](../quality-tooling.md))

## Behavior

```
read /modules/<module>/charter.md (Test plan + DoD-per-UC)
filter UCs by --layer / --uc / --severity flags
for each UC:
  for each layer where the UC has a Test plan row:
    L1: go test -run ^TestName$ ./<pkg>/... -race -count=1
    L2: ktw-test harness runs the named .tasktest scenario
        (token-free: scenarios use type: simulator wherever possible)
    L3: stable+candidate orchestrates the named TASK.md
  evaluate the layer outputs against the UC's DoD-per-UC
  record verdict: pass / fail / fail-deferred / no-test / skip

aggregate results
write results/latest/results.json (UC-keyed)
update charter Status board
print per-UC green/red table
```

## Verdict semantics

| Verdict | Meaning | Counts toward UCs-passing? | Blocks promotion? |
|---|---|---|---|
| `pass` (🟢) | All applicable layers pass; DoD-per-UC met | yes | no |
| `fail` (🔴) | Bug; one or more layers failed against DoD-per-UC | no | **yes** (gate) |
| `fail-deferred` (🟠) | Layer fails as expected because the feature is deferred by an explicit OD; DoD body cites the OD ID | no | **no** (documented limitation) |
| `no-test` (⚪) | UC has no test plan rows (DoD-per-UC missing) | no | yes (must close) |
| `skipped` (⏸) | UC filtered out by `--uc` / `--severity` / DoR-not-met | no | no |

## Output example (text)

```
Verifying tasks module — 2026-05-06 14:22Z
Charter: v0.1   Layers: all   UCs in scope: 54   Filtered: 54

UC ID    Severity   L1     L2     L3     DoD met?   Status
─────────────────────────────────────────────────────────────────
UC-001   core       ✓      —      —      yes        🟢 pass
UC-006   core       ✓      —      —      yes        🟢 pass
UC-053   security   ✓      ✗      —      no         🔴 fail (L2: workflow bypass — sec-1)
UC-073   qol        ✓      —      —      n/a        🟠 fail-deferred (OD-004)
UC-094   qol        ✓      —      —      n/a        🟠 fail-deferred (OD-005)
UC-040   core       —      —      —      no DoD     ⚪ no-test
...

Summary
  🟢 pass             8  (15%)
  🔴 fail             6  (bugs — block tier-1)
  🟠 fail-deferred    2  (OD-004, OD-005 — do NOT block tier-1)
  ⚪ no-test         40
  ⏸ skipped          0

Tier-1 DoD met?  ✗   blocked by 6 bug-fails + 40 missing DoDs
```

## Output example (JSON)

```json
{
  "module": "tasks",
  "charter_version": "0.1",
  "ran_at": "2026-05-06T14:22:00Z",
  "by_uc": {
    "UC-001": {"L1": "pass", "verdict": "pass"},
    "UC-053": {"L1": "pass", "L2": "fail", "verdict": "fail",
                "failures": [{"layer": "L2", "reason_kind": "bug",
                              "audit_finding": "sec-1"}]},
    "UC-073": {"L1": "pass", "verdict": "fail-deferred",
                "deferred_by": "OD-004"}
  },
  "totals": {"pass": 8, "fail": 6, "fail_deferred": 2, "no_test": 40, "uc_total": 54},
  "tier_1_dod_met": false,
  "tier_1_blockers": ["6 bug-fails", "40 missing DoD-per-UC"],
  "tier_1_deferred_passes": ["UC-073/OD-004", "UC-094/OD-005"]
}
```

## Cost note

Simulator-backed L2 scenarios run in seconds with **zero LLM token cost**. Full L1+L2 sweep over a typical module's ~50 UCs should complete in <60s on a developer laptop. L3 scenarios add minutes; typically run pre-promotion only.

## Failure handling

If any layer fails to *run* (harness error, build failure, malformed `.tasktest`), the UC verdict is `fail` with `reason_kind: harness-error`, NOT `pass`. A test-result of "no result" never becomes "pass."

## See also

- [`test-layers.md`](../test-layers.md) — what each layer covers
- [`e2e-test-authoring.md`](../e2e-test-authoring.md) — how the tests being run were authored
- [`quality-tooling.md`](../quality-tooling.md) — CI integration patterns
- [`verbs/promote.md`](promote.md) — `verify` is a precursor to `promote`
