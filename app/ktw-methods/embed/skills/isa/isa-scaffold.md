---
name: isa-scaffold
description: >-
  Generate a fresh ISA at a given articulation depth (E1–E5): required sections per
  depth, atomic ISCs (one binary probe each), Anti/Antecedent criteria, ending
  with a completeness pass. Workflow of the isa umbrella skill.
iconName: file-plus
icon: "🏗️"
hashtags: [isa, scaffold, articulation]
category: methodology
advertised: false
---

# ISA — Scaffold

Produce a fresh ISA from a prompt (or from an interview result) at a
specified articulation depth. The output is a populated ISA with all sections the
depth requires — shaped per `ktw-methods:/reference/isa/format-spec.md`.

## Inputs

| Input | Required | Description |
|---|---|---|
| prompt | yes | The request — verbatim or distilled (an interview summary is ideal) |
| depth | yes | E1–E5; if unspecified, judge from scale and say which you picked |
| target | yes | `isa-file` (+ the file path) or `cwm-items` (+ the node ID) |

## Procedure

### 1 — Read the contract and an example

Read `ktw-methods:/reference/isa/format-spec.md`, then the example closest
to the depth (`ktw-methods:/reference/isa/examples/e1-minimal.md` for small,
`.../e3-project.md` for mid-size). Copy section headers from the example.

### 2 — Reverse-engineer the prompt

Distill into:
- explicit wants → Goal + Criteria material
- explicit not-wants → Out of Scope
- implied not-wants (domain inference) → Out of Scope
- domain-implied mandates → Constraints
- stated truths about how to think → Principles

If the prompt is too thin to fill the depth's required sections, stop and run
`ktw-methods:/skill/isa-interview` first — do not pad sections with fluff.

### 3 — Write required sections per depth

| Tier | Required sections |
|---|---|
| E1 | Goal, Criteria |
| E2 | Problem, Goal, Criteria, Test Strategy |
| E3 | Problem, Vision, Out of Scope, Constraints, Goal, Criteria, Features, Test Strategy |
| E4 | all twelve |
| E5 | all twelve + an interview pass before any build starts |

Sections never appear empty: write only what is populated; the gate decides
what MUST be populated. Order is fixed (see the format spec).

### 4 — Apply the splitting test to every ISC

Each ISC is **one binary tool probe**. Split a criterion when it:
- joins two verifiable things with "and"/"with"/"including"
- has a part A that can pass while part B fails
- uses "all"/"every"/"complete" → enumerate what that means
- crosses a domain boundary (UI/API/data/logic → one per boundary)
- has **no nameable probe** — the deepest signal: it is still fluff

Name the probe inline: `(probe: <command or check that returns yes/no>)`.

### 5 — Anti-criteria

Ask: **what must NOT happen?** Derive ≥1 `Anti:` ISC from Out of Scope +
regression concerns. A goal with zero failure modes worth naming is
under-specified — this is a hard gate at every depth.

### 6 — Antecedent (experiential goals only)

If the goal must "land" (art, design, content, UX), add ≥1 `Antecedent:`
ISC naming a precondition that reliably produces the target experience.

### 7 — Materialize per target

- `isa-file`: write the ISA at the given path with the frontmatter from the
  format spec.
- `cwm-items`: create items on the given node per the format spec's CWM
  mapping table (Goal → `goal`, ISCs → `acceptance_criteria[]` inline on
  `use-case` items, Feature → `deliverable`, Test Strategy row →
  `test-case`); put the remaining prose sections in the node body, same
  section order.

### 8 — Completeness pass

Run `ktw-methods:/skill/isa-check-completeness` against the result at the
requested depth. Fill any hard gap before declaring the scaffold done, then
return the path (or node ID) to the caller.

## Failure modes

- **Tier mismatch** — the request is clearly E4 work but E1 was asked:
  surface the mismatch and let the caller decide; do not silently upgrade.
- **ISC count under the depth floor** (E2 ≥16, E3 ≥32 — soft; E4 ≥128,
  E5 ≥256 — hard): keep splitting, or record the justified under-count in
  `## Decisions`.
