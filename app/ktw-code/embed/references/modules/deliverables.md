# Deliverables — What Each Module Ships

Every module that adopts the rock-solid method maintains a **bundle** of artifacts under `/modules/<module>/`. The bundle is what you point at when you ask "is this module production-ready?" — it carries the spec, the evidence, the audit trail, and the test results.

## Bundle structure

```
/modules/<module>/
├── README.md                  ← module-specific entry point
├── charter.md                 ← single source of truth (see charter-template.md)
├── charter-archive/           ← historical charter versions
│   ├── v0.1.md
│   ├── v1.0.md
│   └── ...
├── specs/                     ← per-UC detail files (only for non-trivial UCs)
│   ├── uc-001.md
│   ├── uc-002.md
│   └── ...
├── fixtures/                  ← test inputs, scripted-LLM responses, sample home dirs
│   ├── home-with-coord-task/
│   ├── llm-script-coord-tries-write.json
│   └── ...
├── tests/                     ← test catalog (not the test code itself; that's in /internal/)
│   ├── L1/
│   │   └── catalog.md         ← UC ↔ Go test mapping (mirror of charter's Test plan)
│   ├── L2/
│   │   ├── uc-001-worker.tasktest
│   │   ├── uc-002-coord-escalation.tasktest
│   │   └── ...
│   └── L3/
│       ├── uc-014-cross-team.task.md
│       └── ...
├── audits/                    ← dated audit bundles (immutable once written)
│   ├── 2026-05-06_07-56/      ← e.g. the 13-report ktw-code review
│   │   ├── summary.md
│   │   ├── architecture.md
│   │   ├── ...
│   │   └── manifest.json      ← what was reviewed, by whom, against which git SHA
│   ├── 2026-06-15_10-30/
│   └── ...
├── results/                   ← test-run results (overwritten or dated)
│   ├── latest/
│   │   ├── results.json
│   │   ├── L1.txt
│   │   ├── L2.log
│   │   └── L3.log
│   ├── 2026-05-06/
│   └── ...
└── manifest.json              ← bundle-level metadata
```

Not all subfolders need to exist on day one. Bootstrap creates `README.md` and `charter.md`; the rest grow as the module advances through the workflow.

## Per-folder contract

### `README.md` (mandatory)

Module-specific entry point. Two paragraphs maximum:

1. What the module *is* (one sentence) and where the source lives (path).
2. Where the reader should go next: charter, latest audit, latest results.

It does **not** duplicate the charter. Its only job is "you are here; here's the index."

### `charter.md` (mandatory)

The structure defined in [`charter-template.md`](charter-template.md). The single source of truth. Versioned monotonically. **Never overwritten in place** — significant changes require a `charter-archive/` snapshot first.

### `charter-archive/` (mandatory once the charter is at v1.0+)

Append-only. One file per published charter version. Lets a reader compare what the module promised at v1.0 vs v1.1 vs v2.0.

Filename pattern: `v<major>.<minor>.md`. Snapshots taken at:
- Tier promotion (always)
- Charter version bumps (always)
- Major spec re-shapes (recommended)

Snapshots are byte-for-byte copies of the live `charter.md` at the moment of the snapshot.

### `specs/` (optional)

When a UC's table row in the charter is too terse to test against, expand it here. One file per UC: `specs/uc-001.md`. Each file contains:

- The full UC text (trigger, expected outcome, edge cases)
- The DoR
- The DoD-per-UC (concrete pass criteria)
- Cross-references (to which dimension matrix cells it covers, which audit findings it closes)
- Open questions (if any)

Trivial UCs — e.g. "the loader can parse a Sequence node" — don't need their own spec file. The charter's table row suffices.

### `fixtures/` (mandatory for L2/L3)

Input data the tests consume. Anything bigger than 5 lines should live here, not inline in a test.

