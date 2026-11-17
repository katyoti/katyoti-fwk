# Verb — `code-module charter <module>`

Bootstrap a charter for a new module, or refresh one in place after major spec drift.

## Inputs

- `<module>` — module name (matches the folder under `/modules/`)
- `--source <path>` (optional) — override module source path; defaults to `manifest.json.source_paths[0]`
- `--baseline-audit` (flag) — run a baseline audit if no audit exists yet
- `--refresh` — open an existing charter for review and detect drift; do not bootstrap

## When to use

- New module that doesn't have a charter yet
- Existing charter has drifted enough that re-bootstrapping is cleaner than incremental edits
- A new audit found UCs that aren't in the inventory

## Behavior

```
if /modules/<module>/charter.md does not exist:
  // Bootstrap path — see e2e-test-authoring.md and lessons-from-tasks-bootstrap.md (Lesson 2)
  // for the 12-step procedure.
  create folder
  copy /ktw-code/modules/charter-template.md → /modules/<module>/charter.md
  fill Meta.Module path, Meta.Charter version: 0.1, Meta.Current tier: 0
  if --baseline-audit:
    run audit (see verbs/audit.md)
    populate audit ledger with the result
  prompt user to fill Concepts (interactive — minimum viable)
  // CRITICAL: bootstrap test infrastructure (Lesson 2 precondition for plan-tier)
  create /modules/<module>/tests/{L1,L2,L3}/
  create /modules/<module>/fixtures/
  copy /ktw-code/modules/templates/template.tasktest → /modules/<module>/tests/L2/_template.tasktest
  write /modules/<module>/tests/README.md pointing at e2e-test-authoring.md
  print: "Charter bootstrapped at v0.1. Run plan-tier 1 <module> after specifying UCs."
else:
  open the charter for review
  detect any drift signals (UCs in source/tests not in charter; charter UCs with no test plan rows)
  produce a "drift report" and offer to spawn `decision-…` tasks for unresolved drift
```

## Output

- `/modules/<module>/charter.md` (new or refreshed)
- `/modules/<module>/manifest.json`
- `/modules/<module>/tests/{L1,L2,L3}/` skeletons
- `/modules/<module>/fixtures/` (empty)
- `/modules/<module>/tests/L2/_template.tasktest` (per-module copy)
- `/modules/<module>/tests/README.md`

## Bootstrap step order (12 steps)

The order matters. Captured from [`lessons-from-tasks-bootstrap.md`](../lessons-from-tasks-bootstrap.md):

1. Survey source for ground-truth (read presets, public types, imports)
2. Relocate or copy any existing audit to `/modules/<module>/audits/<date>/` + manifest
3. Write `manifest.json` for the module bundle
4. Write `charter.md` skeleton — Meta + Concepts + glossary
5. Build UC inventory from FOUR sources (see [`charter-template.md`](../charter-template.md))
6. Function inventory + dimension matrix + spawn policies + public API contract
7. DoR + DoD per module (Tier 1/2/3) + audit ledger + status board + ODs + glossary
8. **Set up test infrastructure scaffold** (per Lesson 2 — required before plan-tier)
9. Update `manifest.json` to match charter state

## Preconditions

- `/modules/docs/` (project doc tree) exists
- The four method-level method docs are loadable: `charter-template.md`, `workflow.md`, `e2e-test-authoring.md`, `test-layers.md`
- `/ktw-code/modules/templates/template.tasktest` is loadable (master template)

## Failure modes

- Source path doesn't exist → refuse with clear error
- Charter already exists at v≥1.0 → refuse without `--refresh`; promote a tier instead

## See also

- [`charter-template.md`](../charter-template.md) — the per-module structure
- [`workflow.md`](../workflow.md) — where Bootstrap fits in the lifecycle
- [`lessons-from-tasks-bootstrap.md`](../lessons-from-tasks-bootstrap.md) — heuristics from the first real run
- [`deliverables.md`](../deliverables.md) — the bundle inventory
