# Verb — `code-module plan-tier <n> <module>`

Compute the delta between current state and tier-N DoD; file one request per gap in the module (`ktw-agents requests file`), gathered in a tier milestone.

## Inputs

- `<n>` — tier number (1, 2, 3)
- `<module>` — module name
- `--dry-run` — print the planned requests without filing them
- `--diff` — only file requests the milestone does not already include
- `--group-by <key>` — sub-milestone grouping strategy (default: by-pillar/theme)

## When to use

- After Specify is complete (charter has DoD-per-UC for `core` and `security` UCs)
- After an audit closes; the new findings require new requests
- When a previous milestone is exhausted and tier-N+1 is the next target

## Preconditions (refuses without these)

Per [Lesson 2](../lessons-from-tasks-bootstrap.md), generating test-creation tasks without a procedure to follow produces inconsistent tests + drift. Plan-tier refuses unless:

- `/modules/<module>/charter.md` exists at version ≥0.1
- `/ktw-code/modules/e2e-test-authoring.md` is loadable
- `/modules/<module>/tests/L2/_template.tasktest` exists (or module's manifest declares `no_l2: true`)
- `/modules/<module>/tests/README.md` exists
- Open decisions blocking this tier are either decided (state `applies`) OR explicitly carry a `question` request in the gap list

`--force` available for emergencies; recorded in `manifest.json.forced_plan_tiers[]`.

## Behavior

```
verify preconditions; refuse if any missing (unless --force)
read charter, latest audit, latest results, manifest
compute gap list against tier <n> DoD:
  - UCs without DoD-per-UC
  - UCs with failing tests (verdict = fail; not fail-deferred)
  - audit findings unmitigated (critical, high, then medium per tier policy)
  - open decisions assigned to this tier
  - missing dimension matrix coverage at L2

milestone_name = "ms-<module>-rs-tier-<n>"
ensure the milestone object milestones.<milestone_name> exists (cwm object-create, type milestone, if not)

if gap_count > 20 (or per --batch-threshold):
  group gaps thematically:
    - foundations (resolved-OD work; structural)
    - security fixes
    - lifecycle hygiene
    - test-gap closures (L1)
    - L2 scenarios (per-dimension)
    - housekeeping
  create sub-milestones using msX.Y.Z notation

for each gap:
  determine kind (fix / build / question / review) and priority
  build the full request (title, details, deliverables, on — see "Request content shape")
  if --diff and the milestone already includes a request with that title: skip (with notice)
  otherwise: ktw-agents requests file … (non-interactive), then an includes edge from the milestone

update manifest.json.active_milestone (or .active_sub_milestones[] if sub-milestoned)
print summary
```

## Request content shape (`ktw-agents requests file`)

```
--module <module>   --kind fix|build|question|review   --priority P0|P1|P2
--by '{"id":"<module>","kind":"module"}'
--title "<the outcome, one sentence>"
--details "<full body — UC IDs, audit findings (bundle + finding ID, file:line),
            open decisions; for test-creation requests: cite
            e2e-test-authoring.md as the procedure>"
--deliverables '[{"title":"<lifted from DoD-per-UC>","check":"<a command>"}]'
--on <the use case it advances>   [--depends_on <ids>]
```

## Sub-milestone naming (per OD-C)

When `plan-tier` finds more than 20 gaps, it groups them into sub-milestones with `msX.Y.Z` notation. `ms-tasks-rs-tier-1` includes the sub-milestones; `ms-tasks-rs-tier-1.1`, `ms-tasks-rs-tier-1.2`, `ms-tasks-rs-tier-1.3` include the requests.

Default grouping (`--group-by pillar`):

- `.1` — Foundations (resolved-OD work)
- `.2` — Critical fixes + housekeeping
- `.3` — Test gap closures + L2 scenarios

Alternatives via `--group-by`: `severity` / `layer` / `flat`.

## Test-creation request requirements (per Lesson 2)

Every request that says "create a test for UC-X" must:

- Cite [`e2e-test-authoring.md`](../e2e-test-authoring.md) as the procedure to follow
- Specify the layer (L1 / L2 / L3) explicitly
- For L2: copy `_template.tasktest` as the starting point — never invent a new format
- For L2: reference fixtures from `fixtures/` (never inline content)
- For security UCs: include `journal_must_not_contain` (negative DoD)

Plan-tier embeds these requirements in the filed requests' `details` and `deliverables`.

## Output

- A project release per tier (`<module>-rs-tier-<n>`, in `sys-planification`), its requests targeting it
- One request per gap in the module, with its details, deliverables and the UC it is on
- Updated `manifest.json` with `active_milestone` (and `active_sub_milestones[]` if applicable)
- Summary report of N requests planned (M filed, K skipped as existing)

## See also

- [`workflow.md`](../workflow.md) — where Plan-tier fits in the lifecycle
- [`charter-template.md`](../charter-template.md) — the DoD-per-UC and tier-N DoD source
- [`relationship-with-planification.md`](../relationship-with-planification.md) — the non-interactive `requests file` contract
- [`lessons-from-tasks-bootstrap.md`](../lessons-from-tasks-bootstrap.md) Lesson 2 — why preconditions matter
