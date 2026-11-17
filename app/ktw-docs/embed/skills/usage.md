---
name: usage
description: Getting started guide for ktw-docs — document and diagram transformation sidecar
category: system
advertised: false
---

# ktw-docs — Usage Guide

ktw-docs is a document and diagram transformation sidecar. It renders Markdown to PDF/DOCX/PPTX, generates architecture diagrams from structured data, and manages document templates.

## Use Cases

| I want to... | Use skill |
|---|---|
| Create a document (PDF, DOCX, slides, diagram, concept map, mind map) | `ktw-docs:doc-create` |
| Analyze, review, or evaluate a document | `ktw-docs:doc-analyze` |
| Fix formatting or improve writing in an existing document | `ktw-docs:doc-fix` |

## Quick Examples

```bash
# Render Markdown to PDF
ktw-docs llm tools call docs-render '{"input":"report.md","format":"pdf","output":"report.pdf"}'

# Render a diagram source file (mermaid, plantuml, drawio, ...) to an image
ktw-docs llm tools call docs-diagram '{"source":"flow.mmd","format":"svg","output":"flow.svg"}'

# Author an editable drawio diagram from a logical diagram
ktw-docs drawio render logical.json -o diagram.drawio

# Render data into a template
ktw-docs llm tools call docs-render-template '{"template":"template.docx","dataFile":"data.json","output":"result.docx"}'
```

Load a skill for detailed guidance on any use case.
