# ISA format spec — the file-shape contract

The canonical shape of an ISA. **This spec wins on contradiction** with any
skill prose; skills update to match it, never the reverse.

## Frontmatter

```yaml
---
subject: "8-word description of the thing articulated"
depth: E3                 # E1..E5
phase: observe           # observe | build | verify | done
progress: 0/12           # passed-ISCs / total-ISCs
updated: 2026-06-12
---
```

All five keys required. `progress` counts checked ISC boxes over total.

## The twelve-section body — fixed order

| # | Section | Purpose |
|---|---|---|
| 1 | `## Problem` | what is broken or missing now |
| 2 | `## Vision` | experiential intent — what "euphoric surprise" looks like |
| 3 | `## Out of Scope` | anti-vision, declared in prose |
| 4 | `## Principles` | substrate-independent truths the work must respect |
| 5 | `## Constraints` | immovable mandates that bound the solution space |
| 6 | `## Goal` | the hard-to-vary spine — 1–3 sentences naming verifiable done |
| 7 | `## Criteria` | atomic ISCs, one binary probe each, incl. `Anti:` / `Antecedent:` |
| 8 | `## Test Strategy` | per-ISC verification: `isc \| type \| check \| threshold \| tool` |
| 9 | `## Features` | work breakdown: `name \| satisfies \| depends_on \| parallelizable` |
| 10 | `## Decisions` | timestamped log incl. dead ends; `refined:` prefix for restructures |
| 11 | `## Changelog` | conjectured / refuted-by / learned / criterion-now entries |
| 12 | `## Verification` | evidence per ISC — quoted output, file content, screenshot path |

Sections never appear empty: a section is either populated or absent. The
depth gate decides which MUST be present. The **Goal** is the tightest verbal
form of the whole artifact: if the Goal can be edited without changing the
ISC set, the Goal is not load-bearing; if the Goal stays but the ISCs drift
from it, the decomposition is wrong.

## Tier completeness gate

| Tier | Required sections | ISC floor |
|---|---|---|
| E1 | Goal, Criteria | — |
| E2 | + Problem, Test Strategy | ≥16 (soft) |
| E3 | + Vision, Out of Scope, Constraints, Features | ≥32 (soft) |
| E4 | all twelve | ≥128 (hard) |
| E5 | all twelve + interview pass before build | ≥256 (hard) |

Soft floors are relaxable with a justification recorded in `## Decisions`.
The floor is a count anchor only — the mix of criteria is judgment.

## ISC grammar

```markdown
- [ ] ISC-1: <one verifiable claim> (probe: <command/check returning yes-no>)
- [ ] ISC-2: Anti: <what must NOT happen> (probe: ...)
- [ ] ISC-3: Antecedent: <precondition that produces the target experience> (probe: ...)
```

- **One binary probe per ISC.** Split until a single Read/Grep/command/
  screenshot/query answers yes-or-no. If no probe is nameable, the claim is
  fluff — rework it.
- **Splitting test** — split when the criterion: joins two verifiable things
  ("and"/"with"/"including"); has a part A that passes while B fails; says
  "all"/"every"/"complete" (enumerate); crosses a domain boundary (one per
  boundary); has no nameable probe.
- **`Anti:`** — ≥1 required at every depth, derived from Out of Scope +
  regression concerns.
- **`Antecedent:`** — ≥1 required when the goal is experiential (art,
  design, content — anything that must "land").

## ID stability

ISC IDs **never re-number on edit**. Splitting `ISC-7` yields `ISC-7.1` /
`ISC-7.2` (parent preserved). A dropped ISC leaves a tombstone:

```markdown
- [ ] ISC-7: [DROPPED — see Decisions 2026-06-12]
```

Renumbering silently breaks every reference in Decisions, Changelog, and
Verification — it is a hard violation.

## Guardrail taxonomy

| Guardrail | Binds | Lives in |
|---|---|---|
| Principles | the thinking | `## Principles` |
| Constraints | the solution space | `## Constraints` |
| Out of Scope | the vision | `## Out of Scope` |
| Anti-criteria | the test surface | `## Criteria` with `Anti:` prefix |

The first three are author-stated prose; anti-criteria are how they become
probe-able.

## Append-only section formats

`## Decisions` — one line per entry, timestamped, dead ends marked:

```markdown
- 2026-06-12 14:00: Hand-rolled parse over a library — bounded shape, zero-deps constraint.
- 2026-06-12 18:30: ❌ DEAD END: Promise.all over 100 IDs — rate-limited at 12. Don't retry.
- 2026-06-13 09:00: refined: split ISC-7 into ISC-7.1/ISC-7.2 (domain boundary).
```

`## Changelog` — every entry needs all four pieces, in order; a partial
entry is a Decision, not a Changelog:

```markdown
- conjectured: <what we believed>
  refuted by: <the observation that broke it>
  learned: <the corrected understanding>
  criterion now: <the ISC added/changed as a result>
```

`## Verification` — evidence per ISC: the probe run + its quoted output (or
file content / screenshot path).

## CWM materialization (`target: cwm-items`)

The same content as CWM items on a target node — one method, two
materializations:

| ISA section | CWM shape |
|---|---|
| Goal | the direction it serves: one `goal` (always `want.reachability: asymptotic` — no test ever says it is done); the thing a test declares done is the `feature` the ISA is about |
| Criteria (each ISC) | one `acceptance-criterion` object (`checkable`: the probe; `Anti:` criteria too) under the `use-case` it proves, or directly under the feature; ids `<feature-or-uc-id>-AC<n>` |
| Features (each row) | one `feature` object (always `closable`; a sub-feature has its feature as parent) with a `serves` edge to the goal; epic and user story are size words, never types. What it produces is a `deliverable` record |
| Test Strategy (each row) | one `test-case` item (+ `verifies` edge to its use-case) |
| Decisions | `decision` items |
| Problem, Vision, Out of Scope, Principles, Constraints, Changelog, Verification | prose sections of the node body, same order |

AC record IDs follow the ID-stability rule exactly as ISC lines do (split
`AC-7` → `AC-7.1`/`AC-7.2`, tombstone on drop). The depth gate and all ISC
rules are materialization-independent.
