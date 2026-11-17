---
name: display-document-demo
advertised: false
description: Sub-task of display-documents-showcase — produces ONE text document of the chosen type (a Markdown report with mermaid diagrams, an HTML dashboard, a mermaid diagram, or code and data) with create-document; it opens as a tab and streams in live
type: task
iconName: file-text
icon: "📄"
hashtags: [demo, document, gui, streaming]
category: demo
config:
  capability: smart
  plugins:
    - standard
---

# Document demo — one text document by media type

You demonstrate the `create-document` tool: it opens a document as a new tab in
the GUI, which picks the viewer from the document's **media type** and renders
it while you write it.

## Step 1 — Read your input

Read `topic` and `document` from the `FORM_INPUT` block (or from the message
text when run from the CLI). Do NOT use `ask-user`. When `document` is absent,
use `markdown`.

## Step 2 — Create the document

Call `create-document` ONCE, with `title`, `mediaType` and `content` as below.
The content must genuinely cover the topic — no lorem ipsum, no preamble, no
apology inside the document.

### `markdown` — mediaType `text/markdown`

A report of 40–80 lines:
- a `# H1` title and at least 3 `## H2` sections;
- a Markdown table with at least 3 rows;
- a bulleted list and a numbered list;
- **two** ```` ```mermaid ```` fenced blocks, drawn as diagrams in the tab: a
  `flowchart` of a process of the topic and a `sequenceDiagram` (or a
  `timeline`) of its actors;
- one fenced code block in a programming language;
- bold and italic emphasis, a blockquote, a horizontal rule.

### `html` — mediaType `text/html`

A self-contained page (`<!DOCTYPE html>`, inline `<style>`, no external
script, CSS or image — the iframe is sandboxed) rendering a metrics dashboard:
a dark theme (background `#1a1b1e`, light-grey text), at least 4 metric cards
in a CSS grid (label, large value, subtitle), a progress bar, a `<table>` with
3+ rows, colour-coded badges (green healthy, yellow warning, red critical).
60–120 lines.

### `mermaid` — mediaType `text/vnd.mermaid`

ONE mermaid diagram (no fences, no prose — the whole content is mermaid
source): a `flowchart TD` of 10–16 nodes with at least two subgraphs and
labelled edges, describing how the topic works.

### `code` — three documents

Call `create-document` three times, each a real artifact of the topic:
1. mediaType `application/json` — a configuration or API response (30–60 lines);
2. mediaType `application/yaml` — the same data as a YAML configuration;
3. mediaType `text/x-python` (or `text/x-go`, `text/x-typescript`) — a
   40–80 line module that uses that configuration.

## Step 3 — Complete

Call `complete` with one line naming what you produced (e.g. `"Opened a
Markdown report on <topic> with two mermaid diagrams."`).
