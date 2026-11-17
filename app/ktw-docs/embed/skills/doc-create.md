---
name: doc-create
description: >-
  Universal document and diagram creation and conversion skill. Analyzes user
  intent, proposes matching document type(s) via interactive selection, then
  dispatches to specialized guides. Supports: design documents (C4), PRDs, LOE
  estimates, summaries, outlines, tags, rich markdown with diagrams (28+ types
  via Kroki), PDFs (Pandoc + Typst), Word documents, slide files
  (PDF, PPTX — a presentation to show is a deck, skill deck-author), standalone diagrams with dedicated Mermaid and
  PlantUML syntax sub-skills, concept maps (Vis.js), mind maps (Markmap), and
  format conversion (50+ formats via Pandoc). Use when the user wants to
  create, generate, author, write, produce, or convert any kind of document,
  diagram, or visual artifact.
provides: [task]
requires:
  - docs
---

# Document Creation

**Input:** a topic, description, file path, or document type.

Orchestrate document creation by understanding the user's intent, selecting the right document type, and dispatching to specialized reference guides.

## Tools Available

All rendering is performed server-side by the ktw-docs sidecar through two agent tools. The reference guides describe *what* to produce; the tools produce *how*.

### `render`

Converts a Markdown file to a formatted output. Supported formats:

- **pdf** — Professional PDFs via Pandoc + Typst
- **docx** — Word documents (use `reference_doc` for corporate styling)
- **pptx** — PowerPoint presentations (use `reference_doc` for slide templates)
- **html** — Standalone HTML pages

Kroki diagram rendering for fenced code blocks (mermaid, plantuml, graphviz, d2, …) happens inside the tool — no filter flag required.

Parameters:
- `input_path` (required) — path to the Markdown source file
- `output_path` (required) — path for the rendered output
- `output_format` (required) — one of: `pdf`, `docx`, `pptx`, `html`
- `template` (optional) — template name or path for custom styling
- `reference_doc` (optional) — reference document for DOCX/PPTX styling

### `diagram`

Renders a single diagram from inline source to an image. Supports 28+ diagram types.

Parameters:
- `type` (required) — diagram type: `mermaid`, `plantuml`, `graphviz`, `d2`, `c4`, `erd`, `dbml`, `vegalite`, `wavedrom`, `excalidraw`, `ditaa`, `nomnoml`, `svgbob`, `umlet`, `pikchr`, `bytefield`, `bpmn`, `blockdiag`, `seqdiag`, `actdiag`, `nwdiag`, `packetdiag`, `rackdiag`, `structurizr`, `tikz`, `wireviz`, `vega`
- `source` (required) — diagram source code (inline string, not a file path)
- `format` (optional, default `svg`) — output format: `svg`, `png`, `jpg`, `pdf`

## Process

### Step 1 — Understand Intent

Analyze `$ARGUMENTS` and the user's request. Classify into one or more document categories:

| Category | Doc Types | Trigger keywords |
|----------|-----------|------------------|
| Content Documents | design-document, prd, loe-document, summary, recursive-outline, tags | architecture, design doc, requirements, PRD, estimate, LOE, summarize, outline, breakdown, keywords, tags |
| Rich Documents | rich-markdown, pdf, docx, slides | PDF, report, Word, DOCX, presentation, PowerPoint, PPTX, slides, markdown with diagrams, Confluence page |
| Visual Artifacts | diagram, conceptmap, markmap | diagram, flowchart, sequence diagram, UML, concept map, knowledge graph, mind map |
| Conversion | convert-to-markdown, convert-document | convert, transform, export, change format |

### Step 2 — Select Document Types

Present the proposed document type(s) to the user via `AskUserQuestion` (multiSelect), grouped by category.

Pre-select types that match the detected intent. If intent is unambiguous (user explicitly said "make a PRD"), skip selection and confirm directly.

Available types with descriptions:

