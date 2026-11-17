# Diagram Pitfalls — Kroki + Pandoc + Typst

Known issues when embedding diagrams in markdown for PDF generation via Pandoc/Typst with Kroki rendering.

---

## Mermaid Case-Sensitive Keywords

Mermaid keywords are **case-sensitive** in Kroki. Using the wrong casing produces HTTP 400.

| Wrong | Correct |
|---|---|
| `gitgraph` | `gitGraph` |
| `stateediagram-v2` | `stateDiagram-v2` |
| `sequencediagram` | `sequenceDiagram` |
| `classdiagram` | `classDiagram` |
| `erdiagram` | `erDiagram` |

The code block language tag (` ```mermaid `) is lowercase, but the **first line inside** the block must use official Mermaid casing.

---

## Mermaid SVG + Typst = No Text

Mermaid renders text using `<foreignObject>` (HTML embedded in SVG). Typst cannot render `<foreignObject>`, so **all text labels disappear** in PDF output.

**Fix**: `render` auto-detects Typst output and rasterizes Mermaid (and other diagram types that rely on `<foreignObject>`) to PNG instead of SVG before compilation. This is handled inside the sidecar; no markdown changes are needed. Be aware:
- Mermaid diagrams in PDFs will be rasterized (PNG) — slightly less crisp than vector
- D2, PlantUML, GraphViz produce clean SVGs (no foreignObject) — these stay as SVG

---

## Mermaid stateDiagram: No `note` Directives

Kroki's Mermaid version does not support `note` directives in `stateDiagram-v2`. These produce HTTP 400.

Bad:
```
stateDiagram-v2
    [*] --> Active
    Active --> Done
    note right of Active: This causes 400
```

Good — use comments or labels instead:
```
stateDiagram-v2
    [*] --> Active
    Active --> Done : completes
```

---

## YAML Frontmatter: Always Set `mainfont`

Pandoc 3.6.4+ generates `font: ()` (empty tuple) in the Typst template when no `mainfont` is specified. Typst 0.14+ rejects empty font lists with:

```
error: font fallback list must not be empty
```

**Always include a mainfont in frontmatter:**

```yaml
---
title: "My Document"
mainfont: "Segoe UI"
---
```

Safe cross-platform font choices:
| OS | Fonts |
|---|---|
| Windows | `Segoe UI`, `Calibri`, `Arial` |
| macOS | `Helvetica Neue`, `SF Pro`, `Arial` |
| Linux | `DejaVu Sans`, `Liberation Sans`, `Arial` |
| Universal | `Arial`, `Times New Roman` |

---

## Mermaid Diagram Size Limits

Very large Mermaid diagrams (many nodes, long labels) may hit Kroki's rendering timeout (60s) or memory limits. If a complex diagram fails:
- Split into multiple smaller diagrams
- Simplify labels (shorter text)
- Reduce node count

---

## D2 Special Characters in Labels

D2 labels with special characters need quoting:

```d2
# Bad — colon breaks parsing
node: Label: with colon

# Good — quote the label
node: "Label: with colon"
```

---

## Summary Checklist

Before generating PDF, verify:

1. Mermaid keywords use official casing (`gitGraph`, `stateDiagram-v2`, `sequenceDiagram`)
2. No `note` directives in `stateDiagram-v2`
3. YAML frontmatter includes `mainfont`
4. D2 labels with special characters are quoted
5. Diagram code blocks use correct language tag (`mermaid`, `d2`, `plantuml`, `graphviz`)
