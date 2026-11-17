---
name: mermaid-syntax
description: >-
  Author correct Mermaid diagram source — pick the right diagram type among the
  23 supported, load its exact syntax, apply theming and avoid the known parser
  gotchas. Load when writing or fixing Mermaid source (flowchart, sequence,
  class, state, ER, Gantt, C4, mindmap, timeline, …) or when a Mermaid diagram
  fails to render.
hashtags: [diagram, mermaid, syntax]
category: guide
advertised: false
---

# Mermaid syntax

You are writing or repairing Mermaid diagram source. The per-type syntax lives
in references — load only the one you need. The same source serves both
delivery forms: a **standalone file** is rendered by calling the **diagram**
tool with `{"type": "mermaid", "source": "<diagram source>", "format": "svg"}`;
an **embedded block** is a fenced ```` ```mermaid ```` block written inside a
markdown document and rendered when the document is rendered — never call the
diagram tool for it.

## 1. Pick the diagram type

Choose the type that matches what must be shown, then read its syntax
reference before writing any source:

| Type | Reference |
|---|---|
| Flowchart | `ktw-docs:/reference/diagrams/mermaid/flowchart.md` |
| Sequence | `ktw-docs:/reference/diagrams/mermaid/sequenceDiagram.md` |
| Class | `ktw-docs:/reference/diagrams/mermaid/classDiagram.md` |
| State | `ktw-docs:/reference/diagrams/mermaid/stateDiagram.md` |
| Entity-Relationship | `ktw-docs:/reference/diagrams/mermaid/entityRelationshipDiagram.md` |
| Gantt | `ktw-docs:/reference/diagrams/mermaid/gantt.md` |
| Pie | `ktw-docs:/reference/diagrams/mermaid/pie.md` |
| Mindmap | `ktw-docs:/reference/diagrams/mermaid/mindmap.md` |
| Timeline | `ktw-docs:/reference/diagrams/mermaid/timeline.md` |
| Git graph | `ktw-docs:/reference/diagrams/mermaid/gitgraph.md` |
| Quadrant chart | `ktw-docs:/reference/diagrams/mermaid/quadrantChart.md` |
| Requirement | `ktw-docs:/reference/diagrams/mermaid/requirementDiagram.md` |
| C4 | `ktw-docs:/reference/diagrams/mermaid/c4.md` |
| Sankey | `ktw-docs:/reference/diagrams/mermaid/sankey.md` |
| XY chart | `ktw-docs:/reference/diagrams/mermaid/xyChart.md` |
| Block | `ktw-docs:/reference/diagrams/mermaid/block.md` |
| Packet | `ktw-docs:/reference/diagrams/mermaid/packet.md` |
| Kanban | `ktw-docs:/reference/diagrams/mermaid/kanban.md` |
| Architecture | `ktw-docs:/reference/diagrams/mermaid/architecture.md` |
| Radar | `ktw-docs:/reference/diagrams/mermaid/radar.md` |
| Treemap | `ktw-docs:/reference/diagrams/mermaid/treemap.md` |
| User journey | `ktw-docs:/reference/diagrams/mermaid/userJourney.md` |
| ZenUML | `ktw-docs:/reference/diagrams/mermaid/zenuml.md` |

If no type fits cleanly, prefer Flowchart — it is the most general.

## 2. Configuration and theming

When the diagram needs styling, layout control, or math rendering, read the
matching reference:

| Topic | Reference |
|---|---|
| Theming | `ktw-docs:/reference/diagrams/mermaid/config-theming.md` |
| Directives | `ktw-docs:/reference/diagrams/mermaid/config-directives.md` |
| Layouts | `ktw-docs:/reference/diagrams/mermaid/config-layouts.md` |
| Configuration | `ktw-docs:/reference/diagrams/mermaid/config-configuration.md` |
| Math | `ktw-docs:/reference/diagrams/mermaid/config-math.md` |
| Tidy tree | `ktw-docs:/reference/diagrams/mermaid/config-tidy-tree.md` |
| Worked examples | `ktw-docs:/reference/diagrams/mermaid/examples.md` |

For C4 diagrams, also read
`ktw-docs:/reference/diagrams/mermaid/c4-layout-science.md` — declaration
ordering, edge-crossing targets, and colour standards that make C4 output
readable instead of tangled.

## 3. Avoid the known parser gotchas

These break rendering most often — check your source against them before
rendering:

| Gotcha | Problem | Fix |
|---|---|---|
| `<br/>` in flowchart edge labels | parser rejects HTML in edge labels | comma-separated text: `-->\|"Uses, HTTPS"\|` |
| `end` as node ID | reserved keyword | different ID: `EndNode["End"]` |
| Gantt date formats | strict date format | `YYYY-MM-DD` |
| Gantt task status | invalid status keywords | only `done`, `active`, `crit`, `milestone` |
| Parentheses in labels | unescaped `()` breaks node parsing | quote the label: `Node["Label (with parens)"]` |
| Special chars in IDs | hyphens, dots, spaces break IDs | camelCase or underscores: `apiGateway` |
| ER attribute syntax | attributes need the exact pattern | follow `entity { type name }` |
| Subgraph names with spaces | unquoted spaces fail | `subgraph "My Group"` |

## 4. Validate

Check the source renders: for a standalone file that IS the delivery; for an
embedded block, one **diagram** tool call is a cheap syntax check before the
block goes into the document. If rendering fails, re-read the type reference
for the failing construct — do not guess syntax variants. For embedded blocks
also check `ktw-docs:/reference/diagrams/diagram-pitfalls.md` (Kroki + Pandoc
+ Typst traps). If Mermaid itself turns out to be the wrong engine for the
subject, go back to `ktw-docs:/skill/diagram-create` and re-select.
