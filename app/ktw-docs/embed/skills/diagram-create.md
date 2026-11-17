---
name: diagram-create
description: >-
  Go from an intention ("show how X works") to the right diagram of the right
  type — clarify what must be shown, pick the diagram type and engine, route to
  the engine's syntax skill, and apply the design-quality gates. Load whenever
  a diagram must be created or chosen, standalone or embedded in a document.
hashtags: [diagram, selection, design]
category: guide
advertised: false
---

# Creating the right diagram

You own the path from intention to a rendered diagram of the right type and a
clean design. Do not write diagram source before steps 1–2 are answered.

A diagram is delivered in one of three forms — decide which one early:

- **Standalone diagram file** — an image (`svg`/`png`/`jpg`/`pdf`) produced by
  calling the **diagram** tool with the source. The user gets a file.
- **Embedded diagram block** — a fenced code block (e.g. ```` ```mermaid ````)
  written INSIDE a markdown document; it is rendered server-side (Kroki) when
  the document itself is rendered to PDF/DOCX/HTML/slides. The user gets a
  document containing the diagram; you never call the diagram tool for it.
- **Editable drawio diagram** — a `.drawio` file the user opens, edits and
  publishes to Confluence, for an architecture view (ArchiMate, C4, BPMN,
  AWS, UML) or any diagram with clickable links. You write no drawio XML and no
  engine syntax: you describe a **LogicalDiagram** (typed nodes, edges, zones
  and a viewpoint URI) and the drawio engine lays it out and draws it —
  `ktw-docs drawio render <logical.json> -o <file>.drawio`, or the
  `diagram-emit` agent tool. Steps 3 and 5 below do not apply; the shapes
  are the viewpoint's templates (`ktw-docs:/reference/drawio-authoring.md`).

## 1. Clarify what must be shown

Answer these (ask the user only for the ones the request leaves open):

- **Subject** — what is being shown? A process/flow, an interaction over time,
  a structure (classes, entities, components), a system architecture, data/
  metrics, a network/infrastructure, a plan/timeline, or a concept map?
- **Audience + context** — technical readers or stakeholders? Standalone image
  or embedded in a document/slide deck?
- **Scale** — how many elements? A diagram past ~20 nodes needs decomposition
  into multiple views, not a bigger canvas.

## 2. Pick the type and the engine

Map the subject to a diagram type. The compact routing:

| Subject | First choice | Engine |
|---|---|---|
| Process / decision flow | flowchart | mermaid |
| Interaction over time | sequence diagram | mermaid (simple) / plantuml (complex) |
| System architecture | C4 (context/container/component) | plantuml (C4-PlantUML) |
| Class / type structure | class diagram | plantuml |
| Entity-relationship / schema | ER diagram | mermaid (simple) / dbml, erd (schema-heavy) |
| State machine | state diagram | mermaid |
| Plan / schedule | Gantt | mermaid |
| Metrics / statistics | bar/line/scatter chart | vegalite |
| Network / racks / packets | nwdiag family | nwdiag, rackdiag, packetdiag |
| Business process (formal) | BPMN | bpmn |
| Concepts / hierarchy | mindmap, treemap | mermaid |
| Enterprise architecture to edit and publish | ArchiMate / UML / flux view | drawio engine (LogicalDiagram) |

For the full need→type catalog (28+ types with alternatives), read
`ktw-docs:/reference/diagrams/diagram-types.md`. When two types fit, present
the trade-off to the user rather than picking silently.

**Engine rule of thumb**: mermaid for most day-to-day diagrams (renders
everywhere, 23 types); plantuml when UML rigor or C4 architecture is the
point; the specialist engines (vegalite, nwdiag, bpmn, dbml, …) when the
subject is exactly their domain.

## 3. Route to the engine's syntax

- **Mermaid** source — load `ktw-docs:/skill/mermaid-syntax` and follow it
  (per-type syntax references, theming, parser gotchas).
- **PlantUML / C4** source — load `ktw-docs:/skill/plantuml-syntax` and follow
  it (C4 element/boundary syntax, layout-conflict rules, gotchas).
- **Other engines** — read the family reference and write from it:
  GraphViz / D2 → `ktw-docs:/reference/diagrams/flowcharts.md`;
  Vega / Vega-Lite → `ktw-docs:/reference/diagrams/data-visualization.md`;
  BlockDiag family → `ktw-docs:/reference/diagrams/network-infra.md`;
  Nomnoml → `ktw-docs:/reference/diagrams/uml.md`;
  Structurizr → `ktw-docs:/reference/diagrams/architecture.md`;
  everything else (BPMN, Excalidraw, Ditaa, DBML, WaveDrom, …) →
  `ktw-docs:/reference/diagrams/misc-diagrams.md`.

## 4. Design-quality gates

Before finalizing any diagram:

- Read `ktw-docs:/reference/diagrams/diagram-pitfalls.md` — Kroki rendering
  and PDF-embedding traps (case-sensitive keywords, sizing, fonts).
- Layout: prefer a reading direction (top-down for processes, left-right for
  pipelines); group related elements; for C4 follow the layout-science rules
  the engine skill names.
- Labels: every edge that isn't obvious gets a short verb label; no
  unexplained acronyms for stakeholder audiences.
- One message per diagram — if it explains two things, split it.

## 5. Deliver

**Standalone file**: render with the **diagram** tool
(`{"type": "<engine>", "source": "<source>", "format": "svg"}`), show the
result, iterate on user feedback. The full standalone workflow (input modes,
the optional `.diagram.txt` authoring file, refinement loop) is
`ktw-docs:/reference/diagram.md`.

**Embedded block**: write the fenced code block with the engine's language
tag into the document and hand it back to the calling document flow
(rich-markdown / slides / pdf) — document rendering does the Kroki pass. Do
not render it standalone. If the document is destined for **Confluence**,
diagram blocks render per the configured mode (code / plugin macro / Kroki
image attachment) — follow the publishing sidecar's own markdown reference. The embedded case has extra traps
(Kroki + Pandoc + Typst interplay) — that is exactly what
`ktw-docs:/reference/diagrams/diagram-pitfalls.md` covers; you may still use
the **diagram** tool once as a pure syntax check before embedding.
