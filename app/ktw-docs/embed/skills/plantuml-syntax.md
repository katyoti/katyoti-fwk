---
name: plantuml-syntax
description: >-
  Author correct PlantUML diagram source with a focus on C4-PlantUML for
  architecture diagrams — element and boundary syntax, directional
  relationships, layout-conflict rules, skinparams and themes. Load when
  writing or fixing PlantUML source (C4, sequence, class, activity, state, ER,
  component, deployment, use case) or when a PlantUML diagram renders wrong or
  not at all.
hashtags: [diagram, plantuml, c4, syntax]
category: guide
advertised: false
---

# PlantUML syntax

You are writing or repairing PlantUML diagram source. The per-type syntax lives
in references — load only the one you need. The same source serves both
delivery forms: a **standalone file** is rendered by calling the **diagram**
tool with `{"type": "plantuml", "source": "<diagram source>", "format": "svg"}`
(use `"type": "c4"` for C4-PlantUML when the renderer distinguishes them); an
**embedded block** is a fenced ```` ```plantuml ```` (or ```` ```c4plantuml ````)
block written inside a markdown document and rendered when the document is
rendered — never call the diagram tool for it.

## 1. Pick the diagram type

Choose the type that matches what must be shown, then read its syntax
reference before writing any source:

| Type | Reference |
|---|---|
| C4-PlantUML (architecture) | `ktw-docs:/reference/diagrams/plantuml/c4-plantuml.md` |
| Sequence | `ktw-docs:/reference/diagrams/plantuml/sequence-diagrams.md` |
| Class | `ktw-docs:/reference/diagrams/plantuml/class-diagrams.md` |
| Activity | `ktw-docs:/reference/diagrams/plantuml/activity-diagrams.md` |
| State | `ktw-docs:/reference/diagrams/plantuml/state-diagrams.md` |
| Entity-Relationship | `ktw-docs:/reference/diagrams/plantuml/er-diagrams.md` |
| Component | `ktw-docs:/reference/diagrams/plantuml/component-diagrams.md` |
| Use case | `ktw-docs:/reference/diagrams/plantuml/use-case-diagrams.md` |
| Deployment | `ktw-docs:/reference/diagrams/plantuml/deployment-diagrams.md` |

## 2. C4-PlantUML — the primary case

For architecture diagrams, read
`ktw-docs:/reference/diagrams/plantuml/c4-plantuml.md` first. It carries what
C4 output quality depends on:

- include URLs for the C4 libraries
- element syntax (`Person`, `System`, `Container`, `Component` and variants)
- boundary syntax (`System_Boundary`, `Container_Boundary`)
- **directional relationships** (`Rel_Down`, `Rel_Right`, `Rel_Up`, `Rel_Left`,
  `Rel_Neighbor`) — always prefer these over plain `Rel`
- **layout constraints** (`Lay_Right`, `Lay_Down`, `Lay_Distance`)
- the **layout-conflict rules** — contradictory direction hints on the same
  element pair are the #1 cause of broken C4 renders

For layout science shared across engines (Sugiyama algorithm, edge-crossing
targets, declaration ordering), read
`ktw-docs:/reference/diagrams/mermaid/c4-layout-science.md` — its PlantUML
section covers directional hints.

## 3. Styling and errors

| Topic | Reference |
|---|---|
| Common syntax errors | `ktw-docs:/reference/diagrams/plantuml/common-syntax-errors.md` |
| Styling guide (skinparams, themes) | `ktw-docs:/reference/diagrams/plantuml/styling-guide.md` |

## 4. Avoid the known gotchas

Check your source against these before rendering:

| Gotcha | Problem | Fix |
|---|---|---|
| `Rel_Down` contradicts `Lay_Right` | conflicting direction hints on one pair | make every `Rel_*` consistent with any `Lay_*` on the same pair |
| Missing `@startuml`/`@enduml` | nothing renders | always wrap the source |
| Wrong `!include` URL | C4 macros not found | exact plantuml-stdlib URL, e.g. `…/C4-PlantUML/master/C4_Context.puml` |
| Plain `Rel` | random element placement | directional `Rel_Down`, `Rel_Right`, … |
| Relationship before declaration | undeclared element ID | declare ALL elements before ANY relationships |
| Spaces in element IDs | parser failure | camelCase or underscores: `paymentApi` |
| Empty boundaries | rendering errors | every boundary contains at least one element |
| `\n` in descriptions | rendered as a line break | that is native behavior; write `\\n` for literal text |

## 5. Validate

Check the source renders: for a standalone file that IS the delivery; for an
embedded block, one **diagram** tool call is a cheap syntax check before the
block goes into the document. If rendering fails, re-read the type reference
for the failing construct — do not guess syntax variants. For embedded blocks
also check `ktw-docs:/reference/diagrams/diagram-pitfalls.md` (Kroki + Pandoc
+ Typst traps). If PlantUML itself turns out to be the wrong engine for the
subject, go back to `ktw-docs:/skill/diagram-create` and re-select.
