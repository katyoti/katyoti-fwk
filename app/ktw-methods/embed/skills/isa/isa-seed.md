---
name: isa-seed
description: >-
  Bootstrap a DRAFT project ISA from an existing repo — README, configs, recent
  commits, test files, any pre-existing PRD/acceptance artifact — without
  inventing fiction. Refuses to overwrite; the result is explicitly partial and
  needs an interview pass. Workflow of the isa umbrella skill.
iconName: seedling
icon: "🌱"
hashtags: [isa, seed, bootstrap, articulation]
category: methodology
advertised: false
---

# ISA — Seed

Draft a project ISA from an existing repository that predates the ISA framework.
Seed mines real signals (it never fabricates) and produces a **draft** — a
human interview pass is mandatory before the ISA is treated as authoritative.
Shaped per `ktw-methods:/reference/isa/format-spec.md`.

## Inputs

| Input | Required | Description |
|---|---|---|
| project_path | yes | repository root (default write location for `ISA.md`) |
| target | yes | `isa-file` (default — `<project_path>/ISA.md`) or `cwm-items` (+ node ID) |
| name | no | project name; defaults to the basename of `project_path` |
| depth | no | default E3 (the project-ISA minimum); raise to E4/E5 for a fuller bootstrap |

## Refuse-if-exists

If the target already holds an ISA (the file exists, or the node already has a
`goal` item), STOP and say so — Seed never overwrites. To deepen an existing
project ISA, use `ktw-methods:/skill/isa-interview`.

## Procedure

### 1 — Read the contract + inventory the repo

Read `ktw-methods:/reference/isa/format-spec.md`. Then read, in priority order:
1. `README.md` — primary signal for Vision, Goal, sometimes Problem.
2. manifest/config (`package.json`, `go.mod`, `tsconfig.json`, deploy configs) —
   Constraints (runtime, frameworks, deploy target, dependency limits).
3. recent ~30 git commits — what's actively being built (Features).
4. pre-existing PRD-shaped artifacts (`PRD.md`, `SPEC*.md`, `acceptance.*`,
   `requirements.md`) — source material; cite them in `## Decisions`.
5. top-level directory structure (`src/auth`, `src/api`, …) — Feature units.
6. test files (`*_test.go`, `*.test.ts`) — each test name is a candidate ISC.

### 2 — Draft only what the sources support

- **Problem** — from a README "Why"/"Motivation" section, else inferred in 1–3
  sentences. **Vision** — the README headline pitch. **Out of Scope** — explicit
  "not / non-goals / we don't" lines. **Constraints** — from the configs.
  **Goal** — the clear deliverable, else the README headline.
- **Criteria** — port test names + any acceptance file to atomic ISCs (apply the
  splitting test, `format-spec.md`). With no tests, draft 6–10 conservative ISCs
  (build / typecheck / deploy + the obvious README outcomes), mark `[ ]`, and add
  a `## Decisions` line "ISC-1..N seeded from README — to be refined." Always
  include ≥1 `Anti:` criterion from Out of Scope.
- **Test Strategy** — populate the row for every ISC with an obvious probe
  (build/typecheck/deploy command); leave the rest as `# TODO`.
- **Features** — one per top-level module, cross-referenced with recent commits.

**Never invent fiction.** A thin README → a thin (or stubbed) Vision, not a
fabricated one. Use a `<!-- TODO: author … -->` stub where a source is absent.

### 3 — Skip the author-driven sections

Leave `## Principles`, `## Decisions` (beyond the seed note), `## Changelog`,
`## Verification` out — empty sections never appear. Add ONE `## Decisions` line:
"Seed-generated draft — run `ktw-methods:/skill/isa-interview` to fill Principles,
refine Vision/Goal, and audit Criteria."

### 4 — Materialize (draft)

Write per the target (file at `<project_path>/ISA.md`, or items on the node per
the format-spec CWM mapping). Frontmatter marks it a draft (low/zero progress).

### 5 — Surface the review reminder — do NOT check completeness

Return the path/node ID with **review_required: true**. A seeded ISA is
deliberately partial: do NOT run `isa-check-completeness` against it (it will
fail by design at E3+). The next step is an interview pass, not an audit.

## What Seed does NOT do

- Does not invent fiction (empty source → stub, not fabrication).
- Does not score AI-generated content (it drafts stubs; interview adds depth).
- Does not run check-completeness (seeded ISAs are explicitly partial).
- Does not commit (it writes; the user decides when to commit).

## Failure modes

- **No README / no prose source** — abort; Seed needs at least one prose signal.
  Suggest `ktw-methods:/skill/isa-scaffold` from a prompt instead.
- **Huge history** (>5000 commits) — sample the most recent ~100 for Features.
- **ISA already present** — abort; refuse to overwrite; suggest interview.
