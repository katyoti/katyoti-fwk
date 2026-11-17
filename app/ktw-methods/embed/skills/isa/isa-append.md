---
name: isa-append
description: >-
  Canonical writer for an ISA's append-only sections — Decisions, Changelog
  (the four-piece conjectured/refuted-by/learned/criterion-now entry), and
  Verification. Refuses partial entries; refuses Verification for an unknown ISC
  ID. Workflow of the isa umbrella skill.
iconName: text-plus
icon: "➕"
hashtags: [isa, append, decisions, changelog, verification]
category: methodology
advertised: false
---

# ISA — Append

The single, canonical writer for an ISA's three append-only sections. These
formats (especially the four-piece Changelog) degrade under free-form editing —
routing every entry through here keeps them clean and cross-project consistent.
Shaped per `ktw-methods:/reference/isa/format-spec.md` (the spec wins).

## Inputs

| Input | Required | Description |
|---|---|---|
| target | yes | `isa-file` (+ path) or `cwm-items` (+ node ID) |
| type | yes | `decision` · `changelog` · `verification` |
| fields | yes | the entry pieces for that type (below) |

## The three entry types

### Decision — a timestamped log line

```
- YYYY-MM-DD HH:MM: <decision text>
- YYYY-MM-DD HH:MM: refined: <what was restructured and why>   # Goal/ISC-set changes
- YYYY-MM-DD HH:MM: ❌ DEAD END: tried <X> — failed because <Y> (don't retry)
```
Required: `text`. Optional: `kind` (`decision` | `refined` | `dead-end`).

### Changelog — the four-piece conjecture/refutation/learning entry

```
- YYYY-MM-DD | conjectured: <what we believed>
  refuted by: <the observation that broke it>
  learned: <the corrected understanding>
  criterion now: <the ISC added / changed / dropped as a result>
```
Required: ALL FOUR (`conjectured`, `refuted_by`, `learned`, `criterion_now`).
**A partial entry is a Decision, not a Changelog** — if any of the four is
missing, refuse, name the missing piece, and ask (or file it as a Decision).

### Verification — ISC-keyed evidence

```
- ISC-N: <probe type> — <one-line evidence: quoted probe output / file / screenshot path>
```
Required: `isc_id` (MUST exist in `## Criteria`), `probe_type`, `evidence`
(quoted verbatim from the tool output).

## Procedure

### 1 — Resolve the target section

Read the ISA (file body, or the CWM node — Decisions map to `decision` items;
Changelog/Verification are prose sections of the node body, per the format-spec
mapping). If the section is absent, create it in canonical order: Decisions after
Features, Changelog after Decisions, Verification last.

### 2 — Validate the entry shape (refuse mode)

| Type | Refuse when… |
|---|---|
| decision | `text` empty |
| changelog | any of the four C/R/L pieces missing |
| verification | `isc_id` not in `## Criteria`, or `evidence` empty |

On failure, do NOT write — surface the missing piece. Silently writing partial
entries defeats the whole purpose of a canonical writer.

### 3 — Format + append

Format with the schema above verbatim (four-line indented form for Changelog).
Append to the END of the target section, preserving prior entries — never
rewrite or reorder existing lines (these sections are append-only).

### 4 — Flip the ISC on a Verification entry

When a Verification entry corresponds to a previously-open ISC, also mark that
ISC done in `## Criteria` (or set the `acceptance_criteria[]` record's status
under `cwm-items`). IDs are matched exactly — never renumber (`format-spec.md`
§ID stability).

### 5 — Return the appended block

Echo the exact text appended + the target, so the caller can confirm.

## Failure modes

- **Partial Changelog** — missing refutation or learning: refuse; it's a Decision.
- **Verification for an unknown ISC** — refuse; the ID must exist (the same
  ID-stability contract `ktw-methods:/skill/isa-reconcile` relies on).
- **Missing section, ambiguous position** — if the file is malformed and the
  canonical slot is unclear, abort and surface the structural problem.
- **Concurrent edits** — Append reads-then-writes; serialize structural ISA edits.
