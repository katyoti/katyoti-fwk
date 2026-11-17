# Convert Document

Reference for compiling Markdown to PDF, DOCX, PPTX, or HTML using the `render` tool.

---

## Purpose

Compile a Markdown source file into a formatted output document (PDF, DOCX, PPTX, HTML) via the ktw-docs sidecar's `render` tool. The tool handles Pandoc + Typst + Kroki diagram rendering server-side — no local pandoc, typst, python, or pandocfilters install required.

## Scope

Input: a Markdown file path and a target format (one of `pdf`, `docx`, `pptx`, `html`). Optional template or reference document for styling.

Out of scope: conversions where neither input nor output is Markdown or one of the four supported outputs (e.g. LaTeX → EPUB, DOCX → Markdown, Jupyter round-trips). For those, a direct host `pandoc` call remains the user's fallback — this skill does not wrap it.

## Process

### Step 1 — Identify Source and Target

Parse the arguments to determine:
1. Input file — must be a Markdown (`.md`) file. If the source is in another format, convert it to Markdown first using the `convert-to-markdown` reference.
2. Target format — one of `pdf`, `docx`, `pptx`, `html`.

#### Supported Target Formats

| Extension | `output_format` | Notes |
|---|---|---|
| `.pdf` | `pdf` | Rendered via Pandoc + Typst |
| `.docx` | `docx` | Use `reference_doc` for corporate styling |
| `.pptx` | `pptx` | Use `reference_doc` for slide master |
| `.html` | `html` | Standalone HTML |

### Step 2 — Fenced Diagrams

Kroki diagram rendering happens automatically inside the tool. Fenced code blocks with a diagram-language identifier (`mermaid`, `plantuml`, `graphviz`, `d2`, `vegalite`, …) are rendered to images before conversion. No filter flag, no `KROKI_INPUT_FILE` env var, no `pandocfilters` dependency. Just write the fenced block — see the [rich-markdown.md](rich-markdown.md) reference for the authoring pattern.

### Step 3 — Build the Tool Call

Call the `render` tool with these parameters:

| Parameter | Required | Purpose |
|---|---|---|
| `input_path` | yes | Path to the Markdown source file |
| `output_path` | yes | Path for the rendered output |
| `output_format` | yes | One of `pdf`, `docx`, `pptx`, `html` |
| `template` | no | Template name (bundled) or absolute path to a custom template |
| `reference_doc` | no | Reference document for DOCX / PPTX styling |

#### PDF Output (Markdown → PDF via Typst)

```json
// Basic
{
  "input_path": "input.md",
  "output_path": "output.pdf",
  "output_format": "pdf"
}

// With a bundled Typst template
{
  "input_path": "input.md",
  "output_path": "output.pdf",
  "output_format": "pdf",
  "template": "assets/templates/article.typ"
}
```

#### Bundled Typst Templates

| Template | Best For | Path |
|---|---|---|
| report.typ | Reports with title page, TOC, headers/footers | `assets/templates/report.typ` |
| article.typ | Papers, blog posts, articles | `assets/templates/article.typ` |
| letter.typ | Formal letters with sender/recipient | `assets/templates/letter.typ` |
| invoice.typ | Structured invoices | `assets/templates/invoice.typ` |

Paths are resolved relative to the skill root directory. For bibliographies, citations (`--citeproc`), and custom templates the tool accepts the same template mechanics Pandoc does — pass the template path via `template`.

#### DOCX Output

```json
// Basic
{
  "input_path": "input.md",
  "output_path": "output.docx",
  "output_format": "docx"
}

// With reference doc styling
{
  "input_path": "input.md",
  "output_path": "output.docx",
  "output_format": "docx",
  "reference_doc": "reference.docx"
}
```

Custom styling via reference documents:
1. Generate a default reference using the host `pandoc` CLI once: `pandoc -o custom-reference.docx --print-default-data-file reference.docx`.
2. Open in Word, modify styles (Home → Styles panel).
3. Pass the saved file via `reference_doc`.

#### Slides to present

A presentation to show or present is a **deck** — markdown shown live with
reveal.js by the display tool (`display-ui-component`, `kind: "deck"`), with a
speaker view, annotations and PDF. Load `ktw-docs:/skill/deck-author`. The
sections below produce slide FILES for PowerPoint or print.

#### Slides — PDF

```json
{
  "input_path": "deck.md",
  "output_path": "slides.pdf",
  "output_format": "pdf",
  "template": "assets/templates/slide-template.typ"
}
```

Paper sizes are controlled by the template (`presentation-16-9`, `presentation-4-3`).

#### Slides — PowerPoint PPTX

```json
// Basic
{
  "input_path": "deck.md",
  "output_path": "slides.pptx",
  "output_format": "pptx"
}

// With custom slide master
{
  "input_path": "deck.md",
  "output_path": "slides.pptx",
  "output_format": "pptx",
  "reference_doc": "template.pptx"
}
```

To create a PPTX master:
1. Generate via host `pandoc`: `pandoc -o template.pptx --print-default-data-file reference.pptx`.
2. Edit Slide Master in PowerPoint (View → Slide Master).
3. Pass the saved file via `reference_doc`.

#### Standalone HTML

Choose `output_format: html` with a plain Markdown source. For a self-contained page (images and CSS inlined), include `embed-resources: true` in the source's YAML frontmatter — Pandoc picks it up via metadata.

### Step 4 — Execute and Verify

1. Invoke `render` with the constructed arguments.
2. If the tool returns an error, check:
   - **PDF**: set `mainfont` in the Markdown frontmatter for reliable font rendering; large diagrams may need a local Kroki server (sidecar configuration).
   - **DOCX**: the `reference_doc` must be a valid Word file with the expected named styles.
   - **Slides**: for PPTX, verify in PowerPoint; for slide PDF, check page breaks.
   - **General**: the input must be UTF-8 Markdown.
3. The tool returns the resolved output path and format — report it back to the user.

## Output Format

The tool returns a JSON object: `{ "outputPath": "<path>", "format": "<format>" }`. All invocations produce a single output file.

## Constraints

- Input must be Markdown (`.md`). For other input formats, convert via [convert-to-markdown.md](convert-to-markdown.md) first.
- Output format is one of `pdf`, `docx`, `pptx`, `html`. For exotic outputs (EPUB, LaTeX source, RST, Jupyter), host `pandoc` is the user's fallback — this skill does not wrap it.
- PDF output depends on the sidecar having access to its embedded Typst engine and fonts. `mainfont` in the source frontmatter gives the most reliable rendering.
- Pandoc expects UTF-8 input.
- Some conversions lose information due to format differences:
  - Markdown to DOCX: custom styles require a reference document.
  - Complex math in HTML: use MathJax by enabling it in the Markdown source.
