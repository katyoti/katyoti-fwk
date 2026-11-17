---
name: skill-import
description: "Turn an existing skill or knowledge source into contract-valid ktw skills, then hand off to the creation chain. Load when asked to import, port, or convert an external skill or knowledge doc."
advertised: false
hashtags: [skill, import, conversion, authoring, create-items]
category: guide
---

# Importing a skill

You convert what already exists instead of authoring from scratch. Two source
shapes, one rule: the OUTPUT is always a contract-valid ktw skill — the source
format never leaks through.

## 1. Identify the source shape

- **Anthropic/Claude SKILL.md format** — a `SKILL.md` (often with
  `references/` and `scripts/` folders) whose frontmatter carries keys like
  `paths:`, `allowed-tools:`, `argument-hint:`. → §2.
- **Plain knowledge** — docs, wiki pages, notes, cookbooks with no skill
  framing. → §3.

Either way, run the **extend-before-create check first**
(`ktw-compose:/skill/create-artifact` §1): if a live skill already owns the
source's trigger, the import becomes an EXTENSION of that skill, not a new
sibling.

## 2. Anthropic-format conversion

Apply the table — every divergence between the formats:

| Source element | ktw conversion |
|---|---|
| `name`, `description` | keep; REWRITE description to what+when (+ anti-triggers). Source style "This skill should be used when…" becomes a direct what + "Load when…" |
| `paths:` (glob triggers) | DROP — ktw selection is description-driven, not path-driven. Fold any signal the globs carried into the description's when-clause |
| `allowed-tools:`, `argument-hint:`, any other foreign key | DROP — the strict 9-key contract rejects them; the run-host owns tool permissions |
| `references/<file>` + relative links | files → the sidecar's `embed/references/<topic>/…`; every link becomes a bare URI `<provider>:/reference/<path>` in a table or prose — NEVER a relative link |
| `scripts/` + "run this script" | ktw skills have no scripts key — promote the logic to a slot method (code track) or drop; say so in the body |
| surface-specific text ("use the Read tool", "run with Bash", CLI command lines as the loading mechanism) | name the target by bare URI and stop — the host preamble says how to load (see `ktw-compose:/skill/skill-format` cross-reference rules) |
| provenance/meta text ("sourced from project X", sync notes, version framing) | DROP — a skill is timeless knowhow, not a changelog |
| "you can also … / or alternatively …" | ONE method per outcome — pick the canonical one, cut the rest |
| product-specific integrations (foreign commands, e.g. `/vendor:verb` tables) | DROP the column/section; keep the knowledge it decorated |

Then decide the shape per the standard rules: single leaf, or advertised top +
`advertised:false` sub-skills (a big source skill with many reference files
often becomes ONE sub-skill per knowledge cluster plus references for the
lookup tables — `ktw-compose:/skill/skill-layout-planning` plans the
inventory when the source yields more than a couple of items).

## 3. Plain-knowledge import

The source has no skill framing — create it:

1. Split the material: **procedures with decisions** → skill bodies;
   **passive lookup material** (tables, schemas, catalogs, cheat-sheets) →
   reference files. The test: knowhow you DECIDE to consult → skill; a value
   you LOOK UP → reference.
2. Load `ktw-compose:/skill/skill-layout-planning` to inventory every item
   (level, trigger, placement) before writing anything.
3. Author each item through `ktw-compose:/skill/create-skill` (which loads
   the format contract). The source text is raw material — rewrite, never
   paste: second-person imperative, URIs for cross-references, no
   source-document voice.

## 4. Place, write, validate

- **Home**: the sidecar that owns the capability (`app/<sidecar>/embed/…`,
  rebuild needed) or the workspace skills folder (runtime-loaded). Ask when
  unclear — never default silently. When the owning sidecar's source is in this
  workspace, iterate the imported skill via the overlay (no rebuild) and promote
  when validated — `ktw-compose:/skill/overlay-iteration`.
- **First references in a sidecar**: if the target never shipped references
  before, add `embed/references` to the `//go:embed` directive or the URIs
  silently fail to resolve (the embedded pack publishes the folder). Verify
  with a `resources read` on each new URI after rebuild — "not found" means
  the embed directive is missing the folder.
- Write per `ktw-compose:/skill/skill-format`; sub-skills `advertised:false`.
- Validate: strict frontmatter parses (the corpus gate), the skill lists or
  loads by URI, and EVERY reference URI the bodies name resolves via
  `resources read` — an imported skill whose references are dead is worse
  than no import.
- Leave the source untouched (staging areas are the user's); report which
  source elements were dropped and why.
