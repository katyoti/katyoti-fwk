# Markmap Mind-Map

Reference for transforming any input into a hierarchical mind-map visualization using Markmap syntax.

---

## Purpose

Transform any input -- text, code, concepts, or data -- into a clear, hierarchical mind-map using Markmap-compatible Markdown syntax. Generate a `.mm.md` file that can be rendered as an interactive mind-map.

## Scope

Input: `$ARGUMENTS` -- a topic to visualize, a file path to read, or inline content.

Output: a `.mm.md` file (e.g., `topic-name.mm.md`) in the same directory as the input file, or in the current working directory if the input was inline.

Allowed tools: Read, Write, Glob.

## Process

### Step 1 -- Understand the input

1. If `$ARGUMENTS` is a file path, read the file in full.
2. Identify the core concepts, relationships, and hierarchy in the input.
3. If the input is too broad, identify the primary takeaway and focus on that.

### Step 2 -- Build the Markmap structure

Generate a Markdown document using Markmap-compatible syntax, following these rules strictly.

#### Syntax reference

Markmap renders standard Markdown headings and lists as a mind-map tree:

- `#` = root node (one only -- the central topic)
- `##` = primary branches
- `###` = sub-branches
- `- item` = leaf nodes (bulleted lists under any heading)
- Nested lists (`  - child`) create deeper branches

Supported inline formatting:

- **bold**, *italic*, ~~strikethrough~~, ==highlight==, `inline code`
- `[links](url)`
- `[x]` checkboxes
- KaTeX math: `$formula$`
- Code blocks, tables, and images (rendered inside nodes)

#### Optional YAML front matter

```yaml
---
markmap:
  colorFreezeLevel: 2
  maxWidth: 300
---
```

#### Structure guidelines

- One root `#` heading -- the central topic or title.
- 3-7 primary branches `##` -- the main categories or dimensions.
- Sub-branches `###` and lists -- supporting details, examples, relationships.
- Use list nesting depth (up to 3-4 levels) to show granularity.
- Keep node text concise -- short phrases, not full sentences.
- Use **bold** for key terms, `code` for technical identifiers, *italic* for annotations.
- Add `<!-- markmap: fold -->` after a list item to collapse that branch by default.
- If blocks and lists appear at the same level, lists are ignored by Markmap -- avoid mixing them.

#### Complexity calibration

- **Simple input** (single concept, short text): 1 root + 3-5 branches, shallow depth.
- **Moderate input** (multi-concept, article): 1 root + 5-7 branches, 2-3 levels deep.
- **Complex input** (architecture, large document, dense data): 1 root + 5-7 branches, 3-4 levels deep with folded sub-branches.

### Step 3 -- Write the output

1. Write the Markmap Markdown to a `.mm.md` file (e.g., `topic-name.mm.md`) in the same directory as the input file, or in the current working directory if the input was inline.
2. Output the generated Markmap content in the response so the user can preview it.

### Step 4 -- Explain the visualization

After the Markmap output, provide a brief explanation section:

- 3-5 bullet points (max 15 words each) explaining how the input was organized into the map.
- If the input was simplified to fit, explain what was summarized and why.

## Output Format

File extension: `.mm.md`

The file contains standard Markdown with optional YAML front matter for Markmap configuration. Headings define the tree hierarchy; bulleted lists define leaf nodes.

## Constraints

- Exactly one `#` root heading per file.
- 3-7 primary `##` branches.
- Do not mix blocks and lists at the same level (lists are ignored by Markmap in that case).
- Keep node text concise -- short phrases, not full sentences.
- Explanation bullets must be max 15 words each.