| Type | Description |
|------|-------------|
| design-document | C4 architecture, security posture, risk assessment |
| prd | Product requirements, user stories, functional specs |
| loe-document | Effort estimation, resource planning, cost breakdown |
| summary | Structured summary with key points and takeaways |
| recursive-outline | Hierarchical task/concept decomposition |
| tags | Keyword extraction for categorization |
| convert-to-markdown | Clean markdown from any content format |
| rich-markdown | Markdown with embedded Kroki diagrams (28+ types) |
| pdf | Publication-quality PDF via Pandoc + Typst |
| docx | Professional Word document |
| slides | A slide FILE (PDF or PPTX) for PowerPoint; a presentation to show or present is a deck — load `ktw-docs:/skill/deck-author` instead |
| convert-document | Universal format conversion via Pandoc (50+ formats) |
| conceptmap | Interactive HTML concept map (Vis.js) |
| markmap | Mind-map visualization (Markmap) |
| diagram | Standalone diagram image (28+ types via Kroki) |

### Step 3 — Dispatch to References

Each document type has a dedicated guide in the provider references directory:

- `design-document.md` — C4 architecture, security posture, risk assessment
- `loe-document.md` — Effort estimation, resource planning, cost breakdown
- `prd.md` — Product requirements, user stories, functional specs
- `summary.md` — Structured summary with key points and takeaways
- `recursive-outline.md` — Hierarchical task/concept decomposition
- `tags.md` — Keyword extraction for categorization
- `convert-to-markdown.md` — Clean markdown from any content format
- `rich-markdown.md` — Markdown with embedded Kroki diagrams
- the publishing sidecar's markdown reference — Markdown intended for Confluence publication (CSF-supported subset, panels, macros, diagram rendering modes); supplements rich-markdown when the target is Confluence (lives in that sidecar)
- `pdf.md` — Publication-quality PDF via Pandoc + Typst
- `docx.md` — Professional Word document
- `slides.md` — Slide files (PDF or PPTX) via Pandoc; a presentation to show is a deck (`ktw-docs:/skill/deck-author`)
- `convert-document.md` — Universal format conversion via Pandoc
- `conceptmap.md` — Interactive HTML concept map (Vis.js)
- `markmap.md` — Mind-map visualization
- `diagram.md` — Standalone diagram image (dispatches to `ktw-docs:/reference/diagrams/<family>.md`)

**Execution strategy**: Launch selected types as parallel agents. Each agent:
1. Reads its guide (the `ktw-docs:/reference/<type>.md` resource named above)
2. For diagrams: also reads the relevant `ktw-docs:/reference/diagrams/<family>.md` and `ktw-docs:/reference/diagrams/diagram-pitfalls.md`
3. For pdf/docx/slides: reads `ktw-docs:/reference/rich-markdown.md` for authoring phase, then `ktw-docs:/reference/convert-document.md` for compilation
4. Produces its output (files and/or inline content)

**Diagram creation**: whenever a diagram must be created or chosen (standalone
`diagram` type, or fenced diagram blocks inside rich-markdown/slides), load
`ktw-docs:/skill/diagram-create` and follow it — it clarifies what must be
shown, picks the diagram type and engine, then routes to the engine's syntax
skill (`ktw-docs:/skill/mermaid-syntax` or `ktw-docs:/skill/plantuml-syntax`,
each backed by per-type references under
`ktw-docs:/reference/diagrams/{mermaid,plantuml}/`) and applies the
design-quality gates.

### Step 4 — Present Results

After all agents complete:
1. List all generated files with descriptions.
2. For single-output cases, show a preview.
3. Suggest follow-up actions (e.g., "Convert this markdown to PDF using the convert-document reference").

## Rules

- Always let the user select document types before dispatching (unless intent is unambiguous).
- Each reference guide is self-contained — do not mix instructions from different references.
- For multi-type requests, produce separate outputs per type.
- Report generated file paths to the user.
