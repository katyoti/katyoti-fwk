---
name: asset-intake
description: "Triage external assets against the live katyoti catalog, reaching an add/enhance/included/superseded verdict for each. Load to decide what external material is worth importing before importing it."
iconName: inbox-arrow-down
icon: "📥"
hashtags: [asset-intake, triage, value-fit, import, compose]
category: development
---

# Asset intake — what's worth adding to katyoti?

You are handed a pile of "nice things" — skills, tasks, documents,
explanations — and asked which bring value to the katyoti solution. You
triage them against what already exists and route only what's additive.

The judgment is **`ktw-methods:/skill/value-fit`** — load it and apply
its four-verdict doctrine
(add / enhance / included / superseded, each with evidence). This skill adds
the katyoti specifics: where the catalog is, and where each verdict routes.

## 1 — Inventory the live catalog (the comparison surface)

value-fit's overlap scan needs the current surface. List it:

- **`skills list`** — every advertised skill across the universe.
- **`tasks list`** — contributed tasks.
- **`ktw-univers scan-inventory`** — the sidecar binaries.
- **the universe-wide contribution listing** — references, data-types, configs.

This is the same surface the create-artifact reads before extend-vs-create —
asset-intake just triages a BATCH against it.

## 2 — Triage each candidate (value-fit)

Per candidate, reach one verdict with evidence per
`ktw-methods:/reference/value-fit/taxonomy.md`: the verdict, the existing URI
it `compared_against` (or "none"), the `probe` (testable value for add /
the missing case for enhance). No verdict without a comparison.

If the candidate is a **foreign-format skill** (Anthropic `SKILL.md`) or
**plain-text knowledge**, you still triage it AS-IS first — decide add /
enhance / included / superseded against the catalog. Only AFTER an add or
enhance verdict does it pass through `ktw-compose:/skill/skill-import` to
become a ktw-shaped artifact. Triage before import — never import what's
already INCLUDED.

## 3 — Route (one route per verdict)

| Verdict | Route |
|---|---|
| **add** | becomes a `deliverable` on a **`composition`** object → the **compose-artifact wizard** (`ktw-compose:tasks/compose-artifact`) builds it. For a single asset, route straight to `ktw-compose:/skill/create-artifact`. |
| **enhance** | extend the named existing artifact with the delta ONLY — plan the placement via `ktw-compose:/skill/skill-layout-planning` (a sub-skill / widened section / appended reference). |
| **included** | discard; record the covering URI so it is not re-triaged. |
| **superseded** | discard after harvesting the one real nuance into the existing artifact. |

A candidate worth having but not now is an **idea**: file it as a feature in
state `idea` under the module it concerns (`cwm-object-create` with
`type: feature`; a goal via `ktw-methods:goals/add` only when it is a direction
no test could ever close), with the asset as its evidence — never as a loose
note.

## 4 — The triage view (the record)

Produce a **triage board** — one row per candidate: `candidate | verdict |
compared_against | probe/delta | route`. For a quick run this is a markdown
report. For a GUI-driven intake, materialize it in CWM so the user approves
per-row:

- a **`triage`** object holds the batch;
- one **`triage-candidate`** object under it per asset (verdict /
  compared_against / probe / delta / route / status), authored via the cwm-* tools;
- the schema-driven board renders the rows; the user approves or overrides;
- **add**-approved candidates flow to a composition as `deliverable` objects —
  the wizard builds them, the acceptance criteria verify them.

This closes the loop: a pile of nice things → triaged with evidence →
realized, tested artifacts (or a clear record of why each was discarded).

## Anti-patterns

- **Importing before triaging** — running skill-import on a candidate that's
  already INCLUDED. Triage first.
- **ADD-ing a duplicate** because it reads well (value-fit's add-the-duplicate
  guard). Quality is not additivity.
- **A verdict with no `compared_against`** — rescan the catalog; you judged
  blind.
