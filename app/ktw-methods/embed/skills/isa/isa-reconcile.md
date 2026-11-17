---
name: isa-reconcile
description: >-
  Merge an ephemeral / working ISA back into its master, keyed on stable ISC
  IDs — adds, edits, drops (with tombstones), and unions the append-only
  sections, NEVER renumbering. Workflow of the isa umbrella skill.
iconName: git-merge
icon: "🔀"
hashtags: [isa, reconcile, merge, articulation]
category: methodology
advertised: false
---

# ISA — Reconcile

Merge a working copy of an ISA (edited in an ephemeral session, a branch, or a
parallel agent) back into the master ISA. The merge is **deterministic and keyed
on stable ISC IDs** — it matches criteria by ID, never by text or position, so
references in Decisions / Changelog / Verification stay valid. Shaped per
`ktw-methods:/reference/isa/format-spec.md` (the spec wins on contradiction).

## Inputs

| Input | Required | Description |
|---|---|---|
| master | yes | the canonical ISA (file path or CWM node ID) — the merge TARGET |
| working | yes | the edited copy to fold in (path / node ID / inline content) |
| target | yes | `isa-file` (+ path) or `cwm-items` (+ node ID) — must match the master's materialization |

## The cardinal rule — match on ID, never renumber

Every ISC carries a stable ID (`ISC-7`, split children `ISC-7.1`). Reconcile
pairs master↔working lines **by ID only**. Renumbering, or re-pairing by text
similarity / list order, silently breaks every reference — it is a hard
violation (`format-spec.md` §ID stability). The same rule applies to
`acceptance_criteria[]` record IDs under `cwm-items`.

## Procedure

### 1 — Read the contract + both sides

Read `ktw-methods:/reference/isa/format-spec.md`. Load master and working;
index each side's ISCs by ID into a map. Note each side's depth.

### 2 — Diff by ID into four buckets

Walk the union of IDs:

- **unchanged** — same ID, same claim + probe on both → keep master verbatim.
- **edited** — same ID, changed claim/probe → take the working version; record
  the change in `## Decisions` (`refined: ISC-<id> reworded …`).
- **added** — ID only in working → insert; if it's a split of an existing
  master ISC, use the child grammar (`ISC-7` → `ISC-7.1`/`ISC-7.2`, parent
  preserved), never a fresh top number that collides.
- **dropped** — ID only in master AND explicitly removed in working (not merely
  absent because working was a partial view) → leave a **tombstone**, do not
  delete: `- [ ] ISC-7: [DROPPED — see Decisions <date>]`, with the reason in
  `## Decisions`.

Absence is NOT a drop. Only treat an ID as dropped when the working copy is a
full view that intentionally removed it; when in doubt, keep the master line and
note the ambiguity in `## Decisions`.

### 3 — Union the append-only sections

`## Decisions`, `## Changelog`, `## Verification` are append-only — never
rewrite an existing line. Union both sides by (timestamp + content), drop exact
duplicates, keep chronological order. A `## Changelog` entry missing any of its
four pieces (conjectured / refuted-by / learned / criterion-now) is a Decision,
not a Changelog — file it accordingly.

### 4 — Reconcile the prose guardrails

Problem / Vision / Out of Scope / Principles / Constraints: take the working
version when it changed, but record the guardrail change in `## Decisions` (a
loosened Constraint or narrowed Out-of-Scope is a real decision, not a silent
edit).

### 5 — Materialize + verify

Write the merged ISA to the master target (`isa-file` path or `cwm-items` node,
per the format spec). Then run `ktw-methods:/skill/isa-check-completeness` at the
master's depth; fix any hard gap the merge introduced before declaring done.
Return the master path / node ID.

## Failure modes

- **ID collision** — working invented a top-level ID that already means
  something else in master: stop, surface it; never overwrite by number.
- **Renumber temptation** — gaps after drops are EXPECTED (tombstones hold the
  slot). Do not "tidy" the numbering.
- **Lossy append** — never collapse or rewrite Decisions/Changelog/Verification
  history; only add.
- **Materialization mismatch** — master is `cwm-items` but working is a file (or
  vice-versa): convert via the format-spec mapping table, don't merge raw.

## Idempotency

Reconcile is idempotent: re-running it on the same working copy against the same
master produces no further changes (unchanged ISCs no-op, append-only unions
drop duplicates) and reports zero applied changes. Safe to retry. Append entries
it stages pass the same shape validation as `ktw-methods:/skill/isa-append` —
the merge cannot smuggle in a malformed Changelog/Verification entry.
