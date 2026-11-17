# Slide Deck

Reference for producing slide FILES — PDF slides or PowerPoint PPTX — through a structured 4-phase process with Pandoc.

A presentation to **show or present** is not made here: it is a deck, markdown shown live with reveal.js (speaker view, annotations, PDF) — load `ktw-docs:/skill/deck-author`. Use this reference only when the user needs a `.pptx` or a slide PDF file.

---

## Purpose

Create presentation slide decks using a structured 4-phase pipeline: describe goals, build a skeleton, write full slide markdown with diagrams and speaker notes, then compile to the final format. Supports Kroki diagrams (Mermaid, PlantUML, GraphViz, D2, Vega-Lite, and 28+ types), speaker notes, incremental reveals, multi-column layouts, and themes.

## Scope

Input: a topic, outline, content description, or existing file path.

## Process

### Pipeline Overview

| Phase | Name | Output | Description |
|---|---|---|---|
| 1 | Description | `<name>.slides-brief.md` | Presentation goals, audience, key messages |
| 2 | Skeleton | `<name>.slides-skeleton.md` | Slide-by-slide structure |
| 3 | Full Markdown | `<name>.slides.md` | Complete slide deck with content and diagrams |
| 4 | Compile | `<name>.html / .pdf / .pptx` | Final presentation (read `convert-document.md` reference) |

#### Phase Skipping

- If the input is an existing `.slides.md` file -- skip to Phase 4.
- If the input is an existing `.slides-skeleton.md` file -- skip to Phase 3.
- If the input is an existing `.slides-brief.md` file -- skip to Phase 2.

### Phase 1 -- Presentation Description

Create a brief document that defines the presentation's purpose and constraints. Determine from the input or by asking the user:

1. Audience: Who will watch? (technical team, executives, students, general public)
2. Goal: What should the audience take away?
3. Key Messages: 3-5 main takeaways
4. Duration: How long? (determines slide count -- roughly 1 slide per minute)
5. Tone: Formal, casual, technical, inspirational?
6. Output Format: PDF slides (polished) or PPTX (editable)?

Write to `<name>.slides-brief.md`:

```markdown
# Presentation Brief: <Title>

## Audience
<who>

## Goal
<what the audience should take away>

## Key Messages
1. <message 1>
2. <message 2>
3. <message 3>

## Constraints
- Duration: <X> minutes (~<Y> slides)
- Tone: <formal/casual/technical>
- Output format: <pdf/pptx>

## Additional Context
<any relevant information>
```

### Phase 2 -- Slide Skeleton

Based on the brief, create a structural outline of every slide. For each slide, define:

- Title
- Content type: text, bullet points, diagram, image, quote, two-column, etc.
- Diagram specification (if applicable): diagram type and what it should show
- Speaker notes summary: one-line description

Write to `<name>.slides-skeleton.md`:

```markdown
# Slide Skeleton: <Title>

## Slide 1: Title Slide
- Type: Title
- Content: Title, subtitle, author, date

## Slide 2: <Section Title>
- Type: Section divider

## Slide 3: <Slide Title>
- Type: Bullet points
- Content: 3-4 key points about <topic>
- Notes: Explain context and relevance

## Slide 4: <Slide Title>
- Type: Diagram
- Diagram: Mermaid flowchart showing <process>
- Notes: Walk through each step

## Slide 5: <Slide Title>
- Type: Two-column
- Left: Comparison point A
- Right: Comparison point B
- Notes: Highlight differences
```

Present the skeleton to the user for review. Iterate if they want to reorder, add, or remove slides.

### Phase 3 -- Full Slide Markdown

Convert the skeleton into a complete slide deck. Use the `rich-markdown.md` reference and the diagrams references for syntax. Read `diagrams/diagram-pitfalls.md` before creating diagrams.

#### Slide Structure Rules

- Level-1 headings (`# Section Title`) create section divider slides.
- Level-2 headings (`## Slide Title`) create content slides.
- Use `--slide-level=2` during compilation.

#### Markdown Format

````markdown
---
title: "Presentation Title"
author: "Speaker Name"
date: "YYYY-MM-DD"
---

# Section Title

## Slide Title

Content for this slide.

- Bullet point 1
- Bullet point 2
- Bullet point 3

::: notes
Detailed speaker notes for this slide.
:::

## Diagram Slide

```mermaid
graph LR
    A[Idea] --> B[Prototype] --> C[Product]
```

::: notes
Walk through the pipeline.
:::

## Two-Column Slide

:::::: columns
::: column
Left column content
:::
::: column
Right column content
:::
::::::

## Incremental Reveal

::: incremental
- First point (appears first)
- Second point (appears next)
- Third point (appears last)
:::
````

#### Authoring Guidelines

- Keep each slide focused on one idea.
- Maximum 3-5 bullet points per slide.
- Place diagrams on dedicated slides with minimal surrounding text.
- Use speaker notes for detailed talking points.
- Include section dividers between major topic changes.
- For PDF output: set `mainfont` in frontmatter (see `rich-markdown.md` reference PDF guidelines).

Write the full deck to `<name>.slides.md`.

### Phase 4 -- Compile

Read the `convert-document.md` reference for exact compilation commands. The convert-document reference bundles the Kroki filter and slide template. See its Slides sections for:

- PDF slides: Typst engine with `slide-template.typ` at `assets/templates/`, paper sizes (16:9, 4:3)
- PowerPoint PPTX: reference doc for custom styling

#### Verify

1. PDF: Check page breaks and diagram rendering.
2. PPTX: Open in PowerPoint, verify slide layouts.

## Constraints

- Level-1 headings are section dividers only, not content slides.
- Level-2 headings define content slides; use `--slide-level=2`.
- Maximum 3-5 bullet points per slide.
- Diagrams should be placed on dedicated slides.
- For PDF output, `mainfont` must be set in frontmatter.
- Requires pandoc (>=3.1.2); typst required for PDF output.
