---
title: "Sample Technical Document"
author: "ktw-docs Test Suite"
date: 2026-03-08
---

# Introduction

This is a **sample document** for testing the ktw-docs rendering pipeline. It exercises the major Markdown features that ktw-docs must support.

## Text Formatting

- **Bold text** and *italic text* and ***bold-italic***
- ~~Strikethrough~~ and `inline code`
- ==Highlighted text== (if supported)
- H~2~O (subscript) and x^2^ (superscript)

## Lists

### Unordered

- First item
  - Nested item
  - Another nested
- Second item
- Third item

### Ordered

1. Step one
2. Step two
3. Step three

### Task List

- [x] Implement Kroki client
- [x] Build Pandoc executor
- [ ] Add template management
- [ ] Write integration tests

## Links and Images

- [ktw-docs README](../README.md)
- External: [Pandoc User Guide](https://pandoc.org/MANUAL.html)

## Code Blocks

```go
func main() {
    fmt.Println("Hello from ktw-docs!")
}
```

```python
def render_diagram(source: str, diagram_type: str) -> bytes:
    """Render a diagram via Kroki API."""
    url = f"https://kroki.io/{diagram_type}/svg"
    return requests.post(url, json={"diagram_source": source}).content
```

## Tables

| Feature | Status | Notes |
|---------|--------|-------|
| PDF rendering | Done | Pandoc + Typst |
| DOCX rendering | Done | Pandoc |
| Diagram rendering | Done | Kroki API |
| Template management | Planned | Phase 3 |

## Blockquotes

> This is a blockquote. It can contain **formatted text** and
> span multiple lines.

### GitHub Alerts

> [!NOTE]
> This is a note alert — useful for additional context.

> [!WARNING]
> This is a warning — pay attention to this section.

> [!TIP]
> This is a tip — a helpful suggestion for the reader.

## Horizontal Rule

---

## Footnotes

ktw-docs uses Pandoc[^1] for document conversion and Kroki[^2] for diagram rendering.

[^1]: Pandoc is a universal document converter. See https://pandoc.org
[^2]: Kroki provides a unified API for diagram generation. See https://kroki.io

## Math (if supported)

Inline: $E = mc^2$

Display:

$$
\sum_{i=1}^{n} x_i = x_1 + x_2 + \cdots + x_n
$$

## Conclusion

This document tests the core Markdown rendering capabilities of ktw-docs. All features above should render correctly in PDF, DOCX, and HTML output formats.
