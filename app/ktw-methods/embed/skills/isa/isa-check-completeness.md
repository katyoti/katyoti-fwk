---
name: isa-check-completeness
description: >-
  Score an existing ISA against the depth completeness gate (E1–E5) and emit a
  structured pass/fail + gap report — section classification, ISC quality
  audit, severity table. Workflow of the isa umbrella skill.
iconName: checklist
icon: "✅"
hashtags: [isa, audit, completeness]
category: methodology
advertised: false
---

# ISA — CheckCompleteness

Score an ISA against the depth gate and return a structured report. This is
the hard gate other workflows call before declaring anything done.

## Inputs

| Input | Required | Description |
|---|---|---|
| isa | yes | The ISA to score — a file path or a CWM node ID |
| depth | yes | The bar to score against (E1–E5) |
| strict | no | Default true; false downgrades hard fails to warnings (mid-interview use) |

## Procedure

### 1 — Read and parse

Load the ISA (file, or node body + items for `cwm-items`). Parse the
frontmatter and section headers. Malformed or missing frontmatter is an
immediate error — surface it, do not guess.

### 2 — Classify each required section

| Tier | Required sections |
|---|---|
| E1 | Goal, Criteria |
| E2 | Problem, Goal, Criteria, Test Strategy |
| E3 | Problem, Vision, Out of Scope, Constraints, Goal, Criteria, Features, Test Strategy |
| E4 | all twelve |
| E5 | all twelve + evidence an interview pass ran before build |

Per section:

| Class | Test |
|---|---|
| `present` | header exists, body ≥2 sentences (or ≥3 bullets) |
| `thin` | header exists, body ≤1 sentence (or ≤2 bullets) |
| `missing` | header absent |
| `empty` | header exists, body whitespace — acceptable ONLY for `Verification` before the verify phase |

### 3 — Audit ISC quality

Walk every ISC in `## Criteria`:
- **Granularity** — one binary probe each; compound "and/with" criteria fail.
- **Tier floor** — E2 ≥16, E3 ≥32 (soft: relaxable with a `## Decisions`
  justification); E4 ≥128, E5 ≥256 (hard).
- **Anti-criteria** — ≥1 `Anti:` ISC. Zero is a hard fail at every depth.
- **Antecedent** — when the goal is experiential, ≥1 `Antecedent:` ISC.
- **ID stability** — unique IDs, no renumbering gaps; tombstones
  (`ISC-7: [DROPPED — see Decisions YYYY-MM-DD]`) are valid.

### 4 — Emit the report

```yaml
status: pass | fail            # pass = zero hard gaps
depth: E3
sections:                      # every required section, classified
  Problem: present
  Vision: thin
  ...
gaps:
  - section: Vision
    severity: hard
    reason: thin — one sentence at E3
isc_quality:
  total: 24
  depth_floor: 32
  under_floor: true            # soft at E2/E3
  granularity_violations: 0
  anti_criteria_count: 2
  antecedent_present: true
  id_stability_violations: 0
```

### 5 — Severity rules

Hard at every depth: Goal or Criteria missing · zero `Anti:` · missing
`Antecedent:` on an experiential goal · any granularity violation · any
ID-stability violation. Hard from the depth where the section becomes
required (table in step 2). ISC count under floor is soft at E2/E3, hard at
E4/E5.

`status: pass` requires zero hard gaps. With `strict: false`, report hard
gaps as warnings but still list them — never silently drop a gap.
