# DOCX Document

Reference for generating professional Word documents (.docx) from markdown using Pandoc with Kroki diagram rendering.

---

## Purpose

Create professional Word documents by orchestrating markdown authoring and Pandoc compilation. Supports rich content including diagrams (Mermaid, PlantUML, GraphViz, D2, and 28+ types), tables, citations, TOC, custom styling via reference documents, and embedded images.

## Scope

Input: a markdown file path, or a topic/description to create from scratch.

## Process

### Pipeline Overview

1. Phase 1 -- Author Markdown with DOCX-specific rules (read the `rich-markdown.md` reference).
2. Phase 2 -- Compile to DOCX (read the `convert-document.md` reference).

### Phase 1 -- Author the Markdown

If the input is an existing `.md` file with proper frontmatter, skip to Phase 2.

Otherwise, author the content following the `rich-markdown.md` reference, applying DOCX-specific rules below.

#### Recommended YAML Frontmatter

```yaml
---
title: "Document Title"
author: "Author Name"
date: "YYYY-MM-DD"
subtitle: "Subtitle"
abstract: "Brief summary of the document."
toc: true
toc-depth: 3
---
```

#### DOCX Authoring Rules

- Follow the `rich-markdown.md` reference DOCX-Specific Guidelines section.
- Headings map directly to Word heading styles (Heading 1, Heading 2, etc.).
- Tables render as Word tables with borders.
- Images are embedded; footnotes become Word footnotes.
- Diagrams are rendered as PNG and embedded by the Kroki filter.
- Citations with `--citeproc` produce proper bibliography sections.

### Phase 2 -- Compile to DOCX

Read the `convert-document.md` reference for exact compilation commands and Kroki filter usage.

For custom styling, provide a Word reference document (`--reference-doc=reference.docx`). See the `convert-document.md` reference DOCX Output section for details on creating reference documents.

## Constraints

- Requires pandoc to be installed locally.
- Complex Word formatting (columns, text boxes) may simplify when converting from DOCX to Markdown.
- Custom styles require a reference document (`--reference-doc`).
- Diagrams are rendered as PNG via Kroki and embedded as images.
