# DOCX Template Guide

ktw-docs renders DOCX templates using Jinja2 syntax (via the gonja engine). This guide explains how to create well-structured templates.

## Quick Start

1. Create a `.docx` file in Word
2. Add Jinja2 expressions: `{{variable}}`, `{% for %}`, `{% if %}`
3. Render: `ktw-docs render-template template.docx --data-file data.json`

## Variable Substitution

Use `{{variable}}` anywhere in your document:

```
Dear {{name}},

Your order #{{order.id}} has been shipped on {{order.date}}.
```

Nested access: `{{meetingDetails.displayDate}}`, `{{participant.role}}`

## Control Flow Prefixes (IMPORTANT)

Jinja2 control tags (`{% for %}`, `{% if %}`, `{% endfor %}`, etc.) must use a **prefix** that tells the engine which XML element to remove.

### `{%p %}` — Paragraph level

Use when the tag occupies its **own paragraph** (its own line in Word).
The entire paragraph is removed after rendering.

```
{%p for item in items %}
• {{item.name}} — {{item.description}}
{%p endfor %}
```

**Without `{%p %}`**, the paragraph containing `{% for %}` remains as an empty line in the output.

### `{%tr %}` — Table row level

Use when the tag controls a **table row**. The entire `<w:tr>` is removed.

```
┌──────────┬──────────────┐
│ Name     │ Role         │
├──────────┼──────────────┤
│ {%tr for p in participants %}              │
│ {{p.name}} │ {{p.role}} │
│ {%tr endfor %}                             │
└──────────┴──────────────┘
```

### `{%tc %}` — Table cell level

Use when the tag controls a **table cell**.

### `{%r %}` — Run level (inline)

Use when the tag is inline with other text in the same paragraph.

### No prefix — stays inline

Use when the tag is part of a paragraph with other content:

```
Items: {% for item in items %}{{item.name}}, {% endfor %}
```

### Summary

| Prefix | Removes | Use when |
|--------|---------|----------|
| `{%p %}` | Entire paragraph (`<w:p>`) | Tag is alone on its own line |
| `{%tr %}` | Entire table row (`<w:tr>`) | Tag controls a table row |
| `{%tc %}` | Entire table cell (`<w:tc>`) | Tag controls a table cell |
| `{%r %}` | Entire run (`<w:r>`) | Tag is inline but should remove formatting |
| (none) | Nothing | Tag is inline with other content |

## Loops

### List items (each on its own paragraph)

```
{%p for person in people %}
• {{person.name}} ({{person.role}})
{%p endfor %}
```

### Table rows

```
| Header 1 | Header 2 |
|----------|----------|
| {%tr for row in data %}            |
| {{row.col1}} | {{row.col2}} |
| {%tr endfor %}                     |
```

### Inline (comma-separated)

```
Participants: {% for p in people %}{{p.name}}{% if not loop.last %}, {% endif %}{% endfor %}
```

## Conditionals

```
{%p if show_section %}
This section is conditionally visible.
{%p endif %}
```

Inline: `Status: {% if approved %}✓ Approved{% else %}⏳ Pending{% endif %}`

## Filters

Gonja supports Jinja2 filters:

```
{{name | upper}}              → JOHN DOE
{{name | lower}}              → john doe
{{name | capitalize}}         → John doe
{{name | title}}              → John Doe
{{description | truncate(50)}} → First 50 chars...
{{items | length}}            → 5
{{price | default("N/A")}}   → N/A if price is undefined
```

## Document Properties

Template expressions in document properties (Title, Subject, Categories) are also rendered. 
In Word: File → Info → Properties.

```
Title: {{project}}
Subject: {{title}}
Categories: {{category}}
```

**Note**: SDT content controls linked to document properties (via dataBinding) display the property value. Make sure both the SDT text AND the property contain the same template expression.

## Headers and Footers

Template expressions in headers and footers are rendered automatically:

```
Header: {{project}} — {{title}}          {{classification}}
Footer: Page X of Y
```

## Structured Document Tags (SDT)

Word uses SDTs for cover page fields (Title, Subtitle, etc.). These are rendered automatically. If an SDT has a `showingPlcHdr` flag, it's removed after rendering so Word displays the actual content.

## Best Practices

1. **Always use `{%p %}` for standalone control tags** — prevents empty lines
2. **Always use `{%tr %}` for table row loops** — prevents empty table rows  
3. **Test with real data** — render and open in Word to verify
4. **Keep expressions simple** — avoid complex logic in templates; prepare data in JSON
5. **Use document properties** — for values that appear in SDT-linked cover page fields
6. **Name variables descriptively** — `{{meetingDetails.displayDate}}` not `{{d}}`

## Example: Meeting Minutes Template

**Data (JSON):**
```json
{
  "project": "Architecture Review",
  "title": "Steering Meeting #1",
  "meetingDetails": {
    "displayDate": "December 2, 2024",
    "facilitators": [
      {"displayName": "John Doe", "role": "Architect"}
    ],
    "participants": [
      {"displayName": "Jane Smith", "role": "Director"}
    ]
  }
}
```

**Template (in Word):**

Cover page:
```
{{project}}
{{title}}
```

Body:
```
Facilitators:
{%p for f in meetingDetails.facilitators %}
• {{f.displayName}} ({{f.role}})
{%p endfor %}

Participants:
{%p for p in meetingDetails.participants %}
• {{p.displayName}} ({{p.role}})
{%p endfor %}
```

Table:
```
| Decision | Action Items |
|----------|-------------|
| {%tr for o in outcomes %}              |
| {{o.decision}} | {{o.actionItems}} |
| {%tr endfor %}                         |
```

## Style Names

**Important**: Style macros (`<!--ktw table_style: ... -->`, `{%p_style %}`) must use Word's **internal style ID**, not the display name.

To find the style ID, unzip the `.docx` and search `word/styles.xml`:

```bash
unzip -p template.docx word/styles.xml | grep 'w:styleId='
```

Common mappings:

| Display Name (Word UI) | Internal Style ID |
|------------------------|-------------------|
| Light List Accent 1 | `LightList-Accent1` |
| Grid Table 4 Accent 1 | `GridTable4-Accent1` |
| Medium Shading 1 Accent 1 | `MediumShading1-Accent1` |
| Table Grid | `TableGrid` |
| Body Text | `BodyText` |
| Subtitle | `Subtitle` |
| Heading 1 | `Heading1` |
| List Paragraph | `ListParagraph` |
| Normal | `Normal` |

### Pandoc reference doc

When using `.md` as DataFile with a `.docx` template, Pandoc uses the template as `--reference-doc` for styles. The reference doc should define these Pandoc styles to control body text formatting:

- `First Paragraph` — first paragraph after a heading
- `Body Text` — normal body paragraphs
- `Compact` — list items
- `Source Code` — code blocks

If these styles are missing from the reference doc, Pandoc injects its defaults which may not match your template's layout.

## Markdown Templates

Markdown files (`.md`) can also be used as templates with full Jinja2 support:

```
ktw-docs render-template template.md --data-file data.json --output report.docx
```

The markdown is rendered with gonja (Jinja2), then converted to DOCX via Pandoc.

### Style macros in Markdown

Use `<!--ktw -->` comments to control Word styles:

```markdown
<!--ktw table_default_style: Light Grid Accent 1 -->
<!--ktw p_style: Subtitle -->
Section Title
<!--ktw p_style: -->
<!--ktw pagebreak -->
```

See the Lua filter documentation for all available macros.

## Markdown as Data Source

When using a `.docx` template with a `.md` data file:

```
ktw-docs render-template template.docx --data-file content.md
```

The markdown frontmatter provides template variables, and the body is converted via Pandoc and injected at `{{content}}` in the DOCX template.