Naming: descriptive, immutable. Once a fixture is referenced by a test, **don't edit it** — copy + version (`llm-script-coord-tries-write-v2.json`). Tests pin to specific fixture versions; in-place edits silently change what the test verifies.

### `tests/` (mandatory)

The test catalog. **Not the test code** — Go tests live next to the source under `internal/`. This folder catalogs them and holds L2/L3 scenarios.

- `tests/L1/catalog.md` — table of UC ↔ `package.TestName` mappings; mirror of the charter's Test plan column. Auto-regenerable from `go test -list`.
- `tests/L2/` — declarative scenarios (`.tasktest` files or directories).
- `tests/L3/` — TASK.md files an orchestrating agent runs.

Why the catalog exists: when a UC fails, the catalog tells you which test files prove (or fail to prove) that UC. Without it, "UC-002 is broken" is a manual hunt.

### `audits/` (mandatory)

Dated, immutable. Each audit run lands in its own subfolder. The folder name is the audit date in `YYYY-MM-DD_HH-MM` form.

Each audit subfolder contains:

- The full reports produced by the audit (e.g. the 13 markdown files from `ktw-code:code-review`)
- A `summary.md` synthesizing the findings (already part of the `code-review` skill's output format)
- A `manifest.json` recording: who ran the audit, against which git SHA, against which charter version, and how the audit was invoked

```json
// audits/2026-05-06_07-56/manifest.json
{
  "date": "2026-05-06T07:56:00Z",
  "module": "tasks",
  "module_path": "internal/extensions/task/",
  "git_sha": "847e3838",
  "charter_version": "0.1",
  "audit_kind": "ktw-code:code-review",
  "reviews_run": ["architecture","correctness","security","quality","complexity",
                   "library-audit","performance","test-coverage","cli-ux","data-model",
                   "package-layout","threat-model","red-team-architecture"],
  "agents": 13
}
```

Old audits are **never deleted, never edited**. The audit ledger in the charter indexes them.

### `results/` (mandatory)

Test-run artifacts. Two layouts coexist:

- `results/latest/` — overwritten on every run; holds the most recent results
- `results/<date>/` — preserved per-run results when explicitly archived (typically at tier promotion, or when a notable run happens)

Each results folder contains:

- `results.json` — the structured summary (UC ↔ layer ↔ pass/fail; format defined in [`test-layers.md`](test-layers.md))
- `L1.txt` — `go test` output
- `L2.log` — harness output for the scenarios
- `L3.log` — orchestrating-agent transcripts (or pointers to journal IDs)

Auto-archival policy: `results/latest/` gets snapshotted into `results/<date>/` on (a) tier promotion and (b) any run that flips a UC's status.

### `manifest.json` (mandatory)

Bundle-level metadata. The single file an external reader (e.g. a release-tracking tool) parses to know "what does this module declare about itself?"

```json
// /modules/tasks/manifest.json
{
  "module": "tasks",
  "source_paths": ["internal/extensions/task/"],
  "module_kind": "extension",
  "charter_version": "0.1",
  "current_tier": 0,
  "tier_target": 1,
  "linked_goals": ["m.G-001", "m.G-002"],
  "open_decisions": [{"id": "OD-001"}, {"id": "OD-002"}],
  "resolved_decisions": [{"id": "OD-003", "resolution": "accepted"}],
  "coverage": {
    "source": "use-cases/",
    "uc_total": 47,
    "uc_by_status": {"pass": 8, "no-test": 37, "fail-deferred": 2},
    "deferred_uc_ids": ["UC-073", "UC-094"],
    "ac_total": 120,
    "ac_by_status": {"pass": 30, "no-test": 86, "fail-deferred": 4},
    "ac_by_layer": {"L1": 104, "L2": 16},
    "anti_total": 6,
    "coverage_percent": 25.9,
    "uc_by_severity_status": {"core": {"pass": 6, "no-test": 25}, "security": {"pass": 2, "no-test": 12}},
    "last_verified_at": "2026-05-06T08:30:00Z"
  },
  "last_audit": "2026-05-06_07-56",
  "audit_findings_open": {"critical": 0, "high": 0, "medium": 3},
  "lint_status": "clean"
}
```

The `coverage` block is computed from the use cases with the latest `test-run` verdicts (a use case never carries its own status); `last_verified_at` is when that run happened.

The L3 skill reads/writes this on every `status` invocation. Other tooling (CI, dashboards) can read it without parsing markdown.

## Versioning

The bundle as a whole versions with the charter:

- `charter v0.x` → drafts; bundle is fluid
- `charter v1.0` → first tier-1 promotion; archive snapshot taken; bundle becomes a published artifact
- `charter v1.x` → minor evolution within tier 1; no archive snapshot unless a meaningful change
- `charter v2.0` → tier-2 promotion OR breaking spec change; archive snapshot mandatory

A single `git tag module-tasks-v1.0` at promotion time pins the entire bundle to a reproducible point in history.

## What a "tier 1 promotion bundle" must contain

When the L3 skill says "promote tasks to tier 1," the bundle must demonstrate:

| Required artifact | Tier-1 requirement |
|---|---|
| `charter.md` | All `core` and `security` UCs have DoR + DoD-per-UC |
| `charter.md` Status board | UCs passing ≥80%; zero open critical/high audit findings |
| `audits/<latest>/` | Most recent audit dated; criticals/highs zero or mitigated |
| `tests/L1/`, `tests/L2/` | Coverage matches charter's Test plan |
| `results/latest/results.json` | `tier_1_dod_met: true` |
| `manifest.json` | `current_tier: 1` (post-promotion) |
| `charter-archive/v1.0.md` | Snapshot of `charter.md` at the moment of promotion |

If any of these is missing, the L3 skill refuses to promote.

## What the bundle is NOT

- **Not a CMS.** No editorial workflow, no review queue, no comments. Markdown + JSON in git.
- **Not a duplicate of source-code documentation.** Godoc still owns symbol-level docs; the charter cites them.
- **Not a dashboard.** `manifest.json` is structured enough to power one, but the bundle itself is files-on-disk.
- **Not a test runner.** The `results/` folder holds outputs; the runner lives elsewhere (Go test, harness scripts, the L3 agent).

## Lifecycle of a bundle artifact

| Artifact | Created when | Updated when | Archived when | Deleted when |
|---|---|---|---|---|
| `charter.md` | Bootstrap | Every spec change, every workflow phase | Charter version bump (snapshot to archive) | Never |
| `specs/uc-*.md` | When charter row is too terse | When UC is refined | UC retired (move to `specs/retired/`) | Never |
| `fixtures/*` | When a test needs them | Never (versioned copy instead) | Never | Only if no test references them and audit confirms safety |
| `tests/L1/catalog.md` | Bootstrap | After every Go test add/remove | Never | Never |
| `tests/L2/*.tasktest` | When UC needs L2 coverage | When UC's DoD-per-UC changes | UC retired | UC retired (rare) |
| `audits/<date>/` | Each audit run | Never (immutable) | Never | Never |
| `results/latest/` | First test run | Every test run | At tier promotion or status flip | Never |
| `results/<date>/` | Snapshot of `latest/` at notable runs | Never (immutable) | Never | Never |
| `manifest.json` | Bootstrap | Every L3 skill invocation | Never (it IS the snapshot) | Never |

## Cross-bundle relationships

Bundles can reference each other. E.g. the `tasks/` bundle's UC-014 depends on the `agent/` bundle's plugin-hook contract.

- Cross-references live **only** in the charter (Concepts, or Function inventory cross-cite).
- A cross-reference uses a stable form: `[agent: UC-008](../agent/charter.md#uc-008)`.
- A breaking change to a cross-cited symbol bumps both modules' charter versions.

This is rare and should be flagged in Open decisions before becoming a hard dependency.
