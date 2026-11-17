# PDF Document

Reference for generating publication-quality PDF documents from markdown using Pandoc with Typst as the PDF engine and Kroki for diagram rendering.

---

## Purpose

Create publication-quality PDFs by orchestrating markdown authoring and Pandoc/Typst compilation. Supports rich content including diagrams (Mermaid, PlantUML, GraphViz, D2, Vega-Lite, and 28+ types), math equations, tables, citations, custom fonts, and page layouts.

## Scope

Input: a markdown file path, or a topic/description to create from scratch.

## Process

### Pipeline Overview

1. Phase 1 -- Author Markdown with PDF-specific rules (read the `rich-markdown.md` reference).
2. Phase 2 -- Compile to PDF (read the `convert-document.md` reference).

### Phase 1 -- Author the Markdown

If the input is an existing `.md` file with proper PDF frontmatter (has `mainfont` and `papersize`), skip to Phase 2.

Otherwise, author the content following the `rich-markdown.md` reference, applying PDF-specific rules below.

#### Required YAML Frontmatter

```yaml
---
title: "Document Title"
author: "Author Name"
date: "YYYY-MM-DD"
mainfont: "Segoe UI"
papersize: a4
fontsize: 11pt
margin:
  top: 2.5cm
  bottom: 2.5cm
  left: 2.5cm
  right: 2.5cm
toc: true
toc-depth: 3
section-numbering: "1.1.1"
---
```

`mainfont` is required -- Typst 0.14+ rejects empty font lists. Safe cross-platform choices: "Segoe UI" (Windows), "Helvetica Neue" (macOS), "DejaVu Sans" (Linux), "Arial" (universal).

#### PDF Authoring Rules

- Follow the `rich-markdown.md` reference PDF-Specific Guidelines section.
- Read `diagrams/diagram-pitfalls.md` before creating diagrams.
- Mermaid keywords are case-sensitive: `gitGraph` (not `gitgraph`), `stateDiagram-v2`.
- No `note` directives in `stateDiagram-v2`.
- Keep diagrams reasonably sized -- prefer horizontal layouts (`direction: right` in D2, `graph LR` in Mermaid).
- Raw Typst blocks can be embedded: ` ```{=typst} ... ``` `

### Phase 2 -- Compile to PDF

Read the `convert-document.md` reference for the exact compilation commands and Kroki filter usage.

#### Template Selection

Select a template based on document type:

| Document Type | Template |
|---|---|
| Report | `report.typ` |
| Article / Paper | `article.typ` |
| Letter | `letter.typ` |
| Invoice | `invoice.typ` |
| Default | omit `--template` |

Templates are located at `assets/templates/` relative to the skill root. See the `convert-document.md` reference PDF Output section for exact commands.

## Constraints

- `mainfont` is mandatory in frontmatter -- Typst 0.14+ rejects empty font lists.
- `papersize` must be specified (typically `a4`).
- Mermaid keywords are case-sensitive (`gitGraph`, not `gitgraph`; `stateDiagram-v2`).
- No `note` directives in `stateDiagram-v2`.
- Diagrams should use horizontal layouts where possible.
- Requires pandoc (>=3.1.2) and typst to be installed locally.
